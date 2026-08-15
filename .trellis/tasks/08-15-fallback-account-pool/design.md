# Technical design

## 1. Summary

Introduce an account-owned boolean, `is_fallback`, and enforce one invariant at every traffic-allocation entry point:

> After all constraints for the current attempt are applied, select primary candidates when that set is non-empty; otherwise select fallback candidates. Run every existing preference and capacity algorithm only inside that one role.

Repository queries and scheduler snapshots retain both roles. Role partitioning happens in the service layer because only that layer has complete request-specific context.

## 2. Invariants and terminology

### 2.1 Candidate stages

1. **Base candidates:** accounts in the resolved group/ungrouped scope and platform/mixed-platform scope.
2. **Request-eligible candidates:** accounts that pass all existing non-concurrency conditions for this attempt, including exclusions, model mapping, endpoint/image capability, credentials/account type, account state, model-level state, rate-limit precheck, quota, privacy, runtime blocks, parent health, transport, channel rules, and profit controls.
3. **Preferred role:** primary when any request-eligible primary exists; otherwise fallback.
4. **Preference selection:** model routing, subscription priority, sticky bindings, previous-response affinity, priority, score, provider rank, random/LRU, and cost ordering, applied only inside the preferred role.
5. **Capacity admission:** slot acquisition or the existing wait plan. A full concurrency slot never changes the preferred role.

`is_fallback=false` means primary. `is_fallback=true` means fallback.

### 2.2 Dynamic admission failures

Some existing checks happen during or after selection, such as session registration and terminal profit/capability checks. When a non-concurrency check rejects an account, add it to the attempt-local exclusions and rebuild request eligibility and the preferred role.

Concurrency failure is different: if the preferred role has eligible accounts but no free slot, keep selection and waiting in that role. Sticky-escape logic may move to another account in the same role, but cannot activate fallback merely because primary concurrency is full.

### 2.3 Decision and mutation consistency points

A scheduling decision establishes its consistency point after it has loaded complete account metadata, applied request eligibility, and partitioned the eligible candidates. A mutation published before that candidate read must be observed. A mutation published after the decision has established the snapshot races that decision and affects the next one.

For role edits, database commit alone is not the routing-visibility point. The write path must:

1. commit `accounts.is_fallback` and its outbox event in one database transaction;
2. synchronously publish the committed account to the shared Redis full-account and metadata keys with one atomic, monotonic write, or atomically invalidate those keys so readers miss and use the existing DB fallback path;
3. report mutation success only after every affected account has been published or invalidated.

Use a dedicated database-authoritative `pool_revision BIGINT`, not `UpdatedAt`, as the cache fence. Every explicit `is_fallback` mutation executes `pool_revision = pool_revision + 1` in the same row update/transaction and returns the committed row. PostgreSQL row locking therefore serializes concurrent role edits in commit-visible order without relying on application clocks or transaction-start timestamps. One shared `formatPoolRevision` helper encodes every non-negative revision as exactly 20 zero-padded decimal characters; Redis/Lua compares only that fixed-width text and never converts the int64 revision through a floating-point number. Legacy rows/cache entries are revision zero.

A failed publication leaves the durable outbox event for repair and returns a retriable mutation error instead of reporting a routing-visible success. Bulk edits are complete only after all affected account snapshots have reached the same outcome; partial publication is treated as pending repair, not success.

Before returning a hydrated/DB-rechecked account, reread its current shared account snapshot and verify that its role matches the role chosen for the attempt. Role drift releases any acquired slot and restarts selection. A different account recovering after the consistency point is intentionally observed by the next decision.

## 3. Data model and migration

### 3.1 Schema

Add to `backend/ent/schema/account.go`:

```go
field.Bool("is_fallback").
    Default(false).
    Comment("Eligible only when no request-eligible primary account exists.")

field.Int64("pool_revision").
    Default(0).
    NonNegative().
    Comment("Monotonic internal revision for fallback-role cache publication.")
```

Add `IsFallback bool` and internal `PoolRevision int64` to `service.Account` and Ent/service mappers. Add only `is_fallback` to shallow/full admin DTOs and API responses, explicitly without `omitempty`. `PoolRevision` is serialized in scheduler cache payloads but is not an administrator API field, backup field, filter, or UI concept.

No field is added to `account_groups`; group bindings cannot override the global role.

### 3.2 Migration and generation

Create idempotent `backend/migrations/196_account_fallback_pool.sql`:

```sql
ALTER TABLE accounts
ADD COLUMN IF NOT EXISTS is_fallback BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE accounts
ADD COLUMN IF NOT EXISTS pool_revision BIGINT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_accounts_is_fallback_active
ON accounts (is_fallback)
WHERE deleted_at IS NULL;
```

The index supports administrator filtering; scheduler hot paths do not query by role. Run `make generate` from `backend/` and review all generated Ent changes. Do not hand-edit generated files.

### 3.3 Compatibility

- Existing rows receive `is_fallback=false` and `pool_revision=0` from database defaults.
- Old Redis JSON, revision-less Redis keys, and old backup JSON omit the fields and decode as primary/revision zero.
- Old API clients omit `is_fallback` on create and get primary behavior.
- `pool_revision` is internal state: duplication, shadow creation, and backup import create a new account at revision zero while preserving only the role.
- Old application code safely ignores the additive columns during rollback; do not drop them during code rollback.

## 4. Role propagation

| Operation | Role behavior |
| --- | --- |
| Standard/specialized create | Accept `is_fallback`; omitted/false is primary. |
| Automatic internal create | Primary unless a rule below says otherwise. |
| Account update | `*bool`: nil unchanged; false/true explicitly sets role. |
| Bulk update | Same pointer semantics for ID and filtered targets. |
| Duplicate | Copy the source role as durable configuration. |
| Backup export/import | Preserve the role; old payloads default primary. |
| Create Spark shadow | Copy parent role at creation. |
| Later parent/shadow edits | Independent roles; no ongoing propagation. |
| Copy accounts between groups | Bindings change; global roles do not. |

Shadows already own independent status, priority, concurrency, and groups. One-time inheritance avoids surprising parent-driven changes while giving a newly generated shadow the expected initial role. New identities (create, duplicate, import, shadow) begin at `pool_revision=0`; every explicit role update, including an idempotent same-value assignment, atomically increments the existing account's revision. Revision values are never copied or exported.

## 5. Shared pool primitive

Add a package-private helper near the scheduling domain, preferably `backend/internal/service/account_pool.go`:

```go
type accountPoolRole string

const (
    accountPoolPrimary  accountPoolRole = "primary"
    accountPoolFallback accountPoolRole = "fallback"
)

type accountPoolPartition[T any] struct {
    Primary  []T
    Fallback []T
}

func partitionAccountPool[T any](items []T, accountOf func(T) *Account) accountPoolPartition[T]
func (p accountPoolPartition[T]) Preferred() ([]T, accountPoolRole)
```

The helper preserves order and performs no eligibility checks. Generic wrappers support `*Account`, OpenAI candidates, and batch-image `(provider, account)` pairs without duplicating the invariant. If `accountOf` returns nil, that item is skipped and can never count as a primary; normal callers still pass only request-eligible non-nil accounts.

Unit tests cover primary preference, fallback-only behavior, empty input, stable order, nil items not blocking fallback, and recovery across consecutive decisions.

## 6. Scheduling integration

### 6.1 Generic gateway

Affected functions in `gateway_scheduling.go` include `SelectAccountForModelWithExclusions`, `SelectAccountWithLoadAwareness`, `selectAccountForModelWithPlatform`, and `selectAccountWithMixedScheduling`.

Required order:

1. Resolve existing Claude-Code fallback group, composite target, forced platform, and mixed scheduling.
2. Load the complete base candidates for that resolved scope.
3. Apply exclusions and every existing request gate. Sticky-only window/RPM allowances remain valid only for the bound account, but do not outrank role selection.
4. Partition eligible candidates and choose one role.
5. Apply model-routing preference only inside that role. If no routed candidate in that role is usable, continue with normal candidates in the same role.
6. Honor sticky only when the bound account belongs to the chosen eligible role.
7. Run priority/LRU/load logic and slot acquisition inside that role.
8. If all selected-role slots are full, return its existing wait plan.
9. On non-concurrency admission rejection, exclude and recompute; fallback becomes possible only after no eligible primary remains.

Thus an Anthropic route naming fallback accounts cannot bypass an eligible normal primary.

### 6.2 OpenAI-compatible legacy scheduler

`openai_gateway_scheduling.go` serves OpenAI and Grok compatibility paths.

- Build the complete eligible set using current parent-health, runtime, group, compact/endpoint/image capability, transport, channel, quota-auto-pause, profit, and exclusion gates.
- Partition before sticky, legacy priority/LRU/upstream-rate preference, load maps, and wait-plan selection.
- Carry the chosen role through fresh-account hydration and DB recheck. Reject and retry a fresh account whose role changed.
- Preserve existing compact-specific and no-account errors.

### 6.3 OpenAI advanced scheduler

Refactor `defaultOpenAIAccountScheduler.Select` so direct affinity does not run before the candidate pool is known:

1. Resolve the complete eligible set for `OpenAIAccountScheduleRequest`.
2. Partition before previous-response affinity, session sticky, subscription priority, score normalization, top-K, weighted selection, cost overflow, slot probing, and waits.
3. Previous-response and sticky IDs are direct hits/bonuses only if they belong to the chosen eligible role.
4. Subscription priority partitions only the chosen role; a fallback subscription account cannot outrank a primary regular account.
5. Normalize scores only across candidates in the chosen role.
6. Keep load retries and DB rechecks in that role.
7. If all primary slots are full, produce a primary wait plan.

Record chosen role and eligible counts in existing debug/decision data without introducing high-cardinality metrics.

### 6.4 `previous_response_id`

`resolveAccountByPreviousResponseIDForCapability` currently loads one account directly. Validate that binding against the current request's chosen role:

- apply group, exclusion, model, endpoint, compact, transport/WS, parent-health, runtime, quota-auto-pause, and profit gates;
- determine whether another eligible primary exists for the same request;
- accept a fallback binding only when no eligible primary exists;
- when bypassing a fallback binding only because primary recovered, keep the binding and fall through to regular scheduling;
- delete it only for existing permanently invalid cases.

Strict failback intentionally wins over response-chain affinity. No cross-account state migration is added. Existing upstream/retry handling owns any rejection of a response ID on the newly selected primary. An already-open fallback WebSocket remains in flight; a new connection reevaluates the boundary.

### 6.5 Gemini Messages compatibility

For `SelectAccountForModelWithExclusions`, resolve platform/mixed scope, precheck rate limits, apply exclusions/model/account/platform gates, partition, and only then apply sticky and priority/LRU/OAuth preference.

For `SelectAccountForAIStudioEndpoints`, first remove accounts unable to serve `generativelanguage.googleapis.com`, then partition and apply API-key/OAuth rank, priority, and LRU. A primary Vertex-only service account is not matching for AI Studio and cannot block a capable fallback.

### 6.6 Batch-image selection

`BatchImagePublicService.selectProviderAndAccount` selects traffic independently. Request-global group authorization and user/group pricing prerequisites remain request admission and run before pair selection. Pair eligibility is the complete set of deterministic checks available before submission:

- the repository query establishes group/platform scope and persistent schedulability;
- `Account.IsSchedulable` and model mapping must accept the request;
- provider/model unit pricing must resolve for that provider, with existing pricing errors retained for diagnosis when no pair survives;
- `GeminiAPIBatchImageProvider.SupportsAccount` owns Gemini/API-key type and non-empty API-key validation;
- `VertexBatchImageProvider.SupportsAccount` owns Gemini/service-account type and parseable service-account credentials;
- any other deterministic provider/account rejection found after selection must move into this pair-building step.

For an explicit provider, build eligible pairs only for it. For automatic provider selection, build all eligible `(provider, account)` pairs across the existing provider order, partition pairs globally, then preserve provider order and account priority/ID ordering inside the chosen role. A fallback for the first visited provider therefore cannot bypass a primary for another compatible provider.

Batch-image selection has no sticky binding or concurrency wait plan. Network/transport failures that occur only during asynchronous provider execution remain in-flight job behavior and do not retrospectively change the pool decision.

### 6.7 Discovery and diagnosis

Do not role-filter scheduler bucket construction, model availability/diagnosis, available platform discovery, group capacity, batch-image model discovery, or admin candidate inspection. A model supported only by fallback must still be discoverable because there is no matching primary for that model.

### 6.8 Pinned asynchronous resources

New Grok media generation is a normal OpenAI/Grok allocation and follows the strict pool boundary. After a video request is created, `BindGrokMediaVideoRequestAccount` records the account that owns the upstream resource. `ResolveGrokMediaVideoRequestAccount` status/content lookups are continuations of that already-created resource, not opportunities to allocate new traffic.

Refactor the Grok media handler so video status/content lookup validates the owner-scoped binding and resolves/hydrates that bound account directly. Apply existing group ownership, endpoint/capability, credentials, transport, status, and bound-account concurrency admission, but never probe another account. It must not invoke normal scheduling and then compare the newly selected ID, because primary recovery would produce a synthetic 404 for a video still owned by fallback. If the bound account/resource is unavailable, preserve the existing lookup failure; new video generations after recovery select primary.

This pinned-resource exception also describes existing batch-image worker/download behavior. It does not apply to `previous_response_id`, for which R10 explicitly requires strict failback and existing upstream rejection handling.

## 7. Scheduler cache and invalidation

Serialize `is_fallback` and `pool_revision` in both full and metadata account snapshots. Do not add the role to `SchedulerBucket`, bucket keys, or bucket membership queries. Store a separate per-account revision state key used only to fence fallback-role payload writers.

Strengthen the shared cache contract with two atomic operations:

1. `PublishAccountAtPoolRevision` compares the 20-character revision text with current state. Older writes are rejected; newer writes atomically store full payload, metadata payload, and `published` state. At equal revision, an ordinary snapshot rebuild is a no-op against either published data or a tombstone. Only a direct/outbox repair that has just reread the authoritative DB row may refresh equal-revision published data or replace an equal-revision tombstone.
2. `InvalidateAccountAtPoolRevision` rejects older invalidations, atomically removes full/metadata payloads, and stores a `tombstone` at the supplied revision. It must not call ordinary `DeleteAccount` or remove the revision fence.

Snapshot rebuilds, direct `SetAccount`, and outbox repair call the same primitive with an explicit writer authority. A rebuild loaded before a role edit carries a smaller revision and cannot overwrite either a newer payload or tombstone. An authoritative direct/outbox repair loaded after the role edit may publish at the tombstone's equal revision. Equal revision implies the same role because every role change increments the DB revision, while restricting equal replacement to authoritative readers prevents a delayed rebuild from regressing unrelated account fields.

When a stale publish is rejected, snapshot membership still retains that account ID and uses the already-newer shared payload; rejection must not silently remove the account from the bucket. Permanent account deletion remains a separate lifecycle operation and may remove the revision only after existing bucket membership fencing guarantees no older writer can reintroduce the account. Old cache payloads without a revision key are treated as published revision zero.

Role edits commit the account-change/bulk-change outbox event with the DB mutation, read back the committed revision, then synchronously publish. If publish fails, the strict path attempts revision-preserving invalidation; it reports success only if publish or fenced invalidation succeeds. The outbox repairs tombstones/interrupted publication and ordinary membership changes; it is not the normal role visibility boundary.

Both roles remain in each bucket. Tests cover reverse commit order, an application timestamp moving backward without affecting revision order, the single 20-character encoder at zero/max int64, legacy revision zero, atomic full/metadata publication, stale publish after normal publication and after tombstone invalidation, equal-revision rebuild no-op, authoritative equal-revision repair of a tombstone, bulk partial publication, permanent deletion fencing, and two scheduler instances observing the published role.

## 8. Administrator API

### 8.1 Resource field

Propagate `is_fallback` through create, update (`*bool`), bulk update (`*bool`), service/DTO responses, duplicate construction, backup `DataAccount`, and specialized create/import request types used by the create experience. This includes Codex session import, ChatGPT session import/DataAccount construction, Codex PAT creation, and direct OAuth-token-to-account branches; omitted values remain primary. Repository create/update/bulk builders explicitly persist it. Role-only edits trigger strict shared snapshot publication plus normal outbox handling.

### 8.2 List filter

Add `pool` with these values:

- empty: both roles;
- `primary`: `is_fallback=false`;
- `fallback`: `is_fallback=true`.

Invalid values return a structured 400. Propagate the filter through the handler, service, repository query, filtered bulk-edit target, filtered liveness request, scheduler-score list filter, and frontend filter snapshot. Include it in `buildAccountsListETag` and local row/filter reconciliation. No new sort key is needed.

## 9. Administrator UI

### 9.1 Create and edit

Add a standard binary switch labelled “Fallback account” near existing scheduling settings. The default is off. Its supporting text describes behavior, not implementation: the account receives new traffic only when no matching primary account is available.

Carry the value through all `CreateAccountModal.vue` branches that persist accounts, including direct `accounts.create`, OAuth-token-to-create flows, Codex import, and Codex PAT. Specialized backend requests that omit it still default to primary.

Initialize edit state from `account.is_fallback` and submit it through the existing `{ ...form }` update payload.

### 9.2 Bulk edit

Add an opt-in “Change account pool” field consistent with other optional bulk fields. When enabled, choose Primary or Fallback. Send `is_fallback` only when enabled. Both explicit-ID and filtered-target calls use identical semantics.

### 9.3 List and group views

- Add a default-visible, nonsortable Pool column with compact neutral Primary and amber Fallback badges.
- Add an All/Primary/Fallback select to `AccountTableFilters.vue` and preserve it in URL/filter snapshots where existing filters are persisted.
- Extend account types and API request types.
- In `GroupsView.vue`, show the badge on account search results and selected routing-account chips. It is read-only; no group form writes the role.
- Add English and Chinese i18n keys. Do not hard-code labels.

## 10. Failure handling and observability

- Empty preferred input returns the exact existing no-account/model-diagnosis behavior.
- Cache/DB errors continue through existing wrapped service errors.
- Invalid `pool` is a client error, not an empty result.
- Existing selection debug logs/decisions add `account_pool`, `eligible_primary_count`, and `eligible_fallback_count` where the scheduler already records a decision.
- Avoid warning/info logs for every fallback request and avoid a new per-account metric label.
- Admin role changes remain covered by existing account update/audit mechanisms; no separate audit subsystem is introduced.

## 11. Test strategy

### 11.1 Shared contract

Table-driven helper tests cover primary preference, failover, empty input, stable order, nil skipping, and recovery across consecutive decisions. Service/repository tests cover default false/revision zero, explicit true, update false with revision increment, concurrent/reverse-order role updates, bulk ID/filter modes, duplicate, shadow inheritance/independence, backup round trip, old payload defaults, DTO output, list filter validation/querying, ETag variance, and snapshot serialization/refresh. Cache consistency tests prove that role mutation success follows atomic shared publication or fenced tombstone invalidation, stale rebuilds cannot regress the role, equal-revision repair succeeds, failed/bulk-partial publication is not reported as success, and a second scheduler instance observes the published change.

### 11.2 Scheduler matrix

For each selector, test at least primary preference, fallback-only selection, recovery on the next call, request-ineligible primary, and no-account behavior:

| Path | Additional cases |
| --- | --- |
| Generic Anthropic | model routing to fallback cannot override primary; sticky fallback recovery; excluded primary allows fallback. |
| Generic Gemini/Antigravity/mixed | forced platform and mixed/composite constraints precede role; model mismatch primary does not block fallback. |
| OpenAI/Grok legacy | endpoint/compact/image capability, parent health, channel/transport/profit gates; role drift on DB recheck. |
| Grok pinned video | fallback-created video status/content remains on bound account after primary recovery; new generation returns to primary. |
| OpenAI advanced | subscription priority, weighted sticky, score/top-K isolation, all-primary-full wait plan, session admission exclusions. |
| Previous response/WS | fallback binding bypassed after primary recovery; binding retained; open in-flight connection unaffected. |
| Gemini Messages | sticky recovery, rate-limit exclusion, mixed platform, model-specific failover. |
| AI Studio | incompatible primary does not block capable fallback; ranking remains inside role. |
| Batch image | automatic cross-provider primary preference and explicit-provider failover. |

Use deterministic fake caches/load maps and seeded randomness where required. Assert selected IDs/roles and wait-plan role, not only that selection succeeds. For each selector family, assert the exact existing model-not-found versus temporarily-unavailable error identity/status wherever that path distinguishes them; batch-image instead asserts its existing pricing versus no-account errors. Batch-image tests also cover missing API-key, invalid service-account credentials, and provider-pricing rejection before partition.

### 11.3 Frontend

Vitest/component tests cover create default/payload, edit initialization and clearing, both bulk modes, list filter API/URL behavior, local row reconciliation, pool badges, and read-only group routing badges. Typecheck, lint, and production build are required.

## 12. Rollout and rollback

1. Apply additive migration and deploy backend/frontend as one feature release.
2. Before any account is marked fallback, scheduling remains behaviorally unchanged.
3. Mark a small controlled account set as fallback and verify debug decisions plus primary-full wait behavior.
4. Roll back behavior by clearing `is_fallback` for all accounts or deploying the previous application. Keep the additive role/revision columns and index.

No feature flag is required: the all-primary default is the compatibility switch.

## 13. Rejected approaches

- **Reuse `priority`:** advanced scoring can outweigh priority, so it cannot provide a hard boundary.
- **Reuse fallback groups:** existing fields have request-error and Claude-Code meanings, not availability semantics.
- **Store role in `extra` or account-group bindings:** weak typing and per-group divergence violate the global contract.
- **Filter fallback in repository queries:** direct cache/sticky/previous-response paths could bypass it, and request-specific primary eligibility is unknown there.
- **Create role-specific snapshot buckets:** duplicates cache dimensions and invalidation work without solving request-specific filtering.
- **Treat full concurrency as unavailable:** turns fallback into overflow capacity and violates the required queue behavior.
- **Continuously mirror parent role to Spark shadows:** makes independent account edits unstable and differs from existing one-time inheritance.
- **Treat DB commit or best-effort outbox repair as immediate role visibility:** leaves a commit-to-publication gap and permits stale rebuild overwrite; routing visibility requires atomic monotonic publication/invalidation.
- **Query the database on every scheduling decision:** would restore authority but defeats the snapshot hot path; shared atomic account publication gives the required boundary without per-request DB load.
- **Use `UpdatedAt` as a revision fence:** application clock skew and PostgreSQL transaction-start timestamps are not commit-monotonic; use row-serialized `pool_revision` instead.
- **Delete the revision key when invalidating stale payloads:** permits an older rebuild to resurrect the pre-edit role; retain a revision tombstone until authoritative repair.

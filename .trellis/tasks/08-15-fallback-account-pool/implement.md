# Implementation plan

## 1. Delivery strategy

Implement this as one Trellis task with ordered internal phases and one final integration gate.

### Parent/child decision

Do not split this feature into child tasks. The apparent deliverables (account role persistence/API, scheduler enforcement, and admin UI) are independently testable but not independently safe to release:

- exposing/persisting a fallback role before every selector enforces it lets administrators configure a role that traffic ignores;
- enforcing scheduling before mutation/UI propagation leaves no reliable way to configure or inspect the role;
- schema/cache changes and selector behavior share one compatibility contract.

Keep the phases reviewable in separate commits only if needed, but do not deploy or archive the task until all phases pass the cross-platform integration matrix.

## 2. Start gate

Before implementation:

- [ ] User reviews `prd.md`, `design.md`, and this plan.
- [ ] `implement.jsonl` and `check.jsonl` contain real spec/research entries.
- [ ] `task.py validate fallback-account-pool` passes.
- [ ] Only after explicit approval, run `task.py start fallback-account-pool`.
- [ ] Load the implementation manifests and current task artifacts in the implement agent.

Do not modify production code while the task status is `planning`.

## 3. Ordered implementation

### Phase A - Baseline and generated-code boundary

- [ ] Record `git status --short` and preserve unrelated untracked/user changes.
- [ ] Complete a source-wide inventory of account selectors, direct account-ID affinity lookups, provider loops, and already-pinned in-flight account resolvers before editing code; persist the traffic/non-traffic classification in `research/repository-map.md`.
- [ ] Run focused existing scheduler/admin/frontend tests before changes to establish a baseline.
- [ ] Confirm migration `196` is still unused on the implementation branch; renumber if upstream added it.
- [ ] Confirm generated Ent changes are produced only through `make generate` from `backend/` (`go generate ./ent` is the Ent step).

Review gate: baseline failures are recorded before continuing, and every discovered traffic selector is assigned to a later phase before the shared primitive or central scheduler code changes.

### Phase B - Persisted role and cross-layer contracts

- [ ] Add `is_fallback` default false and internal `pool_revision` default zero to `backend/ent/schema/account.go`.
- [ ] Add the idempotent SQL migration and active-row filter index under `backend/migrations/`.
- [ ] Run Ent generation and inspect generated schema, predicates, create/update builders, mutations, and runtime defaults.
- [ ] Add `IsFallback` and internal `PoolRevision` to `backend/internal/service/account.go` and Ent/service mapping in `account_repo.go`; expose only `is_fallback` through admin DTOs.
- [ ] Serialize both fields in full/metadata scheduler account payloads.
- [ ] Add one shared `formatPoolRevision` helper that encodes every revision as exactly 20 zero-padded decimal characters; Redis/Lua compares only that text without numeric conversion.
- [ ] Implement atomic `PublishAccountAtPoolRevision` with explicit rebuild versus authoritative direct/outbox writer modes, plus revision-preserving `InvalidateAccountAtPoolRevision`.
- [ ] Make equal-revision rebuilds no-op against published/tombstone state; allow only a fresh authoritative direct/outbox read to refresh equal published data or repair an equal tombstone.
- [ ] Keep tombstone revisions through transient invalidation so delayed rebuilds cannot resurrect stale role data; define permanent deletion cleanup only after existing membership fencing.
- [ ] Make revision-less old cache entries readable as published revision zero.

Tests:

- [ ] Ent/service mapper and DTO mapper return false/true correctly while keeping `pool_revision` internal.
- [ ] Migration is additive/idempotent and defaults old rows to primary/revision zero.
- [ ] Cache round trip covers true and legacy missing-field/revision-zero payloads.
- [ ] Real-Redis tests cover atomic full/metadata publication, zero/max-int64 20-character encoding, lexical ordering, stale-revision rejection, equal-revision rebuild no-op, fenced invalidation, authoritative equal-revision tombstone repair, and permanent-deletion cleanup.

Review gate: all account representations agree on one boolean name and default before mutation or scheduler work begins.

### Phase C - Account mutation, import/copy, list filtering, and invalidation

- [ ] Propagate the role through standard create, update (`*bool`), and bulk update (`*bool`) handler/service/repository inputs.
- [ ] Ensure every explicit role assignment atomically sets `is_fallback` and increments `pool_revision = pool_revision + 1` under the account row lock; return/read the committed revision before cache publication. Omission leaves both unchanged.
- [ ] Keep the DB mutation and account-change/bulk-change outbox event atomic, then synchronously publish every affected shared snapshot before reporting role-edit success.
- [ ] If publication fails, use `InvalidateAccountAtPoolRevision` and report success only when the fenced tombstone is installed; otherwise return a retriable error and leave the outbox event for repair.
- [ ] Ensure decisions that captured candidates before publication may finish, while a second scheduler instance starting after publication sees the new role.
- [ ] Copy the role in `DuplicateAccount`.
- [ ] Inherit the parent role in `CreateShadow`; leave future parent/shadow updates independent.
- [ ] Add the field to backup `DataAccount` export/import; old backups default primary.
- [ ] Propagate it through specialized creation requests used by `CreateAccountModal` and import surfaces, including Codex session import, ChatGPT session/DataAccount import, Codex PAT, and direct OAuth-token-to-account branches; direct/internal requests that omit it remain primary.
- [ ] Add validated list filter `pool=primary|fallback` through handler, service, repository predicates, filtered bulk targets, filtered liveness checks, scheduler-score filtering, and `buildAccountsListETag`.
- [ ] Keep schedulable/discovery repository queries unfiltered by role.

Tests:

- [ ] Standard create omitted/false/true at revision zero; update true-to-false and same-value explicit update each increment revision; omitted update does not.
- [ ] Concurrent/reverse-commit-order role updates remain strictly ordered independent of application/PostgreSQL timestamps.
- [ ] Bulk explicit IDs and filtered target atomically increment each row's revision; all-target publication/tombstone before success; partial publication repair.
- [ ] Cross-instance recovery, stale publication after tombstone, equal-revision rebuild no-op, authoritative outbox repair, and delayed bucket-rebuild regression tests use the shared pool revision fence.
- [ ] Duplicate preservation, shadow one-time inheritance/independence, backup round trip, and old backup default.
- [ ] List all/primary/fallback plus invalid filter 400 and distinct ETags.
- [ ] Specialized create payload propagation/defaults.

Review gate: an administrator API client can configure, query, copy, export, and clear the role without any scheduler implementation assumptions.

### Phase D - Shared strict-pool primitive

- [ ] Add the package-private stable partition/preferred-role helper from `design.md`.
- [ ] Add table-driven tests for primary preference, fallback-only, empty input, stable order, and nil items being skipped without blocking fallback.
- [ ] Add a small selected-role validation helper for fresh hydration/DB rechecks where reuse is real.

Review gate: every scheduler integration uses this helper or an equivalent reviewed wrapper; no selector reimplements “fallback when no primary” with priority values.

### Phase E - Generic gateway and mixed/composite scheduling

- [ ] Reorder generic legacy selection so all request eligibility precedes role partition and all routing/sticky/LRU preference follows it.
- [ ] Reorder load-aware selection with the same boundary.
- [ ] Keep model-routing preference inside the selected role.
- [ ] Apply exclusions before role partition on every retry.
- [ ] Keep slot probing and wait-plan creation inside the selected role; do not classify full concurrency as ineligible.
- [ ] Recompute eligibility after non-concurrency session/profit/capability rejection.
- [ ] Carry selected role through final account hydration, reread the current shared snapshot, release any acquired slot on role drift, and restart selection.
- [ ] Cover Anthropic, Gemini, Antigravity, forced platform, simple mode, mixed mode, and composite routing.

Focused tests:

- [ ] Primary/fallback/recovery plus exact existing model-not-found, temporarily-unavailable, and no-account error behavior for legacy and load-aware paths.
- [ ] Model-ineligible and excluded primary allow fallback.
- [ ] Fallback sticky/model-route cannot override primary.
- [ ] All primary slots full returns a primary wait plan.
- [ ] Composite/forced-platform constraints are applied before partition.

Rollback point: this is the first central traffic-routing change. If tests expose behavior drift, revert this phase as a unit while retaining only unexposed contract work on the branch.

### Phase F - OpenAI and Grok scheduling

- [ ] Partition complete eligible candidates before sticky and preference logic in the legacy OpenAI-compatible selector.
- [ ] Refactor advanced scheduler selection so previous-response, session sticky, subscription priority, scoring, top-K, weighted selection, cost preference, slot probing, and waiting all use one chosen role.
- [ ] Normalize scores within that role only.
- [ ] Validate role during fresh cache/DB recheck, release any acquired slot on drift, and retry inside a newly computed boundary.
- [ ] Rework `previous_response_id` resolution to compare a bound account with the current preferred request role; retain temporarily bypassed fallback bindings.
- [ ] Treat Grok video generation as normal strict-pool allocation, but resolve video status/content through `ResolveGrokMediaVideoRequestAccount` as an owner-scoped pinned resource; preserve group/capability/credentials/transport/status and bound-account slot admission without scheduling an alternate account or producing a false 404 after recovery.
- [ ] Apply the same strict rule to new WebSocket selection while leaving already-open connections untouched.
- [ ] Add pool role/count fields to existing debug decision data where available.

Focused tests:

- [ ] OpenAI and Grok primary/fallback/recovery plus exact existing no-account/error diagnosis.
- [ ] Compact, endpoint/image capability, transport, parent health, runtime block, quota, channel, and profit mismatch cases.
- [ ] Subscription fallback cannot outrank regular primary.
- [ ] Weighted sticky and previous-response fallback are bypassed after recovery.
- [ ] A video generated on fallback remains queryable for status/content through that bound account after primary recovery; a new video generation selects primary.
- [ ] Retry exclusions allow fallback only after primaries are exhausted.
- [ ] Primary concurrency saturation returns a primary wait plan.
- [ ] Fresh role drift restarts rather than returning the wrong role.

### Phase G - Gemini compatibility and batch image

- [ ] Move Gemini Messages sticky selection after rate-limit/request filtering and role partition.
- [ ] Partition AI Studio endpoint-capable accounts before rank/priority/LRU.
- [ ] Carry the selected role through Gemini/AI Studio final hydration and restart when the chosen account's current role drifts.
- [ ] Build batch-image pair eligibility from persistent schedulability, model mapping, provider-specific credential/type checks, and provider/model pricing before partition; move any other deterministic post-selection rejection into pair construction.
- [ ] Build automatic `(provider, account)` pairs across providers, partition globally, then retain existing order inside the chosen role.
- [ ] Keep model discovery and provider availability aware of both roles.

Focused tests:

- [ ] Gemini Messages primary/fallback/recovery, sticky recovery, model mismatch, exclusions, rate-limit precheck, mixed platform, and exact existing failure diagnosis.
- [ ] AI Studio incompatible primary does not block capable fallback.
- [ ] Batch-image automatic provider selection prefers any eligible primary over earlier-provider fallback.
- [ ] Batch-image invalid API-key/service-account credentials and unavailable provider pricing do not let an unusable primary block fallback; existing pricing/no-account errors remain distinguishable.
- [ ] Explicit provider still fails over within that provider.

Review gate: rerun the Phase A selector/direct-ID inventory and diff the classification. Every new or changed result must be integrated or documented as discovery, maintenance, or an already-pinned in-flight account path.

### Phase H - Administrator frontend

- [ ] Extend `Account`, create/update/bulk request types and list filter types in `frontend/src/types/index.ts` and `frontend/src/api/admin/accounts.ts`.
- [ ] Add an off-by-default fallback switch to create and initialize/send it in every persistence branch.
- [ ] Add the switch to edit with explicit clearing support.
- [ ] Add opt-in Primary/Fallback bulk edit for ID and filtered modes.
- [ ] Add the default-visible Pool column and role badges.
- [ ] Add All/Primary/Fallback filter state, API query propagation, URL/filter snapshot behavior, local row reconciliation, liveness request, and reset behavior.
- [ ] Add read-only role badges to group account search results and selected model-routing chips.
- [ ] Add English and Chinese i18n strings.
- [ ] Verify compact/desktop and mobile layouts have no text/control overlap.

Tests:

- [ ] Create default and every specialized payload branch.
- [ ] Edit initialization and fallback-to-primary clearing.
- [ ] Bulk explicit-ID and filtered-target payloads.
- [ ] List filter/reset/API/URL/ETag-facing request behavior and local reconciliation.
- [ ] Pool column badges and group read-only badges.

## 4. Validation commands

Run focused tests during each phase, then the complete gates from repository roots:

```bash
cd backend
make generate
gofmt -w <changed non-generated Go files>
go test ./internal/repository ./internal/handler/admin ./internal/service
go test ./...
golangci-lint run ./...
```

If `golangci-lint` is not installed locally, use the repository's documented CI/container command and report the limitation rather than skipping silently.

```bash
cd frontend
pnpm test:run -- src/components/account src/components/admin/account src/views/admin
pnpm typecheck
pnpm lint:check
pnpm test:run
pnpm build
```

Also run:

```bash
cmd.exe /c py .trellis/scripts/task.py validate fallback-account-pool

git diff --check
git status --short
git diff --stat
```

Do not run lint in fix mode as the final verification command; avoid unrelated formatting churn.

## 5. Final cross-layer review

Before `trellis-check` signs off:

- [ ] Trace one primary, one fallback, and one recovered-primary request through every selector family.
- [ ] Confirm group/platform/model/endpoint filtering happens before role partition.
- [ ] Confirm scoring/sticky/routing happens after role partition.
- [ ] Confirm concurrency is tested after role selection and all-primary-full waits on primary.
- [ ] Confirm retry exclusions are applied before each fresh partition.
- [ ] Confirm direct account ID paths cannot bypass the boundary.
- [ ] Confirm discovery queries and snapshot buckets still contain both roles.
- [ ] Confirm role-edit success follows atomic shared publication or revision-tombstone invalidation, stale/equal revision rules cannot regress the role, and two scheduler instances agree after the visibility point.
- [ ] Confirm old rows/cache/backups and omitted API fields remain primary.
- [ ] Confirm admin create/edit/bulk/list/group views expose one global role with no group override.
- [ ] Confirm generated Ent changes and migration number are correct and no unrelated user files changed.

## 6. Principal risks and rollback points

- **Selector ordering regression:** central scheduling functions are high blast radius. Keep path-specific deterministic tests and review each selector independently.
- **Hidden direct selector:** repeat repository-wide symbol/search audit after implementation and during check.
- **Concurrency mistaken for eligibility:** assert wait-plan account IDs/roles, not only successful selection.
- **Stale role cache:** use database-monotonic `pool_revision`, atomic full/metadata publication, and revision-preserving tombstones; verify reverse commit order, stale rebuild rejection, equal-revision repair, cross-instance visibility, and outbox recovery.
- **Partial API propagation:** specialized create/import paths and filtered bulk/liveness snapshots need explicit tests.
- **Generated-code churn:** regenerate once after schema stabilization and reject unrelated generated diffs.
- **Operational rollback:** clear `is_fallback` on all accounts or deploy previous code; retain additive role/revision columns and index.

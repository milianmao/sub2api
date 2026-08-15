# Fallback Account Pool Contract

## 1. Scope / Trigger

This contract applies when a request allocates a new upstream account. It covers the account data model, scheduler snapshots, admin mutations, and every selector. It does not reroute an already-created upstream resource that is explicitly owner-bound.

## 2. Signatures

- Database: `accounts.is_fallback BOOLEAN NOT NULL DEFAULT FALSE` and `accounts.pool_revision BIGINT NOT NULL DEFAULT 0`.
- Admin account create/update/bulk payload: `is_fallback` is the public boolean. Update and bulk update use `*bool`: omission preserves the value; `false` explicitly clears fallback membership.
- List query: `pool` accepts `primary`, `fallback`, or omission for both. Invalid input is a structured 400.
- Scheduler cache payloads contain `is_fallback` and internal `pool_revision`; legacy payloads decode to `false` and `0`.

## 3. Contracts

`is_fallback=false` is the primary pool. A selector must apply all request-specific non-concurrency eligibility checks first, partition the remaining candidates by `IsFallback`, then run every preference only in the preferred role:

```go
eligible := filterRequestEligible(candidates)
selected, role := service.PreferAccountPool(eligible, func(candidate Candidate) *service.Account {
    return candidate.Account
})
// Route/sticky/score/LRU/cost/slot wait logic only receives selected.
```

A request-eligible primary with no concurrency slot remains in the selected role and follows its existing wait/queue behavior. It does not activate fallback capacity. Retry exclusions are part of request eligibility and require a fresh partition.

Every explicit role assignment increments `pool_revision` in the same DB mutation. Cache writes encode revisions as fixed-width, 20-digit decimal text and atomically publish full and metadata snapshots. A stale writer cannot replace a newer payload or tombstone. Equal-revision rebuilds are no-ops; only a DB-authoritative direct/outbox repair may replace an equal-revision tombstone.

## 4. Validation & Error Matrix

| Condition | Required behavior |
| --- | --- |
| Matching eligible primary exists | Select only primary candidates. |
| No matching eligible primary; fallback exists | Select fallback candidates. |
| Neither role has a match | Preserve existing no-account/model diagnosis. |
| Primary is full only | Return the existing primary wait plan. |
| Role publication fails | Install a fenced tombstone or return retriable mutation failure; never report routing-visible success otherwise. |
| Existing cache/backup row lacks fields | Interpret as primary, revision zero. |

## 5. Good/Base/Bad Cases

- Good: a primary that lacks the requested model does not block a fallback that supports it.
- Base: deployments with no `is_fallback=true` accounts preserve current scheduling.
- Bad: applying sticky, routing IDs, previous-response affinity, or weighted scoring before role partition. These direct paths can bypass primary recovery.

The Grok media status/content continuation is intentionally different: resolve its account-owner binding directly and never schedule a replacement account. New video generation remains a normal allocation.

## 6. Tests Required

- Account pool helper: stable primary selection, fallback-only, empty input, nil account skipping, and recovery on the next decision.
- Cache: fixed-width revision encoding; stale publication rejection; tombstone protection; equal-revision rebuild no-op; authoritative equal-revision repair; delayed equal invalidation no-op.
- Selectors: primary, fallback-only, recovery, request-ineligible primary, retry exclusions, and primary-full wait behavior for generic, OpenAI/Grok, Gemini/AI Studio, and batch-image paths.
- Mutation/API: create default/explicit role, explicit clear, revision increment, duplicate/shadow/backup behavior, and `pool` list filter/ETag.

## 7. Wrong vs Correct

### Wrong

```go
if account.HasFreeConcurrency() {
    primary = append(primary, account)
}
if len(primary) == 0 {
    return fallback
}
```

This treats saturation as eligibility failure and makes fallback an overflow pool.

### Correct

```go
roleCandidates, role := PreferAccountPool(requestEligible)
return acquireOrBuildWaitPlan(roleCandidates, role)
```

Role selection uses full request eligibility but excludes concurrency availability; capacity admission stays within the chosen role.

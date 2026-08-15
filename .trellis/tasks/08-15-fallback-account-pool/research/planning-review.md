# Planning review resolutions

## Review scope

A read-only planning review checked `prd.md`, `design.md`, `implement.md`, the repository map, and the two context manifests. No production files were changed.

## Resolved findings

### Scheduler visibility and stale writers

The initial design treated database commit plus best-effort account snapshot refresh as sufficient. That left a commit-to-publish window and allowed a delayed bucket rebuild to overwrite a newer role.

Resolution after two review passes:

- define routing visibility at shared account snapshot publication, not raw DB commit;
- add internal `accounts.pool_revision BIGINT NOT NULL DEFAULT 0` because `UpdatedAt` is not commit-monotonic across application clocks or PostgreSQL transaction start times;
- atomically increment `pool_revision` with every explicit role assignment under the account row lock;
- encode every revision through one helper as exactly 20 zero-padded decimal characters so Redis Lua never performs lossy numeric conversion or mixed-width comparison;
- atomically publish full payload, metadata payload, and a published revision state;
- on publish failure, remove payloads only through `InvalidateAccountAtPoolRevision`, which retains/advances a revision tombstone;
- reject older writes; make equal-revision rebuilds no-op, while permitting only a fresh authoritative direct/outbox read to refresh equal published data or repair an equal tombstone;
- do not report role-mutation success until all affected accounts are published or fenced-invalidated;
- reread the selected account's role before return and restart/release any slot on drift;
- test reverse commit order, stale writes after publication and tombstone invalidation, equal-revision repair, partial bulk publication, and two scheduler instances.

### Selector inventory timing

The initial implementation plan deferred the source-wide direct-selector audit until after central scheduler edits.

Resolution: move the first audit to Phase A, persist the traffic/non-traffic classification in `research/repository-map.md`, include both handlers and service declarations, and repeat the audit after selector changes.

### Grok pinned video lookup

The second review found `ResolveGrokMediaVideoRequestAccount`: video status/content is bound to the account that created the upstream resource, but the current handler runs normal scheduling and returns a false 404 if recovery selects another account.

Resolution: classify new Grok media generation as strict-pool allocation, but classify owner-scoped status/content lookup as continuation of a pinned in-flight resource. Resolve the bound account directly, never migrate the lookup, and test fallback creation followed by primary recovery. `previous_response_id` remains strict-failback by explicit requirement.

### Batch-image eligibility

The initial design named only schedulability, model support, and `SupportsAccount`.

Resolution: explicitly include request-global admission, provider/model pricing, provider-specific account type and credential validation, and any other deterministic post-selection rejection before pair partitioning. Preserve pricing versus no-account diagnosis.

### Failure diagnosis

Resolution: every selector family must assert the existing model-not-found versus temporarily-unavailable/no-account error identity or status, not only that selection fails.

### Nil helper semantics

Resolution: `partitionAccountPool` skips items whose `accountOf` result is nil. Nil can never count as primary or block an eligible fallback. Normal callers still provide only request-eligible non-nil items.

### Frontend focused tests

Resolution: the focused command covers `src/components/account`, `src/components/admin/account`, and `src/views/admin` before the full Vitest run.

## Final focused review

A final read-only review found no remaining blocking or high-priority planning issues. It confirmed the database-monotonic revision, published/tombstone repair protocol, and Grok pinned-resource split are coherent and explicitly tested. Its implementation watch items are now encoded in the design: one fixed-width revision formatter, authority-gated equal-revision repair, and preservation of all Grok bound-account validation/admission checks.

## Remaining review gate

The task remains in `planning`. Run `task.py validate fallback-account-pool` after these revisions, then request user review. Do not run `task.py start` until the user explicitly approves implementation.

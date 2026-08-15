# Add fallback account pool

## Goal

Add a global fallback role for accounts while preserving strict priority for the existing primary account pool. Fallback accounts receive new traffic only when the current request has no usable matching primary account, and every later scheduling decision returns to primary as soon as one becomes usable.

## Background

The existing account pool remains the primary pool. This feature provides failover capacity, not load sharing, traffic splitting, or peak-time overflow.

Repository evidence:

- Persistent and time-based schedulability already includes active status, the schedulable switch, expiry, overload, rate limits, temporary blocking, and quota state (`backend/internal/service/account.go:152`).
- Final eligibility is request-specific and additionally checks group/platform membership, model support, endpoint capability, runtime blocks, parent-account health, transport, channel restrictions, and profit controls (`backend/internal/service/openai_gateway_scheduling.go:252`, `backend/internal/service/openai_account_scheduler.go:1313`).
- Candidate pools are bounded by resolved group and platform, with distinct selectors for the generic gateway, OpenAI-compatible gateway, Gemini compatibility service, and batch-image compatibility service (`backend/internal/service/gateway_scheduling.go:960`, `backend/internal/service/openai_gateway_scheduling.go:1205`, `backend/internal/service/gemini_messages_compat_service.go:446`, `backend/internal/service/batch_image_public.go:936`).
- Existing account priority is not a strict fallback boundary. The advanced OpenAI scheduler combines priority with load, queue depth, error rate, latency, reset windows, quota headroom, and cost (`backend/internal/service/openai_account_scheduler.go:956`).
- Existing group fallback fields have unrelated meanings for non-Claude-Code routing and invalid-request/prompt-too-long routing; they cannot be reinterpreted as availability failover (`backend/migrations/029_add_group_claude_code_restriction.sql:8`, `backend/migrations/043b_add_group_invalid_request_fallback.sql:4`).
- Accounts can belong to multiple groups, while each account-group binding stores only binding-local priority (`backend/ent/schema/account_group.go:17`). The pool role therefore belongs to the account itself.

## Requirements

- **R1 - Explicit global membership:** Each account has an administrator-controlled primary/fallback role that applies in every group and in ungrouped/simple-mode scheduling. Existing accounts and ordinary new accounts default to primary. Account duplication and backup export/import preserve the role. A newly generated Spark shadow inherits its parent's role once at creation and is independently editable afterward.
- **R2 - Strict primary priority:** Every account-selection decision must choose only from matching usable primary accounts whenever that set is non-empty. Existing routing preferences, priority, scoring, randomization, LRU, cost preference, and stickiness operate only inside the selected role.
- **R3 - Request-specific failover:** Fallback accounts may be considered only after applying the current attempt's group, platform, model, endpoint, credentials, transport, channel, profit, runtime-state, parent-health, and exclusion constraints and finding no usable primary candidate.
- **R4 - Automatic recovery:** A decision that establishes its request-eligibility snapshot after any matching primary account's role/status change becomes routing-visible must stop selecting fallback accounts. A mutation racing a decision that already established that snapshot affects the next decision; in-flight requests are not migrated.
- **R5 - Existing constraints:** Account type, group, platform, model mapping, endpoint capability, credentials, status, schedulability, rate limits, overload, temporary blocks, quota, privacy, channel restrictions, transport, parent health, profit controls, and existing admission rules continue to apply.
- **R6 - Existing failure behavior:** If neither role contains a usable matching account, callers retain the current no-account/error result, including existing model-not-found versus temporarily-unavailable diagnosis.
- **R7 - Backward compatibility:** Deployments with no explicitly marked fallback accounts preserve current scheduling behavior. Old database rows, old backup payloads, and old scheduler-cache JSON that omit the role/revision are interpreted as primary at revision zero.
- **R8 - Complete selection coverage:** The strict boundary applies to Anthropic, Gemini, Antigravity, OpenAI, Grok, mixed-platform and composite routing, generic and advanced schedulers, Gemini AI Studio compatibility selection, OpenAI previous-response selection, and batch-image provider/account selection. Discovery, capacity, and model-availability queries continue to see both roles.
- **R9 - Capacity semantics:** Free concurrency is not part of role eligibility. A request-eligible primary account with no free slot keeps the request on the existing primary wait/queue path and never activates fallback capacity. A non-concurrency admission rejection may exclude that account and recompute the boundary under existing retry behavior.
- **R10 - Sticky sessions, direct routes, retries, and pinned resources:** Every new allocation attempt re-establishes the boundary using that attempt's exclusions. A fallback session binding, model-routing ID, weighted sticky bonus, or `previous_response_id` binding cannot override an eligible primary account. A retry may enter fallback only after no non-excluded matching primary remains. Operations on an already-created upstream resource whose state is account-local, such as Grok video status/content lookup, continue on the account bound when that resource was created and are not new allocation attempts. Existing requests/resources already executing on fallback are not migrated.
- **R11 - Administrator controls and visibility:** Administrators can assign or clear the role in account create, edit, and bulk-edit flows; identify it in account lists; filter lists by primary/fallback; and see it in group-side account selectors/views as read-only account metadata. No group-level role override is introduced.

## Acceptance Criteria

- [ ] Repeated decisions select no fallback account while at least one matching primary account is usable.
- [ ] When no matching primary is usable and a matching fallback is usable, selection can use fallback.
- [ ] After fallback use begins, making any matching primary usable and completing scheduler-state publication causes the next decision to select only from primary.
- [ ] Role-only edits do not report success before the shared scheduler account snapshot is published or invalidated; delayed snapshot rebuilds cannot overwrite a newer role.
- [ ] A primary that is ineligible for this request, including model, endpoint, transport, parent-health, channel, profit, or retry-exclusion mismatch, does not block an eligible fallback.
- [ ] If neither role has a usable matching account, callers receive the existing failure result.
- [ ] Existing rows, old cache payloads, old backups, and newly created accounts behave as primary unless explicitly marked otherwise.
- [ ] Changing one account's role affects all of its groups and ungrouped/simple-mode routing without changing its group bindings.
- [ ] Duplicate accounts and backup round trips preserve the source role; newly created Spark shadows inherit the parent role once and can later be changed independently.
- [ ] A request-eligible primary with no free concurrency slot returns the existing primary wait/queue behavior instead of using fallback.
- [ ] A session bound to fallback returns to an eligible primary on its next scheduling decision after recovery.
- [ ] Model routing, weighted sticky selection, and `previous_response_id` cannot select fallback while an eligible primary exists.
- [ ] Retry exclusions activate fallback only after all matching non-excluded primary candidates are ineligible.
- [ ] A Grok video created on fallback remains queryable through its bound fallback account after primary recovery; new Grok media generation uses the recovered primary.
- [ ] Create, edit, and bulk-edit APIs and UI can assign and clear the role, including filtered bulk edit.
- [ ] Account lists expose and filter the role; group-side account choices show the same global role without an override.
- [ ] Strict primary-first tests cover Anthropic, Gemini, Antigravity, OpenAI, Grok, mixed/composite routing, Gemini AI Studio, OpenAI previous-response paths, and batch-image selection.
- [ ] Automated tests cover primary preference, failover, recovery, request-specific eligibility, concurrency waiting, sticky/direct-path recovery, retry exclusions, cache/default compatibility, and no-account behavior.

## Out of Scope

- Load sharing, weighted traffic splitting, or peak-capacity overflow into fallback while a matching primary is usable.
- Migrating requests that are already in flight or moving an already-open upstream WebSocket connection between accounts.
- Replacing or redesigning upstream health checks beyond consuming their existing account eligibility state.
- A group-specific fallback role or reinterpretation of existing fallback-group settings.

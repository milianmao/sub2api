# Repository map: fallback account pool

## Purpose

This note records the repository evidence used by the fallback-account-pool design. It is implementation context, not a replacement for `prd.md` or `design.md`.

## Persisted account contract

- `backend/ent/schema/account.go` defines account-owned scheduling fields. A global pool role belongs here, not on `account_groups`, because one account can belong to several groups.
- `backend/ent/schema/account_group.go` stores only binding-local priority. It must not gain a fallback override.
- `backend/internal/service/account.go` is the service model and owns schedulability/model helpers. It will carry the account role plus an internal monotonic pool revision used only for cache fencing.
- `backend/internal/repository/account_repo.go` maps Ent accounts to service accounts, creates/updates/bulk-updates accounts, implements schedulable candidate queries, and immediately synchronizes per-account scheduler snapshots after committed updates.
- `backend/internal/service/scheduler_snapshot_service.go` keys buckets by group/platform/mode. Buckets retain account IDs, while account details are fetched from the per-account scheduler cache. The pool role is account metadata, not a new bucket dimension.
- `backend/internal/repository/scheduler_cache.go` serializes the service account snapshot. Missing role/revision fields in old Redis JSON naturally decode to primary/revision zero.
- `backend/migrations/migrations.go` embeds every SQL migration. The next repository migration number observed during planning is `196` (latest existing migration: `195_add_usage_log_upstream_model_mismatch_index_notx.sql`). The additive migration needs both `is_fallback` and internal `pool_revision`.
- `backend/Makefile` regenerates Ent with `go generate ./ent` through `make generate`.

## Account mutation and admin surfaces

- Standard create/update/bulk request DTOs live in `backend/internal/handler/admin/account_handler.go`; service inputs live in `backend/internal/service/admin_service.go`; implementations live in `backend/internal/service/admin_account.go`.
- `buildAccountForCreate`, `CreateAccount`, `UpdateAccount`, `BulkUpdateAccounts`, and repository create/update/bulk paths must all propagate the role.
- `DuplicateAccount` in `backend/internal/service/admin_account.go` copies durable account configuration into a paused duplicate. The pool role should be copied with that configuration.
- `CreateShadow` in the same file creates a Spark shadow with independent priority, concurrency, status, and groups while inheriting selected parent defaults. It should inherit the parent's role at creation, then remain independently editable.
- Admin backup export/import uses `DataAccount` in `backend/internal/handler/admin/account_data.go`. Adding the role preserves fallback configuration across backups; old backups omit it and therefore import as primary.
- Specialized creation paths include standard account create, Codex session import (`account_codex_import.go`), ChatGPT session import (`account_chatgpt_session_import.go` via `DataAccount`), Codex PAT create (`openai_oauth_handler.go`), and direct platform import flows. Omitted role values must remain primary.
- `AccountFromService`/`AccountFromServiceShallow` in `backend/internal/handler/dto/mappers.go` and `dto.Account` in `backend/internal/handler/dto/types.go` are the public admin response boundary.
- Account list filtering flows through `AccountHandler.List`, `adminServiceImpl.ListAccounts`, and `accountRepository.accountListFilteredQuery`. Filter-based bulk edit and liveness checks reuse snapshots of these filters and must carry the new pool filter.
- `buildAccountsListETag` must include the pool filter as part of the response identity.

## Scheduler cache consistency

- `accountRepository.updateAccount` writes an account-change outbox event in the same transaction and currently calls `syncSchedulerAccountSnapshot` after a standalone transaction commits.
- Scheduler account keys live in shared Redis, so a successful write is visible to every application instance; cross-instance role visibility does not require role-specific bucket rebuilding.
- `GetSnapshot` hydrates bucket IDs from metadata account keys, while direct account paths use the full account key. Both payloads must expose the same role at one publication point.
- `writeAccountIDs` currently pipelines independent full/metadata `SET` operations without a per-account revision fence. A delayed rebuild can therefore overwrite a newer direct account write.
- `UpdatedAt` cannot be the fence: Ent uses application `time.Now()`, bulk SQL uses PostgreSQL `NOW()` (transaction-start time), and neither is monotonic by commit order across instances/blocked transactions.
- Add database `pool_revision`; every explicit role assignment performs `pool_revision = pool_revision + 1` under the row update lock. One shared helper emits exactly 20 zero-padded decimal characters, and Redis/Lua compares that text without numeric conversion.
- Normal publication atomically stores full/meta payloads plus a published revision. Failure invalidation removes payloads but retains a revision tombstone. Equal-revision rebuilds are no-ops; only a fresh authoritative direct/outbox read may refresh equal published data or repair an equal tombstone.
- Role mutation success occurs only after committed data is published or fenced-invalidated. The outbox remains durable repair and ordinary bucket-change propagation.
- Because both roles remain in the same bucket, role-only changes do not alter bucket membership or bucket keys.

## Account-selection entry points

### Generic gateway

`backend/internal/service/gateway_scheduling.go` contains:

- `SelectAccountForModelWithExclusions`
- `SelectAccountWithLoadAwareness`
- `selectAccountForModelWithPlatform`
- `selectAccountWithMixedScheduling`
- model-routing account IDs
- session sticky selection
- load-aware selection, slot acquisition, and wait-plan fallback

The current order can return routed or sticky accounts before evaluating all request-eligible candidates. It must be reorganized so pool selection happens after request eligibility and before routing/sticky/scoring. Concurrency load and slot availability must not remove an otherwise eligible primary account from the selected pool.

### OpenAI-compatible gateway

`backend/internal/service/openai_gateway_scheduling.go` contains legacy OpenAI-compatible selection for OpenAI and Grok, including sticky, compact capability, endpoint capability, parent health, runtime blocks, channel restrictions, DB rechecks, and wait plans.

`backend/internal/service/openai_account_scheduler.go` contains the advanced scheduler. It currently evaluates previous-response stickiness and session stickiness before the load-balanced candidate filter, then scores by priority, load, queue, error rate, latency, reset timing, quota headroom, upstream cost, and sticky bonuses. Strict pool partitioning must precede every one of those preferences.

`backend/internal/service/openai_ws_forwarder_support.go` resolves `previous_response_id` directly to an account. That direct binding must be accepted only when the bound account belongs to the currently preferred request pool. Otherwise it falls through to normal scheduling without deleting a still-valid binding.

### Gemini compatibility

`backend/internal/service/gemini_messages_compat_service.go` has two independent selectors:

- `SelectAccountForModelWithExclusions` with sticky, platform/mixed scheduling, model support, and rate-limit precheck.
- `SelectAccountForAIStudioEndpoints`, which ranks endpoint account types independently.

Both need request-eligible partitioning before sticky or rank/LRU preference. AI Studio-incompatible candidates (for example Vertex-only service accounts for `generativelanguage.googleapis.com`) must not block an eligible fallback account.

### Batch image compatibility

`backend/internal/service/batch_image_public.go` has an additional direct selector: `selectProviderAndAccount`. It loops Gemini API/Vertex providers and currently returns the first supported account. `GeminiAPIBatchImageProvider.SupportsAccount` validates Gemini/API-key type and a non-empty key; `VertexBatchImageProvider.SupportsAccount` validates Gemini/service-account type and parseable credentials. Provider/model unit pricing is resolved after selection today and must move into pair eligibility so an unpriced primary pair cannot suppress a usable fallback pair. Automatic selection must build all eligible `(provider, account)` pairs, partition globally, then retain provider and priority order inside the chosen pool.

### Non-selection queries

The following are discovery/diagnostic views, not traffic allocation, and must retain both roles:

- model availability diagnosis and model-list aggregation
- schedulable-platform discovery
- group capacity/counts
- batch-image public model discovery
- scheduler snapshot bucket construction

### Source-wide selection classification

The planning audit classifies these as new-traffic allocation and therefore in scope:

| Family | Entry points / direct affinity |
| --- | --- |
| Generic gateway | `GatewayService.SelectAccount*`, `selectAccountForModelWithPlatform`, `selectAccountWithMixedScheduling`, model-routing IDs, session sticky, load-aware slot/wait selection. |
| OpenAI/Grok legacy | `OpenAIGatewayService.SelectAccount*`, `selectAccountForModelWithExclusions`, `SelectAccountWithLoadAwareness`, sticky direct lookup, and new Grok media generation. |
| Grok pinned video | `ResolveGrokMediaVideoRequestAccount` status/content lookup is a continuation of an account-local upstream resource and must resolve the owner-bound account directly; it is not repartitioned after recovery. |
| OpenAI advanced | `defaultOpenAIAccountScheduler.Select`, `selectBySessionHash`, `selectByLoadBalance`, weighted sticky, subscription/cost/score order. |
| OpenAI response affinity | `resolveAccountByPreviousResponseIDForCapability` and the WS selector that calls it. |
| Gemini compatibility | `GeminiMessagesCompatService.SelectAccountForModelWithExclusions`, `selectBestGeminiAccount`, and `SelectAccountForAIStudioEndpoints`. |
| Batch image | `BatchImagePublicService.selectProviderAndAccount`, including automatic cross-provider ordering. |

The audit classifies these as not making a new traffic-allocation decision and therefore not role-filtered:

- `GatewayService.GetAvailableModels`, `GetSchedulablePlatforms`, model diagnosis, group capacity, and admin scheduler-score/candidate views are discovery or diagnostics.
- OAuth/token refresh candidate scans and `AccountTestService` are maintenance/admin probes, not user gateway allocation.
- `BatchImageProcessor`, `BatchImageDownloadService`, and Grok video status/content lookup resolve the account already pinned to an existing upstream resource; they are in-flight execution. The current Grok handler still schedules then compares IDs, so Phase F must remove that mismatch path to avoid a false 404 after primary recovery.
- Codex image bridge and OpenAI image upstream strategy consume an account selected by the OpenAI scheduler rather than selecting a second account.

Phase A repeats this inventory against the implementation branch, and Phase G repeats it after edits to detect newly added or hidden paths.

## Frontend contract

- Account types and request types are in `frontend/src/types/index.ts`.
- Account API filters and bulk payloads are in `frontend/src/api/admin/accounts.ts`.
- Main list/filter state and filter snapshots are in `frontend/src/views/admin/AccountsView.vue` and `frontend/src/components/admin/account/AccountTableFilters.vue`.
- Create/edit/bulk controls are in `CreateAccountModal.vue`, `EditAccountModal.vue`, and `BulkEditAccountModal.vue`.
- `GroupsView.vue` has account search/results and selected-account chips for model routing. It is the current group-side account view where the global role can be shown read-only; it must not offer a per-group override.
- English and Chinese account strings are in `frontend/src/i18n/locales/en/admin/accounts.ts` and `frontend/src/i18n/locales/zh/admin/accounts.ts`.

## Design conclusions

1. Persist `accounts.is_fallback BOOLEAN NOT NULL DEFAULT FALSE` and internal `accounts.pool_revision BIGINT NOT NULL DEFAULT 0`.
2. Keep both roles in repository candidate queries and scheduler buckets.
3. Filter complete request eligibility first, partition by role second, and run all preference/capacity logic only inside the chosen role.
4. Treat retry exclusions and terminal admission vetoes as request-local eligibility changes and recompute the role boundary.
5. Do not treat a full concurrency slot as ineligibility; return the existing primary wait plan.
6. Standard creation defaults to primary; duplicate and backup round trips preserve the role; Spark shadow creation inherits once and is independent thereafter.
7. Use API field `is_fallback` and account-list filter `pool=primary|fallback`.
8. Make shared account publication atomic and fence it with database-monotonic `pool_revision`; retain revision tombstones during invalidation so stale writers cannot restore old roles.

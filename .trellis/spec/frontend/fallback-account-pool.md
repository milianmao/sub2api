# Fallback Account Pool UI Contract

## 1. Scope / Trigger

The account pool role is a backend-facing account attribute. Every create, edit, bulk edit, list, and group-side account display must use the same account-owned value. Groups may display it but cannot override it.

## 2. Signatures

```ts
type Account = { is_fallback: boolean }
type AccountQueryFilters = { pool?: 'primary' | 'fallback' | '' }
type UpdateAccountRequest = { is_fallback?: boolean }
type BulkUpdateAccountsRequest = { is_fallback?: boolean }
```

`frontend/src/api/admin/accounts.ts` normalizes `pool: ''` to `undefined` before request serialization. It preserves `primary` and `fallback` in normal and ETag list requests.

## 3. Contracts

- Create defaults `form.is_fallback` to `false` and sends it from all persistence branches, including the shared specialized-create helper and direct OAuth/token import calls.
- Edit initializes from `account.is_fallback === true` and sends `false` when an administrator clears the switch.
- Bulk edit sends the field only when its opt-in control is enabled; both explicit-ID and filtered-target modes share this behavior.
- Account lists render an accessible role label and offer All/Primary/Fallback filtering.
- Group account search results and model-routing chips render a read-only role label. Missing legacy account payload metadata is normalized to primary (`false`).

## 4. Validation & Error Matrix

| UI/input state | Request behavior |
| --- | --- |
| New account, untouched switch | Send `is_fallback: false`. |
| Switch enabled | Send `is_fallback: true`. |
| Existing fallback switch cleared | Send `is_fallback: false`. |
| Bulk role option not enabled | Omit `is_fallback`. |
| List filter All (`''`) | Omit `pool`. |
| List filter Primary/Fallback | Send exact `pool` value. |

## 5. Good/Base/Bad Cases

- Good: a Bedrock, OAuth, service-account, or token-import create path carries `form.is_fallback` just as the API-key path does.
- Base: old list/account responses without `is_fallback` display Primary.
- Bad: showing a switch whose selected value is dropped by a specialized creation helper, or adding a group-level role input.

## 6. Tests Required

- Create default and enabled payloads, including the shared specialized-create helper.
- Edit fallback initialization and explicit clear.
- Bulk explicit-ID and filtered-target opt-in behavior.
- Filter serialization for normal and ETag list requests plus local list reconciliation.
- Primary/fallback labels in account filter/list and read-only group account views.

## 7. Wrong vs Correct

### Wrong

```ts
await adminAPI.accounts.create({ name, platform, credentials })
```

A specialized branch silently loses the administrator's selected role.

### Correct

```ts
await adminAPI.accounts.create({
  name,
  platform,
  credentials,
  is_fallback: form.is_fallback,
})
```

The value crosses each persistence boundary explicitly and defaults to `false` for old or omitted data.

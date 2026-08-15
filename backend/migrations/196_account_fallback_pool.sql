-- Add a global fallback role and an internal monotonic cache-publication fence.
-- Both defaults preserve existing accounts as primary accounts.
ALTER TABLE accounts
ADD COLUMN IF NOT EXISTS is_fallback BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE accounts
ADD COLUMN IF NOT EXISTS pool_revision BIGINT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_accounts_is_fallback_active
ON accounts (is_fallback)
WHERE deleted_at IS NULL;

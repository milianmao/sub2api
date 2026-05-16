-- Add user/group authorization fields for group access control.
ALTER TABLE users
	ADD COLUMN IF NOT EXISTS level INTEGER NOT NULL DEFAULT 0;

ALTER TABLE groups
	ADD COLUMN IF NOT EXISTS access_mode VARCHAR(20) NOT NULL DEFAULT 'public',
	ADD COLUMN IF NOT EXISTS min_user_level INTEGER NOT NULL DEFAULT 0;

UPDATE groups
SET access_mode = 'restricted'
WHERE is_exclusive = TRUE
  AND (access_mode IS NULL OR access_mode = '' OR access_mode = 'public');

UPDATE groups
SET access_mode = 'public'
WHERE access_mode IS NULL OR access_mode = '';

UPDATE groups
SET min_user_level = 0
WHERE min_user_level IS NULL;

ALTER TABLE users
	DROP CONSTRAINT IF EXISTS users_level_non_negative,
	ADD CONSTRAINT users_level_non_negative CHECK (level >= 0);

ALTER TABLE groups
	DROP CONSTRAINT IF EXISTS groups_access_mode_check,
	ADD CONSTRAINT groups_access_mode_check CHECK (access_mode IN ('public', 'restricted')),
	DROP CONSTRAINT IF EXISTS groups_min_user_level_non_negative,
	ADD CONSTRAINT groups_min_user_level_non_negative CHECK (min_user_level >= 0);

CREATE INDEX IF NOT EXISTS idx_users_level ON users(level);
CREATE INDEX IF NOT EXISTS idx_groups_access_mode ON groups(access_mode);
CREATE INDEX IF NOT EXISTS idx_groups_min_user_level ON groups(min_user_level);

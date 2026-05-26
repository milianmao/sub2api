//go:build integration

package repository

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyGroupsMigrationSkipsLegacyOrphanGroupIDs(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()

	preMigrationPath := filepath.Join("..", "..", "migrations", "141b_clear_orphan_api_key_group_ids.sql")
	preMigrationSQL, err := os.ReadFile(preMigrationPath)
	require.NoError(t, err)

	migrationPath := filepath.Join("..", "..", "migrations", "142_add_api_key_groups.sql")
	migrationSQL, err := os.ReadFile(migrationPath)
	require.NoError(t, err)

	prepareAPIKeyGroupsLegacyFixture(t, tx, ctx)

	var userID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO users (email, password_hash, role, status, balance, concurrency)
VALUES ('api-key-groups-migration@example.com', 'hash', 'user', 'active', 0, 1)
RETURNING id
`).Scan(&userID))

	var validGroupID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO groups (name, platform, status)
VALUES ('api-key-groups-valid', 'openai', 'active')
RETURNING id
`).Scan(&validGroupID))

	var validKeyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO api_keys (user_id, key, name, group_id, status)
VALUES ($1, 'sk-api-key-groups-valid', 'valid key', $2, 'active')
RETURNING id
`, userID, validGroupID).Scan(&validKeyID))

	const orphanGroupID int64 = 999999991
	var orphanKeyID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
INSERT INTO api_keys (user_id, key, name, group_id, status)
VALUES ($1, 'sk-api-key-groups-orphan', 'orphan key', $2, 'active')
RETURNING id
`, userID, orphanGroupID).Scan(&orphanKeyID))

	_, err = tx.ExecContext(ctx, string(preMigrationSQL))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)

	var backfilledValidGroupID int64
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT group_id
FROM api_key_groups
WHERE api_key_id = $1
`, validKeyID).Scan(&backfilledValidGroupID))
	require.Equal(t, validGroupID, backfilledValidGroupID)

	var orphanBackfillCount int
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM api_key_groups
WHERE api_key_id = $1
`, orphanKeyID).Scan(&orphanBackfillCount))
	require.Zero(t, orphanBackfillCount)

	var retainedGroupID sql.NullInt64
	require.NoError(t, tx.QueryRowContext(ctx, `
SELECT group_id
FROM api_keys
WHERE id = $1
`, orphanKeyID).Scan(&retainedGroupID))
	require.False(t, retainedGroupID.Valid)
}

func prepareAPIKeyGroupsLegacyFixture(t *testing.T, tx *sql.Tx, ctx context.Context) {
	t.Helper()

	_, err := tx.ExecContext(ctx, `
DROP TABLE IF EXISTS api_key_groups;
ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS api_keys_group_id_fkey;
TRUNCATE TABLE
	api_keys,
	users,
	groups
RESTART IDENTITY CASCADE;
`)
	require.NoError(t, err)
}

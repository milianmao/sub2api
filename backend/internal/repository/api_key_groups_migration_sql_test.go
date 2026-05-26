package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyGroupsPreMigrationClearsLegacyOrphanGroupIDs(t *testing.T) {
	migrationPath := filepath.Join("..", "..", "migrations", "141b_clear_orphan_api_key_group_ids.sql")
	migrationSQL, err := os.ReadFile(migrationPath)
	require.NoError(t, err)

	normalized := strings.Join(strings.Fields(strings.ToLower(string(migrationSQL))), " ")

	require.Contains(t, normalized, "update api_keys k set group_id = null")
	require.Contains(t, normalized, "where k.group_id is not null")
	require.Contains(t, normalized, "not exists ( select 1 from groups g where g.id = k.group_id )")
}

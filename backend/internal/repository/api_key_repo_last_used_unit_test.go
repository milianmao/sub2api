package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newAPIKeyRepoSQLite(t *testing.T) (*apiKeyRepository, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:api_key_repo_last_used?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return &apiKeyRepository{client: client}, client
}

func mustCreateAPIKeyRepoUser(t *testing.T, ctx context.Context, client *dbent.Client, email string) *service.User {
	t.Helper()
	u, err := client.User.Create().
		SetEmail(email).
		SetPasswordHash("test-password-hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	return userEntityToService(u)
}

func mustCreateAPIKeyRepoGroup(t *testing.T, ctx context.Context, client *dbent.Client, name string) *service.Group {
	t.Helper()
	g, err := client.Group.Create().
		SetName(name).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	return groupEntityToService(g)
}

func TestAPIKeyRepository_CreateWithLastUsedAt(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "create-last-used@test.com")

	lastUsed := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	key := &service.APIKey{
		UserID:     user.ID,
		Key:        "sk-create-last-used",
		Name:       "CreateWithLastUsed",
		Status:     service.StatusActive,
		LastUsedAt: &lastUsed,
	}

	require.NoError(t, repo.Create(ctx, key))
	require.NotNil(t, key.LastUsedAt)
	require.WithinDuration(t, lastUsed, *key.LastUsedAt, time.Second)

	got, err := repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)
	require.WithinDuration(t, lastUsed, *got.LastUsedAt, time.Second)
}

func TestAPIKeyRepository_UpdateLastUsed(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "update-last-used@test.com")

	key := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-update-last-used",
		Name:   "UpdateLastUsed",
		Status: service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, key))

	before, err := repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.Nil(t, before.LastUsedAt)

	target := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)
	require.NoError(t, repo.UpdateLastUsed(ctx, key.ID, target))

	after, err := repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, after.LastUsedAt)
	require.WithinDuration(t, target, *after.LastUsedAt, time.Second)
	require.WithinDuration(t, target, after.UpdatedAt, time.Second)
}

func TestAPIKeyRepository_UpdateLastUsedDeletedKey(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "deleted-last-used@test.com")

	key := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-update-last-used-deleted",
		Name:   "UpdateLastUsedDeleted",
		Status: service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, key))
	require.NoError(t, repo.Delete(ctx, key.ID))

	err := repo.UpdateLastUsed(ctx, key.ID, time.Now().UTC())
	require.ErrorIs(t, err, service.ErrAPIKeyNotFound)
}

func TestAPIKeyRepository_UpdateLastUsedDBError(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "db-error-last-used@test.com")

	key := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-update-last-used-db-error",
		Name:   "UpdateLastUsedDBError",
		Status: service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, key))

	require.NoError(t, client.Close())
	err := repo.UpdateLastUsed(ctx, key.ID, time.Now().UTC())
	require.Error(t, err)
}

func TestAPIKeyRepository_CreateDuplicateKey(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "duplicate-key@test.com")

	first := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-duplicate",
		Name:   "first",
		Status: service.StatusActive,
	}
	second := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-duplicate",
		Name:   "second",
		Status: service.StatusActive,
	}

	require.NoError(t, repo.Create(ctx, first))
	err := repo.Create(ctx, second)
	require.ErrorIs(t, err, service.ErrAPIKeyExists)
}

func TestAPIKeyRepository_UpdateWithNilGroupIDsPreservesAuthorizedGroups(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "update-nil-groups@test.com")
	groupA := mustCreateAPIKeyRepoGroup(t, ctx, client, "unit-update-nil-a")
	groupB := mustCreateAPIKeyRepoGroup(t, ctx, client, "unit-update-nil-b")

	key := &service.APIKey{
		UserID:   user.ID,
		Key:      "sk-update-nil-groups",
		Name:     "Update nil groups",
		Status:   service.StatusActive,
		GroupIDs: []int64{groupA.ID, groupB.ID},
	}
	require.NoError(t, repo.Create(ctx, key))

	key.Name = "Update nil groups renamed"
	key.GroupIDs = nil
	require.NoError(t, repo.Update(ctx, key))

	got, err := client.APIKey.Query().
		Where(apikey.IDEQ(key.ID)).
		WithAPIKeyGroups(withAPIKeyGroupsOrdered).
		Only(ctx)
	require.NoError(t, err)
	require.Equal(t, "Update nil groups renamed", got.Name)
	require.Len(t, got.Edges.APIKeyGroups, 2)
	require.Equal(t, []int64{groupA.ID, groupB.ID}, []int64{got.Edges.APIKeyGroups[0].GroupID, got.Edges.APIKeyGroups[1].GroupID})
}

func TestAPIKeyRepository_UpdateWithEmptyGroupIDsClearsAuthorizedGroups(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "update-empty-groups@test.com")
	groupA := mustCreateAPIKeyRepoGroup(t, ctx, client, "unit-update-empty-a")
	groupB := mustCreateAPIKeyRepoGroup(t, ctx, client, "unit-update-empty-b")

	key := &service.APIKey{
		UserID:   user.ID,
		Key:      "sk-update-empty-groups",
		Name:     "Update empty groups",
		Status:   service.StatusActive,
		GroupIDs: []int64{groupA.ID, groupB.ID},
	}
	require.NoError(t, repo.Create(ctx, key))

	key.GroupIDs = []int64{}
	require.NoError(t, repo.Update(ctx, key))

	got, err := client.APIKey.Query().
		Where(apikey.IDEQ(key.ID)).
		WithAPIKeyGroups(withAPIKeyGroupsOrdered).
		Only(ctx)
	require.NoError(t, err)
	require.Empty(t, got.Edges.APIKeyGroups)
}

func TestAPIKeyRepository_LoadAuthorizedGroupsThroughGetAndList(t *testing.T) {
	repo, client := newAPIKeyRepoSQLite(t)
	ctx := context.Background()
	user := mustCreateAPIKeyRepoUser(t, ctx, client, "authorized-groups-read@test.com")
	defaultGroup := mustCreateAPIKeyRepoGroup(t, ctx, client, "unit-default-group")
	groupA := mustCreateAPIKeyRepoGroup(t, ctx, client, "unit-authorized-a")
	groupB := mustCreateAPIKeyRepoGroup(t, ctx, client, "unit-authorized-b")

	key := &service.APIKey{
		UserID:   user.ID,
		Key:      "sk-authorized-groups-read",
		Name:     "Authorized groups read",
		GroupID:  &defaultGroup.ID,
		GroupIDs: []int64{groupB.ID, groupA.ID},
		Status:   service.StatusActive,
	}
	require.NoError(t, repo.Create(ctx, key))

	got, err := repo.GetByID(ctx, key.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Group)
	require.Equal(t, defaultGroup.ID, got.Group.ID)
	require.Equal(t, []int64{groupA.ID, groupB.ID}, got.GroupIDs)
	require.Len(t, got.Groups, 2)
	require.Len(t, got.AuthorizedGroups, 2)
	require.Equal(t, groupA.ID, got.Groups[0].ID)
	require.Equal(t, groupB.ID, got.Groups[1].ID)
	require.Equal(t, groupA.ID, got.AuthorizedGroups[0].GroupID)
	require.Equal(t, groupB.ID, got.AuthorizedGroups[1].GroupID)

	items, _, err := repo.ListByUserID(
		ctx,
		user.ID,
		pagination.PaginationParams{Page: 1, PageSize: 10},
		service.APIKeyListFilters{},
	)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, []int64{groupA.ID, groupB.ID}, items[0].GroupIDs)
	require.Len(t, items[0].AuthorizedGroups, 2)
	require.Equal(t, groupA.ID, items[0].AuthorizedGroups[0].GroupID)
	require.Equal(t, groupB.ID, items[0].AuthorizedGroups[1].GroupID)
}

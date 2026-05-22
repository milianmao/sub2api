package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newMicrosoftEmailRepoSQLite(t *testing.T) (service.MicrosoftEmailRepository, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return NewMicrosoftEmailRepository(client), client
}

func TestMicrosoftEmailRepository_CRUDAndResultUpdates(t *testing.T) {
	repo, _ := newMicrosoftEmailRepoSQLite(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, &service.MicrosoftEmailAccount{
		Email:        "ms-crud@example.com",
		Password:     "raw-password",
		ClientID:     "client-id",
		RefreshToken: "raw-refresh-token",
		Status:       service.MicrosoftEmailStatusActive,
	})
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, "raw-password", created.Password)
	require.Equal(t, "raw-refresh-token", created.RefreshToken)
	require.Equal(t, service.MicrosoftEmailStatusActive, created.Status)

	byID, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.Email, byID.Email)
	require.Equal(t, "raw-password", byID.Password)

	updated, err := repo.UpdateCredentials(ctx, created.ID, service.MicrosoftEmailCredentialUpdate{
		Password:     "new-password",
		ClientID:     "new-client-id",
		RefreshToken: "new-refresh-token",
	})
	require.NoError(t, err)
	require.Equal(t, "new-password", updated.Password)
	require.Equal(t, "new-client-id", updated.ClientID)
	require.Equal(t, "new-refresh-token", updated.RefreshToken)
	require.Equal(t, service.MicrosoftEmailStatusActive, updated.Status)
	require.Nil(t, updated.LastError)

	checkedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	checkErr := "invalid_grant"
	require.NoError(t, repo.UpdateCheckResult(ctx, created.ID, service.MicrosoftEmailStatusInvalid, checkedAt, &checkErr))

	afterCheck, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, service.MicrosoftEmailStatusInvalid, afterCheck.Status)
	require.NotNil(t, afterCheck.LastCheckAt)
	require.WithinDuration(t, checkedAt, *afterCheck.LastCheckAt, time.Second)
	require.NotNil(t, afterCheck.LastError)
	require.Equal(t, checkErr, *afterCheck.LastError)

	fetchedAt := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, repo.UpdateFetchResult(ctx, created.ID, fetchedAt, nil, nil))

	afterFetch, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, service.MicrosoftEmailStatusInvalid, afterFetch.Status, "nil fetch status must preserve existing status")
	require.NotNil(t, afterFetch.LastFetchAt)
	require.WithinDuration(t, fetchedAt, *afterFetch.LastFetchAt, time.Second)
	require.Nil(t, afterFetch.LastError)

	require.NoError(t, repo.Delete(ctx, created.ID))
	_, err = repo.GetByID(ctx, created.ID)
	require.ErrorIs(t, err, service.ErrMicrosoftEmailNotFound)
}

func TestMicrosoftEmailRepository_ListFiltersAndPagination(t *testing.T) {
	repo, _ := newMicrosoftEmailRepoSQLite(t)
	ctx := context.Background()

	accounts := []*service.MicrosoftEmailAccount{
		{Email: "alice@example.com", Password: "p1", ClientID: "c1", RefreshToken: "r1", Status: service.MicrosoftEmailStatusActive},
		{Email: "bob@example.com", Password: "p2", ClientID: "c2", RefreshToken: "r2", Status: service.MicrosoftEmailStatusInvalid},
		{Email: "carol@contoso.com", Password: "p3", ClientID: "c3", RefreshToken: "r3", Status: service.MicrosoftEmailStatusActive},
	}
	for _, account := range accounts {
		_, err := repo.Create(ctx, account)
		require.NoError(t, err)
	}

	items, total, err := repo.List(ctx, service.MicrosoftEmailListFilter{Email: "example", Status: service.MicrosoftEmailStatusActive, Limit: 1, Offset: 0})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, "alice@example.com", items[0].Email)

	items, total, err = repo.List(ctx, service.MicrosoftEmailListFilter{Limit: 2, Offset: 1})
	require.NoError(t, err)
	require.Equal(t, 3, total)
	require.Len(t, items, 2)
	require.Equal(t, "bob@example.com", items[0].Email)
	require.Equal(t, "alice@example.com", items[1].Email)
}

func TestMicrosoftEmailRepository_NotFound(t *testing.T) {
	repo, _ := newMicrosoftEmailRepoSQLite(t)
	ctx := context.Background()

	_, err := repo.GetByID(ctx, 404)
	require.ErrorIs(t, err, service.ErrMicrosoftEmailNotFound)

	_, err = repo.GetByEmail(ctx, "missing@example.com")
	require.ErrorIs(t, err, service.ErrMicrosoftEmailNotFound)

	err = repo.UpdateCheckResult(ctx, 404, service.MicrosoftEmailStatusActive, time.Now(), nil)
	require.ErrorIs(t, err, service.ErrMicrosoftEmailNotFound)

	err = repo.Delete(ctx, 404)
	require.ErrorIs(t, err, service.ErrMicrosoftEmailNotFound)
}

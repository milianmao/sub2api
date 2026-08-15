package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestPublishOrInvalidatePoolRoleSnapshotDoesNotFailAfterCacheOutage(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := newSchedulerCacheWithChunkSizes(client, defaultSchedulerSnapshotMGetChunkSize, defaultSchedulerSnapshotWriteChunkSize)
	server.Close()
	repo := &accountRepository{schedulerCache: cache}

	err := repo.publishOrInvalidatePoolRoleSnapshot(context.Background(), &service.Account{ID: 1, PoolRevision: 1})

	require.NoError(t, err)
}

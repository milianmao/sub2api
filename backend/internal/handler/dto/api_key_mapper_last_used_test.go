package dto

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyFromService_MapsLastUsedAt(t *testing.T) {
	lastUsed := time.Now().UTC().Truncate(time.Second)
	src := &service.APIKey{
		ID:         1,
		UserID:     2,
		Key:        "sk-map-last-used",
		Name:       "Mapper",
		Status:     service.StatusActive,
		LastUsedAt: &lastUsed,
	}

	out := APIKeyFromService(src)
	require.NotNil(t, out)
	require.NotNil(t, out.LastUsedAt)
	require.WithinDuration(t, lastUsed, *out.LastUsedAt, time.Second)
}

func TestAPIKeyFromService_MapsNilLastUsedAt(t *testing.T) {
	src := &service.APIKey{
		ID:     1,
		UserID: 2,
		Key:    "sk-map-last-used-nil",
		Name:   "MapperNil",
		Status: service.StatusActive,
	}

	out := APIKeyFromService(src)
	require.NotNil(t, out)
	require.Nil(t, out.LastUsedAt)
}

func TestAPIKeyFromServiceMapsAuthorizedGroups(t *testing.T) {
	defaultGroupID := int64(1)
	key := &service.APIKey{
		ID:       10,
		UserID:   20,
		Name:     "multi-group-key",
		GroupID:  &defaultGroupID,
		GroupIDs: []int64{1, 2},
		Group:    &service.Group{ID: 1, Name: "default"},
		Groups: []*service.Group{
			{ID: 1, Name: "default"},
			{ID: 2, Name: "image"},
		},
	}

	out := APIKeyFromService(key)
	require.NotNil(t, out)
	require.Equal(t, []int64{1, 2}, out.GroupIDs)
	require.Len(t, out.Groups, 2)
	require.Equal(t, int64(1), out.Groups[0].ID)
	require.Equal(t, int64(2), out.Groups[1].ID)
}

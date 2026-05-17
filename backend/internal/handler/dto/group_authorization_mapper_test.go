package dto

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserFromServiceAdminMapsAuthorizationFields(t *testing.T) {
	t.Parallel()

	out := UserFromServiceAdmin(&service.User{
		ID:            42,
		Email:         "user@example.com",
		Role:          service.RoleUser,
		Status:        service.StatusActive,
		Level:         7,
		AllowedGroups: []int64{10, 20},
	})

	require.NotNil(t, out)
	require.Equal(t, 7, out.Level)
	require.Equal(t, []int64{10, 20}, out.AllowedGroups)
}

func TestGroupFromServiceAdminMapsAuthorizationFields(t *testing.T) {
	t.Parallel()

	out := GroupFromServiceAdmin(&service.Group{
		ID:           9,
		Name:         "restricted",
		Status:       service.StatusActive,
		AccessMode:   service.GroupAccessModeRestricted,
		MinUserLevel: 3,
	})

	require.NotNil(t, out)
	require.Equal(t, service.GroupAccessModeRestricted, out.AccessMode)
	require.Equal(t, 3, out.MinUserLevel)
}

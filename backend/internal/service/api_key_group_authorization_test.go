package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyServiceCanUserBindGroupInternal_GroupAuthorization(t *testing.T) {
	svc := &APIKeyService{}
	user := &User{
		ID:            7,
		Level:         1,
		AllowedGroups: []int64{20},
	}

	tests := []struct {
		name        string
		group       Group
		subscribed  map[int64]bool
		wantAllowed bool
	}{
		{
			name: "public group allows user meeting level",
			group: Group{
				ID:           10,
				AccessMode:   GroupAccessModePublic,
				MinUserLevel: 1,
			},
			wantAllowed: true,
		},
		{
			name: "public group rejects user below level",
			group: Group{
				ID:           11,
				AccessMode:   GroupAccessModePublic,
				MinUserLevel: 2,
			},
			wantAllowed: false,
		},
		{
			name: "restricted group requires explicit user grant",
			group: Group{
				ID:           12,
				AccessMode:   GroupAccessModeRestricted,
				MinUserLevel: 1,
			},
			wantAllowed: false,
		},
		{
			name: "restricted group allows explicit user grant below level",
			group: Group{
				ID:           20,
				AccessMode:   GroupAccessModeRestricted,
				MinUserLevel: 5,
			},
			wantAllowed: true,
		},
		{
			name: "public group allows explicit user grant below level",
			group: Group{
				ID:           20,
				AccessMode:   GroupAccessModePublic,
				MinUserLevel: 5,
			},
			wantAllowed: true,
		},
		{
			name: "legacy exclusive group is restricted when access mode is empty",
			group: Group{
				ID:            21,
				IsExclusive:   true,
				MinUserLevel:  1,
				AccessMode:    "",
			},
			wantAllowed: false,
		},
		{
			name: "subscription group still requires active subscription",
			group: Group{
				ID:               30,
				AccessMode:       GroupAccessModePublic,
				MinUserLevel:     1,
				SubscriptionType: SubscriptionTypeSubscription,
			},
			subscribed:  map[int64]bool{30: true},
			wantAllowed: true,
		},
		{
			name: "restricted subscription group requires subscription and explicit grant",
			group: Group{
				ID:               31,
				AccessMode:       GroupAccessModeRestricted,
				MinUserLevel:     1,
				SubscriptionType: SubscriptionTypeSubscription,
			},
			subscribed:  map[int64]bool{31: true},
			wantAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.subscribed == nil {
				tt.subscribed = map[int64]bool{}
			}
			require.Equal(t, tt.wantAllowed, svc.canUserBindGroupInternal(user, &tt.group, tt.subscribed))
		})
	}
}

func TestAPIKeyServiceCanUserBindGroup_QueryPathUsesSameAuthorization(t *testing.T) {
	svc := &APIKeyService{
		userSubRepo: &stubUserSubscriptionRepoForGroupAuth{
			activeByGroup: map[int64]bool{20: true},
		},
	}
	user := &User{ID: 7, Level: 1, AllowedGroups: []int64{20}}
	group := &Group{
		ID:               20,
		AccessMode:       GroupAccessModeRestricted,
		MinUserLevel:     1,
		SubscriptionType: SubscriptionTypeSubscription,
	}

	require.True(t, svc.canUserBindGroup(context.Background(), user, group))
}

type stubUserSubscriptionRepoForGroupAuth struct {
	activeByGroup map[int64]bool
}

func (s *stubUserSubscriptionRepoForGroupAuth) Create(context.Context, *UserSubscription) error {
	panic("unexpected Create call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) GetByID(context.Context, int64) (*UserSubscription, error) {
	panic("unexpected GetByID call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) GetByUserIDAndGroupID(context.Context, int64, int64) (*UserSubscription, error) {
	panic("unexpected GetByUserIDAndGroupID call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) GetActiveByUserIDAndGroupID(_ context.Context, _ int64, groupID int64) (*UserSubscription, error) {
	if s.activeByGroup[groupID] {
		return &UserSubscription{GroupID: groupID}, nil
	}
	return nil, ErrSubscriptionNotFound
}

func (s *stubUserSubscriptionRepoForGroupAuth) ListByUserID(context.Context, int64) ([]UserSubscription, error) {
	panic("unexpected ListByUserID call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	panic("unexpected ListActiveByUserID call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) ListAllActive(context.Context) ([]UserSubscription, error) {
	panic("unexpected ListAllActive call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroupID call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) List(context.Context, pagination.PaginationParams, *int64, *int64, string, string, string, string) ([]UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) Update(context.Context, *UserSubscription) error {
	panic("unexpected Update call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	panic("unexpected ExistsByUserIDAndGroupID call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) ExtendExpiry(context.Context, int64, time.Time) error {
	panic("unexpected ExtendExpiry call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) UpdateStatus(context.Context, int64, string) error {
	panic("unexpected UpdateStatus call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) UpdateNotes(context.Context, int64, string) error {
	panic("unexpected UpdateNotes call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) ActivateWindows(context.Context, int64, time.Time) error {
	panic("unexpected ActivateWindows call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) ResetDailyUsage(context.Context, int64, time.Time) error {
	panic("unexpected ResetDailyUsage call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) ResetWeeklyUsage(context.Context, int64, time.Time) error {
	panic("unexpected ResetWeeklyUsage call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) ResetMonthlyUsage(context.Context, int64, time.Time) error {
	panic("unexpected ResetMonthlyUsage call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) IncrementUsage(context.Context, int64, float64) error {
	panic("unexpected IncrementUsage call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) ExpireOldSubscriptions(context.Context) error {
	panic("unexpected ExpireOldSubscriptions call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) BatchUpdateExpiredStatus(context.Context) (int64, error) {
	panic("unexpected BatchUpdateExpiredStatus call")
}

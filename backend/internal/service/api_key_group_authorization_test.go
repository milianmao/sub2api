package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAPIKeyGroupIDs(t *testing.T) {
	group1 := int64(1)
	invalidGroup := int64(-1)

	tests := []struct {
		name    string
		groupID *int64
		input   []int64
		want    []int64
		wantErr error
	}{
		{name: "group id only becomes authorized group", groupID: &group1, want: []int64{1}},
		{name: "group ids include legacy group id", groupID: &group1, input: []int64{2, 1}, want: []int64{1, 2}},
		{name: "group ids without legacy group id are source of truth", groupID: &group1, input: []int64{2}, want: []int64{2}},
		{name: "nil default rejects empty group set", input: nil, wantErr: ErrDefaultGroupNotAuthorized},
		{name: "nil default accepts group ids", input: []int64{1}, want: []int64{1}},
		{name: "duplicates are removed", groupID: &group1, input: []int64{1, 1, 2}, want: []int64{1, 2}},
		{name: "non-positive ids are rejected", groupID: &group1, input: []int64{1, 0}, wantErr: ErrInvalidAPIKeyGroupID},
		{name: "nil default rejects zero authorized group as invalid", input: []int64{0}, wantErr: ErrInvalidAPIKeyGroupID},
		{name: "nil default rejects negative authorized group as invalid", input: []int64{-1}, wantErr: ErrInvalidAPIKeyGroupID},
		{name: "non-positive default group is rejected", groupID: &invalidGroup, wantErr: ErrInvalidAPIKeyGroupID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeAPIKeyGroupIDs(tt.groupID, tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

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
				ID:           21,
				IsExclusive:  true,
				MinUserLevel: 1,
				AccessMode:   "",
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

func TestAPIKeyServiceCreateRejectsUnauthorizedAuthorizedGroup(t *testing.T) {
	defaultGroupID := int64(1)
	apiKeyRepo := &stubAPIKeyRepoForGroupAuth{}
	svc := &APIKeyService{
		apiKeyRepo: apiKeyRepo,
		userRepo: &stubUserRepoForGroupAuth{
			users: map[int64]*User{7: {ID: 7, Level: 1}},
		},
		groupRepo: &stubGroupRepoForGroupAuth{
			groups: map[int64]*Group{
				1: {ID: 1, AccessMode: GroupAccessModePublic, MinUserLevel: 1},
				2: {ID: 2, AccessMode: GroupAccessModeRestricted, MinUserLevel: 1},
			},
		},
		userSubRepo: &stubUserSubscriptionRepoForGroupAuth{},
	}

	_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{
		Name:      "key",
		GroupID:   &defaultGroupID,
		GroupIDs:  []int64{1, 2},
		CustomKey: stringPtrForGroupAuth("custom-key-12345"),
	})

	require.ErrorIs(t, err, ErrGroupNotAllowed)
	require.Nil(t, apiKeyRepo.created)
}

func TestAPIKeyServiceCreateUsesGroupIDsAsSingleGroupSet(t *testing.T) {
	apiKeyRepo := &stubAPIKeyRepoForGroupAuth{}
	svc := &APIKeyService{
		apiKeyRepo: apiKeyRepo,
		userRepo: &stubUserRepoForGroupAuth{
			users: map[int64]*User{7: {ID: 7, Level: 1, AllowedGroups: []int64{2}}},
		},
		groupRepo: &stubGroupRepoForGroupAuth{
			groups: map[int64]*Group{
				1: {ID: 1, AccessMode: GroupAccessModePublic, MinUserLevel: 1},
				2: {ID: 2, AccessMode: GroupAccessModeRestricted, MinUserLevel: 1},
			},
		},
		userSubRepo: &stubUserSubscriptionRepoForGroupAuth{},
	}

	out, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{
		Name:      "key",
		GroupIDs:  []int64{2, 1},
		CustomKey: stringPtrForGroupAuth("custom-key-12345"),
	})

	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, out.GroupIDs)
	require.NotNil(t, out.GroupID)
	require.Equal(t, int64(1), *out.GroupID)
	require.NotNil(t, apiKeyRepo.created)
	require.Equal(t, []int64{1, 2}, apiKeyRepo.created.GroupIDs)
	require.NotNil(t, apiKeyRepo.created.GroupID)
	require.Equal(t, int64(1), *apiKeyRepo.created.GroupID)
}

func TestAPIKeyServiceUpdateNameOnlyPreservesAuthorizedGroups(t *testing.T) {
	defaultGroupID := int64(1)
	apiKeyRepo := &stubAPIKeyRepoForGroupAuth{
		apiKey: &APIKey{
			ID:       10,
			UserID:   7,
			Key:      "custom-key-12345",
			Name:     "old-name",
			GroupID:  &defaultGroupID,
			GroupIDs: []int64{1, 2},
		},
	}
	svc := &APIKeyService{apiKeyRepo: apiKeyRepo}
	newName := "new-name"

	out, err := svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{Name: &newName})

	require.NoError(t, err)
	require.Equal(t, "new-name", out.Name)
	require.Equal(t, []int64{1, 2}, out.GroupIDs)
	require.NotNil(t, apiKeyRepo.updated)
	require.Equal(t, []int64{1, 2}, apiKeyRepo.updated.GroupIDs)
}

func TestAPIKeyServiceUpdateRejectsUnauthorizedAuthorizedGroup(t *testing.T) {
	defaultGroupID := int64(1)
	apiKeyRepo := &stubAPIKeyRepoForGroupAuth{
		apiKey: &APIKey{
			ID:       10,
			UserID:   7,
			Key:      "custom-key-12345",
			Name:     "key",
			GroupID:  &defaultGroupID,
			GroupIDs: []int64{1},
		},
	}
	groupIDs := []int64{1, 2}
	svc := &APIKeyService{
		apiKeyRepo: apiKeyRepo,
		userRepo: &stubUserRepoForGroupAuth{
			users: map[int64]*User{7: {ID: 7, Level: 1}},
		},
		groupRepo: &stubGroupRepoForGroupAuth{
			groups: map[int64]*Group{
				1: {ID: 1, AccessMode: GroupAccessModePublic, MinUserLevel: 1},
				2: {ID: 2, AccessMode: GroupAccessModeRestricted, MinUserLevel: 1},
			},
		},
		userSubRepo: &stubUserSubscriptionRepoForGroupAuth{},
	}

	_, err := svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{GroupIDs: &groupIDs})

	require.ErrorIs(t, err, ErrGroupNotAllowed)
	require.Nil(t, apiKeyRepo.updated)
}

func TestAPIKeyServiceUpdateUsesGroupIDsAsSingleGroupSet(t *testing.T) {
	defaultGroupID := int64(1)
	apiKeyRepo := &stubAPIKeyRepoForGroupAuth{
		apiKey: &APIKey{
			ID:       10,
			UserID:   7,
			Key:      "custom-key-12345",
			Name:     "key",
			GroupID:  &defaultGroupID,
			GroupIDs: []int64{1},
		},
	}
	groupIDs := []int64{2}
	svc := &APIKeyService{
		apiKeyRepo: apiKeyRepo,
		userRepo: &stubUserRepoForGroupAuth{
			users: map[int64]*User{7: {ID: 7, Level: 1, AllowedGroups: []int64{2}}},
		},
		groupRepo: &stubGroupRepoForGroupAuth{
			groups: map[int64]*Group{
				2: {ID: 2, AccessMode: GroupAccessModeRestricted, MinUserLevel: 1},
			},
		},
		userSubRepo: &stubUserSubscriptionRepoForGroupAuth{},
	}

	out, err := svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{GroupIDs: &groupIDs})

	require.NoError(t, err)
	require.Equal(t, []int64{2}, out.GroupIDs)
	require.NotNil(t, out.GroupID)
	require.Equal(t, int64(2), *out.GroupID)
	require.NotNil(t, apiKeyRepo.updated)
	require.Equal(t, []int64{2}, apiKeyRepo.updated.GroupIDs)
	require.NotNil(t, apiKeyRepo.updated.GroupID)
	require.Equal(t, int64(2), *apiKeyRepo.updated.GroupID)
}

func TestAPIKeyServiceUpdateExplicitAuthorizedGroups(t *testing.T) {
	defaultGroupID := int64(1)
	apiKeyRepo := &stubAPIKeyRepoForGroupAuth{
		apiKey: &APIKey{
			ID:       10,
			UserID:   7,
			Key:      "custom-key-12345",
			Name:     "key",
			GroupID:  &defaultGroupID,
			GroupIDs: []int64{1},
		},
	}
	groupIDs := []int64{2, 1}
	svc := &APIKeyService{
		apiKeyRepo: apiKeyRepo,
		userRepo: &stubUserRepoForGroupAuth{
			users: map[int64]*User{7: {ID: 7, Level: 1, AllowedGroups: []int64{2}}},
		},
		groupRepo: &stubGroupRepoForGroupAuth{
			groups: map[int64]*Group{
				1: {ID: 1, AccessMode: GroupAccessModePublic, MinUserLevel: 1},
				2: {ID: 2, AccessMode: GroupAccessModeRestricted, MinUserLevel: 1},
			},
		},
		userSubRepo: &stubUserSubscriptionRepoForGroupAuth{},
	}

	out, err := svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{GroupIDs: &groupIDs})

	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, out.GroupIDs)
	require.NotNil(t, apiKeyRepo.updated)
	require.Equal(t, []int64{1, 2}, apiKeyRepo.updated.GroupIDs)
}

func TestAPIKeyServiceCreateRejectsInvalidAuthorizedGroupIDBeforeLookup(t *testing.T) {
	apiKeyRepo := &stubAPIKeyRepoForGroupAuth{}
	groupRepo := &stubGroupRepoForGroupAuth{}
	svc := &APIKeyService{
		apiKeyRepo: apiKeyRepo,
		userRepo: &stubUserRepoForGroupAuth{
			users: map[int64]*User{7: {ID: 7, Level: 1}},
		},
		groupRepo:   groupRepo,
		userSubRepo: &stubUserSubscriptionRepoForGroupAuth{},
	}

	_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{
		Name:      "key",
		GroupIDs:  []int64{0},
		CustomKey: stringPtrForGroupAuth("custom-key-12345"),
	})

	require.ErrorIs(t, err, ErrInvalidAPIKeyGroupID)
	require.Empty(t, groupRepo.getByIDCalls)
	require.Nil(t, apiKeyRepo.created)
}

func TestAPIKeyServiceUpdateRejectsInvalidAuthorizedGroupIDBeforeLookup(t *testing.T) {
	apiKeyRepo := &stubAPIKeyRepoForGroupAuth{
		apiKey: &APIKey{
			ID:       10,
			UserID:   7,
			Key:      "custom-key-12345",
			Name:     "key",
			GroupID:  nil,
			GroupIDs: nil,
		},
	}
	groupIDs := []int64{0}
	groupRepo := &stubGroupRepoForGroupAuth{}
	svc := &APIKeyService{
		apiKeyRepo: apiKeyRepo,
		userRepo: &stubUserRepoForGroupAuth{
			users: map[int64]*User{7: {ID: 7, Level: 1}},
		},
		groupRepo:   groupRepo,
		userSubRepo: &stubUserSubscriptionRepoForGroupAuth{},
	}

	_, err := svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{GroupIDs: &groupIDs})

	require.ErrorIs(t, err, ErrInvalidAPIKeyGroupID)
	require.Empty(t, groupRepo.getByIDCalls)
	require.Nil(t, apiKeyRepo.updated)
}

func TestAPIKeyServiceCreateRejectsInvalidDefaultGroupIDBeforeLookup(t *testing.T) {
	invalidGroupID := int64(0)
	apiKeyRepo := &stubAPIKeyRepoForGroupAuth{}
	groupRepo := &stubGroupRepoForGroupAuth{}
	svc := &APIKeyService{
		apiKeyRepo: apiKeyRepo,
		userRepo: &stubUserRepoForGroupAuth{
			users: map[int64]*User{7: {ID: 7, Level: 1}},
		},
		groupRepo:   groupRepo,
		userSubRepo: &stubUserSubscriptionRepoForGroupAuth{},
	}

	_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{
		Name:      "key",
		GroupID:   &invalidGroupID,
		CustomKey: stringPtrForGroupAuth("custom-key-12345"),
	})

	require.ErrorIs(t, err, ErrInvalidAPIKeyGroupID)
	require.Empty(t, groupRepo.getByIDCalls)
	require.Nil(t, apiKeyRepo.created)
}

func TestAPIKeyServiceUpdateRejectsInvalidDefaultGroupIDBeforeLookup(t *testing.T) {
	defaultGroupID := int64(1)
	invalidGroupID := int64(-1)
	apiKeyRepo := &stubAPIKeyRepoForGroupAuth{
		apiKey: &APIKey{
			ID:       10,
			UserID:   7,
			Key:      "custom-key-12345",
			Name:     "key",
			GroupID:  &defaultGroupID,
			GroupIDs: []int64{1},
		},
	}
	groupRepo := &stubGroupRepoForGroupAuth{}
	svc := &APIKeyService{
		apiKeyRepo: apiKeyRepo,
		userRepo: &stubUserRepoForGroupAuth{
			users: map[int64]*User{7: {ID: 7, Level: 1}},
		},
		groupRepo:   groupRepo,
		userSubRepo: &stubUserSubscriptionRepoForGroupAuth{},
	}

	_, err := svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{GroupID: &invalidGroupID})

	require.ErrorIs(t, err, ErrInvalidAPIKeyGroupID)
	require.Empty(t, groupRepo.getByIDCalls)
	require.Nil(t, apiKeyRepo.updated)
}

func TestAPIKeyServiceUpdateExplicitEmptyAuthorizedGroups(t *testing.T) {
	defaultGroupID := int64(1)
	apiKeyRepo := &stubAPIKeyRepoForGroupAuth{
		apiKey: &APIKey{
			ID:       10,
			UserID:   7,
			Key:      "custom-key-12345",
			Name:     "key",
			GroupID:  &defaultGroupID,
			GroupIDs: []int64{1, 2},
		},
	}
	groupIDs := []int64{}
	svc := &APIKeyService{
		apiKeyRepo: apiKeyRepo,
		userRepo: &stubUserRepoForGroupAuth{
			users: map[int64]*User{7: {ID: 7, Level: 1}},
		},
		groupRepo:   &stubGroupRepoForGroupAuth{},
		userSubRepo: &stubUserSubscriptionRepoForGroupAuth{},
	}

	_, err := svc.Update(context.Background(), 10, 7, UpdateAPIKeyRequest{GroupIDs: &groupIDs})

	require.ErrorIs(t, err, ErrDefaultGroupNotAuthorized)
	require.Nil(t, apiKeyRepo.updated)
}

func stringPtrForGroupAuth(v string) *string {
	return &v
}

type stubAPIKeyRepoForGroupAuth struct {
	apiKey  *APIKey
	created *APIKey
	updated *APIKey
}

func (s *stubAPIKeyRepoForGroupAuth) Create(_ context.Context, key *APIKey) error {
	clone := *key
	s.created = &clone
	return nil
}

func (s *stubAPIKeyRepoForGroupAuth) GetByID(_ context.Context, _ int64) (*APIKey, error) {
	if s.apiKey == nil {
		return nil, ErrAPIKeyNotFound
	}
	clone := *s.apiKey
	clone.GroupIDs = append([]int64(nil), s.apiKey.GroupIDs...)
	return &clone, nil
}

func (s *stubAPIKeyRepoForGroupAuth) GetKeyAndOwnerID(context.Context, int64) (string, int64, error) {
	panic("unexpected GetKeyAndOwnerID call")
}
func (s *stubAPIKeyRepoForGroupAuth) GetByKey(context.Context, string) (*APIKey, error) {
	panic("unexpected GetByKey call")
}
func (s *stubAPIKeyRepoForGroupAuth) GetByKeyForAuth(context.Context, string) (*APIKey, error) {
	panic("unexpected GetByKeyForAuth call")
}
func (s *stubAPIKeyRepoForGroupAuth) Update(_ context.Context, key *APIKey) error {
	clone := *key
	clone.GroupIDs = append([]int64(nil), key.GroupIDs...)
	s.updated = &clone
	return nil
}
func (s *stubAPIKeyRepoForGroupAuth) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}
func (s *stubAPIKeyRepoForGroupAuth) DeleteWithAudit(context.Context, int64) error {
	panic("unexpected DeleteWithAudit call")
}
func (s *stubAPIKeyRepoForGroupAuth) ListByUserID(context.Context, int64, pagination.PaginationParams, APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserID call")
}
func (s *stubAPIKeyRepoForGroupAuth) VerifyOwnership(context.Context, int64, []int64) ([]int64, error) {
	panic("unexpected VerifyOwnership call")
}
func (s *stubAPIKeyRepoForGroupAuth) CountByUserID(context.Context, int64) (int64, error) {
	panic("unexpected CountByUserID call")
}
func (s *stubAPIKeyRepoForGroupAuth) ExistsByKey(context.Context, string) (bool, error) {
	return false, nil
}
func (s *stubAPIKeyRepoForGroupAuth) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]APIKey, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroupID call")
}
func (s *stubAPIKeyRepoForGroupAuth) SearchAPIKeys(context.Context, int64, string, int) ([]APIKey, error) {
	panic("unexpected SearchAPIKeys call")
}
func (s *stubAPIKeyRepoForGroupAuth) ClearGroupIDByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected ClearGroupIDByGroupID call")
}
func (s *stubAPIKeyRepoForGroupAuth) UpdateGroupIDByUserAndGroup(context.Context, int64, int64, int64) (int64, error) {
	panic("unexpected UpdateGroupIDByUserAndGroup call")
}
func (s *stubAPIKeyRepoForGroupAuth) CountByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected CountByGroupID call")
}
func (s *stubAPIKeyRepoForGroupAuth) ListKeysByUserID(context.Context, int64) ([]string, error) {
	panic("unexpected ListKeysByUserID call")
}
func (s *stubAPIKeyRepoForGroupAuth) ListKeysByGroupID(context.Context, int64) ([]string, error) {
	panic("unexpected ListKeysByGroupID call")
}
func (s *stubAPIKeyRepoForGroupAuth) IncrementQuotaUsed(context.Context, int64, float64) (float64, error) {
	panic("unexpected IncrementQuotaUsed call")
}
func (s *stubAPIKeyRepoForGroupAuth) UpdateLastUsed(context.Context, int64, time.Time) error {
	panic("unexpected UpdateLastUsed call")
}
func (s *stubAPIKeyRepoForGroupAuth) IncrementRateLimitUsage(context.Context, int64, float64) error {
	panic("unexpected IncrementRateLimitUsage call")
}
func (s *stubAPIKeyRepoForGroupAuth) ResetRateLimitWindows(context.Context, int64) error {
	panic("unexpected ResetRateLimitWindows call")
}
func (s *stubAPIKeyRepoForGroupAuth) GetRateLimitData(context.Context, int64) (*APIKeyRateLimitData, error) {
	panic("unexpected GetRateLimitData call")
}

type stubUserRepoForGroupAuth struct {
	users map[int64]*User
}

func (s *stubUserRepoForGroupAuth) Create(context.Context, *User) error {
	panic("unexpected Create call")
}
func (s *stubUserRepoForGroupAuth) GetByID(_ context.Context, id int64) (*User, error) {
	if user := s.users[id]; user != nil {
		clone := *user
		clone.AllowedGroups = append([]int64(nil), user.AllowedGroups...)
		return &clone, nil
	}
	return nil, ErrUserNotFound
}
func (s *stubUserRepoForGroupAuth) GetByIDIncludeDeleted(ctx context.Context, id int64) (*User, error) {
	return s.GetByID(ctx, id)
}
func (s *stubUserRepoForGroupAuth) GetByEmail(context.Context, string) (*User, error) {
	panic("unexpected GetByEmail call")
}
func (s *stubUserRepoForGroupAuth) GetFirstAdmin(context.Context) (*User, error) {
	panic("unexpected GetFirstAdmin call")
}
func (s *stubUserRepoForGroupAuth) Update(context.Context, *User) error {
	panic("unexpected Update call")
}
func (s *stubUserRepoForGroupAuth) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}
func (s *stubUserRepoForGroupAuth) GetUserAvatar(context.Context, int64) (*UserAvatar, error) {
	panic("unexpected GetUserAvatar call")
}
func (s *stubUserRepoForGroupAuth) UpsertUserAvatar(context.Context, int64, UpsertUserAvatarInput) (*UserAvatar, error) {
	panic("unexpected UpsertUserAvatar call")
}
func (s *stubUserRepoForGroupAuth) DeleteUserAvatar(context.Context, int64) error {
	panic("unexpected DeleteUserAvatar call")
}
func (s *stubUserRepoForGroupAuth) List(context.Context, pagination.PaginationParams) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}
func (s *stubUserRepoForGroupAuth) ListWithFilters(context.Context, pagination.PaginationParams, UserListFilters) ([]User, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}
func (s *stubUserRepoForGroupAuth) GetLatestUsedAtByUserIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserIDs call")
}
func (s *stubUserRepoForGroupAuth) GetLatestUsedAtByUserID(context.Context, int64) (*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserID call")
}
func (s *stubUserRepoForGroupAuth) UpdateUserLastActiveAt(context.Context, int64, time.Time) error {
	panic("unexpected UpdateUserLastActiveAt call")
}
func (s *stubUserRepoForGroupAuth) UpdateBalance(context.Context, int64, float64) error {
	panic("unexpected UpdateBalance call")
}
func (s *stubUserRepoForGroupAuth) DeductBalance(context.Context, int64, float64) error {
	panic("unexpected DeductBalance call")
}
func (s *stubUserRepoForGroupAuth) UpdateConcurrency(context.Context, int64, int) error {
	panic("unexpected UpdateConcurrency call")
}
func (s *stubUserRepoForGroupAuth) BatchSetConcurrency(context.Context, []int64, int) (int, error) {
	panic("unexpected BatchSetConcurrency call")
}
func (s *stubUserRepoForGroupAuth) BatchAddConcurrency(context.Context, []int64, int) (int, error) {
	panic("unexpected BatchAddConcurrency call")
}
func (s *stubUserRepoForGroupAuth) ExistsByEmail(context.Context, string) (bool, error) {
	panic("unexpected ExistsByEmail call")
}
func (s *stubUserRepoForGroupAuth) RemoveGroupFromAllowedGroups(context.Context, int64) (int64, error) {
	panic("unexpected RemoveGroupFromAllowedGroups call")
}
func (s *stubUserRepoForGroupAuth) AddGroupToAllowedGroups(context.Context, int64, int64) error {
	panic("unexpected AddGroupToAllowedGroups call")
}
func (s *stubUserRepoForGroupAuth) RemoveGroupFromUserAllowedGroups(context.Context, int64, int64) error {
	panic("unexpected RemoveGroupFromUserAllowedGroups call")
}
func (s *stubUserRepoForGroupAuth) ListUserAuthIdentities(context.Context, int64) ([]UserAuthIdentityRecord, error) {
	panic("unexpected ListUserAuthIdentities call")
}
func (s *stubUserRepoForGroupAuth) UnbindUserAuthProvider(context.Context, int64, string) error {
	panic("unexpected UnbindUserAuthProvider call")
}
func (s *stubUserRepoForGroupAuth) UpdateTotpSecret(context.Context, int64, *string) error {
	panic("unexpected UpdateTotpSecret call")
}
func (s *stubUserRepoForGroupAuth) EnableTotp(context.Context, int64) error {
	panic("unexpected EnableTotp call")
}
func (s *stubUserRepoForGroupAuth) DisableTotp(context.Context, int64) error {
	panic("unexpected DisableTotp call")
}

type stubGroupRepoForGroupAuth struct {
	groups       map[int64]*Group
	getByIDCalls []int64
}

func (s *stubGroupRepoForGroupAuth) Create(context.Context, *Group) error {
	panic("unexpected Create call")
}
func (s *stubGroupRepoForGroupAuth) GetByID(_ context.Context, id int64) (*Group, error) {
	s.getByIDCalls = append(s.getByIDCalls, id)
	if group := s.groups[id]; group != nil {
		clone := *group
		return &clone, nil
	}
	return nil, ErrGroupNotFound
}
func (s *stubGroupRepoForGroupAuth) GetByIDLite(ctx context.Context, id int64) (*Group, error) {
	return s.GetByID(ctx, id)
}
func (s *stubGroupRepoForGroupAuth) Update(context.Context, *Group) error {
	panic("unexpected Update call")
}
func (s *stubGroupRepoForGroupAuth) Delete(context.Context, int64) error {
	panic("unexpected Delete call")
}
func (s *stubGroupRepoForGroupAuth) DeleteCascade(context.Context, int64) ([]int64, error) {
	panic("unexpected DeleteCascade call")
}
func (s *stubGroupRepoForGroupAuth) List(context.Context, pagination.PaginationParams) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}
func (s *stubGroupRepoForGroupAuth) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string, *bool) ([]Group, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}
func (s *stubGroupRepoForGroupAuth) ListActive(context.Context) ([]Group, error) {
	panic("unexpected ListActive call")
}
func (s *stubGroupRepoForGroupAuth) ListActiveByPlatform(context.Context, string) ([]Group, error) {
	panic("unexpected ListActiveByPlatform call")
}
func (s *stubGroupRepoForGroupAuth) ExistsByName(context.Context, string) (bool, error) {
	panic("unexpected ExistsByName call")
}
func (s *stubGroupRepoForGroupAuth) GetAccountCount(context.Context, int64) (int64, int64, error) {
	panic("unexpected GetAccountCount call")
}
func (s *stubGroupRepoForGroupAuth) DeleteAccountGroupsByGroupID(context.Context, int64) (int64, error) {
	panic("unexpected DeleteAccountGroupsByGroupID call")
}
func (s *stubGroupRepoForGroupAuth) GetAccountIDsByGroupIDs(context.Context, []int64) ([]int64, error) {
	panic("unexpected GetAccountIDsByGroupIDs call")
}
func (s *stubGroupRepoForGroupAuth) BindAccountsToGroup(context.Context, int64, []int64) error {
	panic("unexpected BindAccountsToGroup call")
}
func (s *stubGroupRepoForGroupAuth) UpdateSortOrders(context.Context, []GroupSortOrderUpdate) error {
	panic("unexpected UpdateSortOrders call")
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

func (s *stubUserSubscriptionRepoForGroupAuth) GetByIDIncludeDeleted(context.Context, int64) (*UserSubscription, error) {
	panic("unexpected GetByIDIncludeDeleted call")
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

func (s *stubUserSubscriptionRepoForGroupAuth) Restore(context.Context, int64, string) (*UserSubscription, error) {
	panic("unexpected Restore call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) ExistsByUserIDAndGroupID(context.Context, int64, int64) (bool, error) {
	panic("unexpected ExistsByUserIDAndGroupID call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) ExistsActiveByUserIDAndGroupID(_ context.Context, _ int64, groupID int64) (bool, error) {
	return s.activeByGroup[groupID], nil
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

func (s *stubUserSubscriptionRepoForGroupAuth) ResetUsageWindows(context.Context, int64, bool, bool, bool, time.Time) error {
	panic("unexpected ResetUsageWindows call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) ResetDailyUsage(context.Context, int64, *time.Time, time.Time) error {
	panic("unexpected ResetDailyUsage call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) ResetWeeklyUsage(context.Context, int64, *time.Time, time.Time) error {
	panic("unexpected ResetWeeklyUsage call")
}

func (s *stubUserSubscriptionRepoForGroupAuth) ResetMonthlyUsage(context.Context, int64, *time.Time, time.Time) error {
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

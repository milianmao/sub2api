package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/dgraph-io/ristretto"
)

const apiKeyAuthSnapshotVersion = 13 // v13: default group and priority-preserving authorized groups

type apiKeyAuthCacheConfig struct {
	l1Size        int
	l1TTL         time.Duration
	l2TTL         time.Duration
	negativeTTL   time.Duration
	jitterPercent int
	singleflight  bool
}

func newAPIKeyAuthCacheConfig(cfg *config.Config) apiKeyAuthCacheConfig {
	if cfg == nil {
		return apiKeyAuthCacheConfig{}
	}
	auth := cfg.APIKeyAuth
	return apiKeyAuthCacheConfig{
		l1Size:        auth.L1Size,
		l1TTL:         time.Duration(auth.L1TTLSeconds) * time.Second,
		l2TTL:         time.Duration(auth.L2TTLSeconds) * time.Second,
		negativeTTL:   time.Duration(auth.NegativeTTLSeconds) * time.Second,
		jitterPercent: auth.JitterPercent,
		singleflight:  auth.Singleflight,
	}
}

func (c apiKeyAuthCacheConfig) l1Enabled() bool {
	return c.l1Size > 0 && c.l1TTL > 0
}

func (c apiKeyAuthCacheConfig) l2Enabled() bool {
	return c.l2TTL > 0
}

func (c apiKeyAuthCacheConfig) negativeEnabled() bool {
	return c.negativeTTL > 0
}

// jitterTTL 为缓存 TTL 添加抖动，避免多个请求在同一时刻同时过期触发集中回源。
// 这里直接使用 rand/v2 的顶层函数：并发安全，无需全局互斥锁。
func (c apiKeyAuthCacheConfig) jitterTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return ttl
	}
	if c.jitterPercent <= 0 {
		return ttl
	}
	percent := c.jitterPercent
	if percent > 100 {
		percent = 100
	}
	delta := float64(percent) / 100
	randVal := rand.Float64()
	factor := 1 - delta + randVal*(2*delta)
	if factor <= 0 {
		return ttl
	}
	return time.Duration(float64(ttl) * factor)
}

func (s *APIKeyService) initAuthCache(cfg *config.Config) {
	s.authCfg = newAPIKeyAuthCacheConfig(cfg)
	if !s.authCfg.l1Enabled() {
		return
	}
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: int64(s.authCfg.l1Size) * 10,
		MaxCost:     int64(s.authCfg.l1Size),
		BufferItems: 64,
	})
	if err != nil {
		return
	}
	s.authCacheL1 = cache
}

// StartAuthCacheInvalidationSubscriber starts the Pub/Sub subscriber for L1 cache invalidation.
// This should be called after the service is fully initialized.
func (s *APIKeyService) StartAuthCacheInvalidationSubscriber(ctx context.Context) {
	if s.cache == nil || s.authCacheL1 == nil {
		return
	}
	if err := s.cache.SubscribeAuthCacheInvalidation(ctx, func(cacheKey string) {
		s.authCacheL1.Del(cacheKey)
	}); err != nil {
		// Log but don't fail - L1 cache will still work, just without cross-instance invalidation
		slog.Warn("failed to start auth cache invalidation subscriber", "error", err)
	}
}

func (s *APIKeyService) authCacheKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func (s *APIKeyService) getAuthCacheEntry(ctx context.Context, cacheKey string) (*APIKeyAuthCacheEntry, bool) {
	if s.authCacheL1 != nil {
		if val, ok := s.authCacheL1.Get(cacheKey); ok {
			if entry, ok := val.(*APIKeyAuthCacheEntry); ok {
				return entry, true
			}
		}
	}
	if s.cache == nil || !s.authCfg.l2Enabled() {
		return nil, false
	}
	entry, err := s.cache.GetAuthCache(ctx, cacheKey)
	if err != nil {
		return nil, false
	}
	s.setAuthCacheL1(cacheKey, entry)
	return entry, true
}

func (s *APIKeyService) setAuthCacheL1(cacheKey string, entry *APIKeyAuthCacheEntry) {
	if s.authCacheL1 == nil || entry == nil {
		return
	}
	ttl := s.authCfg.l1TTL
	if entry.NotFound && s.authCfg.negativeTTL > 0 && s.authCfg.negativeTTL < ttl {
		ttl = s.authCfg.negativeTTL
	}
	ttl = s.authCfg.jitterTTL(ttl)
	_ = s.authCacheL1.SetWithTTL(cacheKey, entry, 1, ttl)
}

func (s *APIKeyService) setAuthCacheEntry(ctx context.Context, cacheKey string, entry *APIKeyAuthCacheEntry, ttl time.Duration) {
	if entry == nil {
		return
	}
	s.setAuthCacheL1(cacheKey, entry)
	if s.cache == nil || !s.authCfg.l2Enabled() {
		return
	}
	_ = s.cache.SetAuthCache(ctx, cacheKey, entry, s.authCfg.jitterTTL(ttl))
}

func (s *APIKeyService) deleteAuthCache(ctx context.Context, cacheKey string) {
	if s.authCacheL1 != nil {
		s.authCacheL1.Del(cacheKey)
	}
	if s.cache == nil {
		return
	}
	_ = s.cache.DeleteAuthCache(ctx, cacheKey)
	// Publish invalidation message to other instances
	_ = s.cache.PublishAuthCacheInvalidation(ctx, cacheKey)
}

func (s *APIKeyService) loadAuthCacheEntry(ctx context.Context, key, cacheKey string) (*APIKeyAuthCacheEntry, error) {
	apiKey, err := s.apiKeyRepo.GetByKeyForAuth(ctx, key)
	if err != nil {
		if errors.Is(err, ErrAPIKeyNotFound) {
			entry := &APIKeyAuthCacheEntry{NotFound: true}
			if s.authCfg.negativeEnabled() {
				s.setAuthCacheEntry(ctx, cacheKey, entry, s.authCfg.negativeTTL)
			}
			return entry, nil
		}
		return nil, fmt.Errorf("get api key: %w", err)
	}
	apiKey.Key = key
	snapshot := s.snapshotFromAPIKey(ctx, apiKey)
	if snapshot == nil {
		return nil, fmt.Errorf("get api key: %w", ErrAPIKeyNotFound)
	}
	entry := &APIKeyAuthCacheEntry{Snapshot: snapshot}
	s.setAuthCacheEntry(ctx, cacheKey, entry, s.authCfg.l2TTL)
	return entry, nil
}

func (s *APIKeyService) applyAuthCacheEntry(key string, entry *APIKeyAuthCacheEntry) (*APIKey, bool, error) {
	if entry == nil {
		return nil, false, nil
	}
	if entry.NotFound {
		return nil, true, ErrAPIKeyNotFound
	}
	if entry.Snapshot == nil {
		return nil, false, nil
	}
	if entry.Snapshot.Version != apiKeyAuthSnapshotVersion {
		return nil, false, nil
	}
	return s.snapshotToAPIKey(key, entry.Snapshot), true, nil
}

func cloneAPIKeyAuthInt64Ptr(v *int64) *int64 {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

func cloneAPIKeyAuthFloat64Ptr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

func cloneAPIKeyAuthIntPtr(v *int) *int {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

func cloneAPIKeyAuthTimePtr(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

func cloneAPIKeyAuthStringSlice(values []string) []string {
	return append([]string(nil), values...)
}

func cloneAPIKeyAuthInt64Slice(values []int64) []int64 {
	return append([]int64(nil), values...)
}

func cloneAPIKeyAuthNotifyEmailEntries(values []NotifyEmailEntry) []NotifyEmailEntry {
	return append([]NotifyEmailEntry(nil), values...)
}

func cloneAPIKeyAuthModelRoutingSnapshot(values map[string][]int64) map[string][]int64 {
	if values == nil {
		return nil
	}
	cloned := make(map[string][]int64, len(values))
	for k, v := range values {
		cloned[k] = cloneAPIKeyAuthInt64Slice(v)
	}
	return cloned
}

func groupSnapshotFromGroup(group *Group) *APIKeyAuthGroupSnapshot {
	if group == nil {
		return nil
	}
	return &APIKeyAuthGroupSnapshot{
		ID:                              group.ID,
		Name:                            group.Name,
		Platform:                        group.Platform,
		Status:                          group.Status,
		SubscriptionType:                group.SubscriptionType,
		RateMultiplier:                  group.RateMultiplier,
		AccessMode:                      group.AccessMode,
		MinUserLevel:                    group.MinUserLevel,
		VisibleUserIDs:                  cloneAPIKeyAuthInt64Slice(group.VisibleUserIDs),
		DailyLimitUSD:                   cloneAPIKeyAuthFloat64Ptr(group.DailyLimitUSD),
		WeeklyLimitUSD:                  cloneAPIKeyAuthFloat64Ptr(group.WeeklyLimitUSD),
		MonthlyLimitUSD:                 cloneAPIKeyAuthFloat64Ptr(group.MonthlyLimitUSD),
		AllowImageGeneration:            group.AllowImageGeneration,
		ImageRateIndependent:            group.ImageRateIndependent,
		ImageRateMultiplier:             group.ImageRateMultiplier,
		ImagePrice1K:                    cloneAPIKeyAuthFloat64Ptr(group.ImagePrice1K),
		ImagePrice2K:                    cloneAPIKeyAuthFloat64Ptr(group.ImagePrice2K),
		ImagePrice4K:                    cloneAPIKeyAuthFloat64Ptr(group.ImagePrice4K),
		ClaudeCodeOnly:                  group.ClaudeCodeOnly,
		FallbackGroupID:                 cloneAPIKeyAuthInt64Ptr(group.FallbackGroupID),
		FallbackGroupIDOnInvalidRequest: cloneAPIKeyAuthInt64Ptr(group.FallbackGroupIDOnInvalidRequest),
		ModelRouting:                    cloneAPIKeyAuthModelRoutingSnapshot(group.ModelRouting),
		ModelRoutingEnabled:             group.ModelRoutingEnabled,
		MCPXMLInject:                    group.MCPXMLInject,
		SupportedModelScopes:            cloneAPIKeyAuthStringSlice(group.SupportedModelScopes),
		AllowMessagesDispatch:           group.AllowMessagesDispatch,
		AllowOpenAICompat:               group.AllowOpenAICompat,
		DefaultMappedModel:              group.DefaultMappedModel,
		MessagesDispatchModelConfig:     group.MessagesDispatchModelConfig,
		RPMLimit:                        group.RPMLimit,
	}
}

func groupFromAuthSnapshot(snapshot *APIKeyAuthGroupSnapshot) *Group {
	if snapshot == nil {
		return nil
	}
	return &Group{
		ID:                              snapshot.ID,
		Name:                            snapshot.Name,
		Platform:                        snapshot.Platform,
		Status:                          snapshot.Status,
		Hydrated:                        true,
		SubscriptionType:                snapshot.SubscriptionType,
		RateMultiplier:                  snapshot.RateMultiplier,
		AccessMode:                      snapshot.AccessMode,
		MinUserLevel:                    snapshot.MinUserLevel,
		VisibleUserIDs:                  cloneAPIKeyAuthInt64Slice(snapshot.VisibleUserIDs),
		DailyLimitUSD:                   cloneAPIKeyAuthFloat64Ptr(snapshot.DailyLimitUSD),
		WeeklyLimitUSD:                  cloneAPIKeyAuthFloat64Ptr(snapshot.WeeklyLimitUSD),
		MonthlyLimitUSD:                 cloneAPIKeyAuthFloat64Ptr(snapshot.MonthlyLimitUSD),
		AllowImageGeneration:            snapshot.AllowImageGeneration,
		ImageRateIndependent:            snapshot.ImageRateIndependent,
		ImageRateMultiplier:             snapshot.ImageRateMultiplier,
		ImagePrice1K:                    cloneAPIKeyAuthFloat64Ptr(snapshot.ImagePrice1K),
		ImagePrice2K:                    cloneAPIKeyAuthFloat64Ptr(snapshot.ImagePrice2K),
		ImagePrice4K:                    cloneAPIKeyAuthFloat64Ptr(snapshot.ImagePrice4K),
		ClaudeCodeOnly:                  snapshot.ClaudeCodeOnly,
		FallbackGroupID:                 cloneAPIKeyAuthInt64Ptr(snapshot.FallbackGroupID),
		FallbackGroupIDOnInvalidRequest: cloneAPIKeyAuthInt64Ptr(snapshot.FallbackGroupIDOnInvalidRequest),
		ModelRouting:                    cloneAPIKeyAuthModelRoutingSnapshot(snapshot.ModelRouting),
		ModelRoutingEnabled:             snapshot.ModelRoutingEnabled,
		MCPXMLInject:                    snapshot.MCPXMLInject,
		SupportedModelScopes:            cloneAPIKeyAuthStringSlice(snapshot.SupportedModelScopes),
		AllowMessagesDispatch:           snapshot.AllowMessagesDispatch,
		AllowOpenAICompat:               snapshot.AllowOpenAICompat,
		DefaultMappedModel:              snapshot.DefaultMappedModel,
		MessagesDispatchModelConfig:     snapshot.MessagesDispatchModelConfig,
		RPMLimit:                        snapshot.RPMLimit,
	}
}

func groupSnapshotsFromGroups(groups []*Group) []*APIKeyAuthGroupSnapshot {
	if groups == nil {
		return nil
	}
	cloned := make([]*APIKeyAuthGroupSnapshot, len(groups))
	for i, group := range groups {
		cloned[i] = groupSnapshotFromGroup(group)
	}
	return cloned
}

func groupsFromAuthSnapshots(snapshots []*APIKeyAuthGroupSnapshot) []*Group {
	if snapshots == nil {
		return nil
	}
	cloned := make([]*Group, len(snapshots))
	for i, snapshot := range snapshots {
		cloned[i] = groupFromAuthSnapshot(snapshot)
	}
	return cloned
}

func authorizedGroupSnapshotsFromAPIKey(groups []APIKeyAuthorizedGroup) []APIKeyAuthorizedGroupAuthSnapshot {
	if groups == nil {
		return nil
	}
	cloned := make([]APIKeyAuthorizedGroupAuthSnapshot, len(groups))
	for i, group := range groups {
		cloned[i] = APIKeyAuthorizedGroupAuthSnapshot{
			GroupID:  group.GroupID,
			Group:    groupSnapshotFromGroup(group.Group),
			Priority: group.Priority,
		}
	}
	return cloned
}

func authorizedGroupsFromAuthSnapshots(snapshots []APIKeyAuthorizedGroupAuthSnapshot) []APIKeyAuthorizedGroup {
	if snapshots == nil {
		return nil
	}
	cloned := make([]APIKeyAuthorizedGroup, len(snapshots))
	for i, snapshot := range snapshots {
		cloned[i] = APIKeyAuthorizedGroup{
			GroupID:  snapshot.GroupID,
			Group:    groupFromAuthSnapshot(snapshot.Group),
			Priority: snapshot.Priority,
		}
	}
	return cloned
}

func (s *APIKeyService) snapshotFromAPIKey(ctx context.Context, apiKey *APIKey) *APIKeyAuthSnapshot {
	if apiKey == nil || apiKey.User == nil {
		return nil
	}
	snapshot := &APIKeyAuthSnapshot{
		Version:          apiKeyAuthSnapshotVersion,
		APIKeyID:         apiKey.ID,
		UserID:           apiKey.UserID,
		GroupID:          cloneAPIKeyAuthInt64Ptr(apiKey.GroupID),
		GroupIDs:         cloneAPIKeyAuthInt64Slice(apiKey.GroupIDs),
		Name:             apiKey.Name,
		Status:           apiKey.Status,
		IPWhitelist:      cloneAPIKeyAuthStringSlice(apiKey.IPWhitelist),
		IPBlacklist:      cloneAPIKeyAuthStringSlice(apiKey.IPBlacklist),
		Quota:            apiKey.Quota,
		QuotaUsed:        apiKey.QuotaUsed,
		ExpiresAt:        cloneAPIKeyAuthTimePtr(apiKey.ExpiresAt),
		RateLimit5h:      apiKey.RateLimit5h,
		RateLimit1d:      apiKey.RateLimit1d,
		RateLimit7d:      apiKey.RateLimit7d,
		Group:            groupSnapshotFromGroup(apiKey.Group),
		Groups:           groupSnapshotsFromGroups(apiKey.Groups),
		AuthorizedGroups: authorizedGroupSnapshotsFromAPIKey(apiKey.AuthorizedGroups),
		User: APIKeyAuthUserSnapshot{
			ID:                         apiKey.User.ID,
			Status:                     apiKey.User.Status,
			Role:                       apiKey.User.Role,
			Level:                      apiKey.User.Level,
			AllowedGroups:              cloneAPIKeyAuthInt64Slice(apiKey.User.AllowedGroups),
			Balance:                    apiKey.User.Balance,
			Concurrency:                apiKey.User.Concurrency,
			Email:                      apiKey.User.Email,
			Username:                   apiKey.User.Username,
			BalanceNotifyEnabled:       apiKey.User.BalanceNotifyEnabled,
			BalanceNotifyThresholdType: apiKey.User.BalanceNotifyThresholdType,
			BalanceNotifyThreshold:     cloneAPIKeyAuthFloat64Ptr(apiKey.User.BalanceNotifyThreshold),
			BalanceNotifyExtraEmails:   cloneAPIKeyAuthNotifyEmailEntries(apiKey.User.BalanceNotifyExtraEmails),
			TotalRecharged:             apiKey.User.TotalRecharged,
			RPMLimit:                   apiKey.User.RPMLimit,
		},
	}

	// 填充 (user, group) RPM override —— snapshot 构建时查一次 DB，后续请求零 DB 往返。
	if apiKey.GroupID != nil && *apiKey.GroupID > 0 && s.userGroupRateRepo != nil {
		override, err := s.userGroupRateRepo.GetRPMOverrideByUserAndGroup(ctx, apiKey.UserID, *apiKey.GroupID)
		if err == nil && override != nil {
			snapshot.User.UserGroupRPMOverride = override
		}
		// 查询失败或无 override 时留 nil，checkRPM 会回退到 DB 查询
	}
	return snapshot
}

func (s *APIKeyService) snapshotToAPIKey(key string, snapshot *APIKeyAuthSnapshot) *APIKey {
	if snapshot == nil {
		return nil
	}
	apiKey := &APIKey{
		ID:               snapshot.APIKeyID,
		UserID:           snapshot.UserID,
		GroupID:          cloneAPIKeyAuthInt64Ptr(snapshot.GroupID),
		GroupIDs:         cloneAPIKeyAuthInt64Slice(snapshot.GroupIDs),
		Key:              key,
		Name:             snapshot.Name,
		Status:           snapshot.Status,
		IPWhitelist:      cloneAPIKeyAuthStringSlice(snapshot.IPWhitelist),
		IPBlacklist:      cloneAPIKeyAuthStringSlice(snapshot.IPBlacklist),
		Quota:            snapshot.Quota,
		QuotaUsed:        snapshot.QuotaUsed,
		ExpiresAt:        cloneAPIKeyAuthTimePtr(snapshot.ExpiresAt),
		RateLimit5h:      snapshot.RateLimit5h,
		RateLimit1d:      snapshot.RateLimit1d,
		RateLimit7d:      snapshot.RateLimit7d,
		Group:            groupFromAuthSnapshot(snapshot.Group),
		Groups:           groupsFromAuthSnapshots(snapshot.Groups),
		AuthorizedGroups: authorizedGroupsFromAuthSnapshots(snapshot.AuthorizedGroups),
		User: &User{
			ID:                         snapshot.User.ID,
			Status:                     snapshot.User.Status,
			Role:                       snapshot.User.Role,
			Level:                      snapshot.User.Level,
			AllowedGroups:              cloneAPIKeyAuthInt64Slice(snapshot.User.AllowedGroups),
			Balance:                    snapshot.User.Balance,
			Concurrency:                snapshot.User.Concurrency,
			Email:                      snapshot.User.Email,
			Username:                   snapshot.User.Username,
			BalanceNotifyEnabled:       snapshot.User.BalanceNotifyEnabled,
			BalanceNotifyThresholdType: snapshot.User.BalanceNotifyThresholdType,
			BalanceNotifyThreshold:     cloneAPIKeyAuthFloat64Ptr(snapshot.User.BalanceNotifyThreshold),
			BalanceNotifyExtraEmails:   cloneAPIKeyAuthNotifyEmailEntries(snapshot.User.BalanceNotifyExtraEmails),
			TotalRecharged:             snapshot.User.TotalRecharged,
			RPMLimit:                   snapshot.User.RPMLimit,
			UserGroupRPMOverride:       cloneAPIKeyAuthIntPtr(snapshot.User.UserGroupRPMOverride),
		},
	}
	s.compileAPIKeyIPRules(apiKey)
	return apiKey
}

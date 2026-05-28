package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveEffectiveAPIKeyGroupUsesDefaultForNormalRequests(t *testing.T) {
	defaultGroup := testEffectiveGroup(1, PlatformAnthropic, true, StatusActive)
	imageGroup := testEffectiveGroup(2, PlatformOpenAI, true, StatusActive)
	apiKey := &APIKey{
		GroupID: effectiveGroupPtrInt64(1),
		Group:   defaultGroup,
		AuthorizedGroups: []APIKeyAuthorizedGroup{
			{GroupID: 2, Group: imageGroup, Priority: 1},
		},
	}

	got, err := ResolveEffectiveAPIKeyGroup(apiKey, EffectiveGroupResolutionRequest{
		Method:         "POST",
		Endpoint:       "/v1/responses",
		RequestedModel: "gpt-5.4",
		Body:           []byte(`{"model":"gpt-5.4","input":"write code"}`),
	})

	require.NoError(t, err)
	require.Same(t, defaultGroup, got)
	require.Same(t, defaultGroup, apiKey.Group)
	require.Equal(t, int64(1), *apiKey.GroupID)
}

func TestResolveEffectiveAPIKeyGroupSelectsAuthorizedGroupByModelRouting(t *testing.T) {
	defaultGroup := testEffectiveGroup(1, PlatformAnthropic, true, StatusActive)
	routedGroup := testEffectiveGroup(2, PlatformAnthropic, true, StatusActive)
	routedGroup.ModelRoutingEnabled = true
	routedGroup.ModelRouting = map[string][]int64{"claude-opus-*": {99}}
	otherGroup := testEffectiveGroup(3, PlatformAnthropic, true, StatusActive)
	apiKey := &APIKey{
		GroupID: effectiveGroupPtrInt64(1),
		Group:   defaultGroup,
		AuthorizedGroups: []APIKeyAuthorizedGroup{
			{GroupID: 3, Group: otherGroup, Priority: 1},
			{GroupID: 2, Group: routedGroup, Priority: 2},
		},
	}

	got, err := ResolveEffectiveAPIKeyGroup(apiKey, EffectiveGroupResolutionRequest{
		Method:         "POST",
		Endpoint:       "/v1/messages",
		RequestedModel: "claude-opus-4-20250514",
	})

	require.NoError(t, err)
	require.Same(t, routedGroup, got)
	require.Same(t, defaultGroup, apiKey.Group)
	require.Equal(t, int64(1), *apiKey.GroupID)
}

func TestResolveEffectiveAPIKeyGroupFallsBackToFirstActiveAuthorizedGroupWithoutDefault(t *testing.T) {
	firstActive := testEffectiveGroup(2, PlatformAnthropic, true, StatusActive)
	secondActive := testEffectiveGroup(3, PlatformOpenAI, true, StatusActive)
	apiKey := &APIKey{
		AuthorizedGroups: []APIKeyAuthorizedGroup{
			{GroupID: 4, Group: testEffectiveGroup(4, PlatformAnthropic, true, StatusDisabled), Priority: 1},
			{GroupID: 2, Group: firstActive, Priority: 2},
			{GroupID: 3, Group: secondActive, Priority: 3},
		},
	}

	got, err := ResolveEffectiveAPIKeyGroup(apiKey, EffectiveGroupResolutionRequest{
		Method:         "POST",
		Endpoint:       "/v1/messages",
		RequestedModel: "claude-sonnet-4-20250514",
	})

	require.NoError(t, err)
	require.Same(t, firstActive, got)
}

func TestResolveEffectiveAPIKeyGroupSelectsGeminiGroupForV1BetaModels(t *testing.T) {
	defaultGroup := testEffectiveGroup(1, PlatformAnthropic, true, StatusActive)
	geminiGroup := testEffectiveGroup(2, PlatformGemini, true, StatusActive)
	apiKey := &APIKey{
		GroupID: effectiveGroupPtrInt64(1),
		Group:   defaultGroup,
		AuthorizedGroups: []APIKeyAuthorizedGroup{
			{GroupID: 2, Group: geminiGroup, Priority: 1},
		},
	}

	got, err := ResolveEffectiveAPIKeyGroup(apiKey, EffectiveGroupResolutionRequest{
		Method:   "GET",
		Endpoint: "/v1beta/models",
	})

	require.NoError(t, err)
	require.Same(t, geminiGroup, got)
	require.Same(t, defaultGroup, apiKey.Group)
	require.Equal(t, int64(1), *apiKey.GroupID)
}

func TestResolveEffectiveAPIKeyGroupPrefersDefaultWhenImageCapable(t *testing.T) {
	defaultGroup := testEffectiveGroup(1, PlatformOpenAI, true, StatusActive)
	otherImageGroup := testEffectiveGroup(2, PlatformOpenAI, true, StatusActive)
	apiKey := &APIKey{
		GroupID: effectiveGroupPtrInt64(1),
		Group:   defaultGroup,
		AuthorizedGroups: []APIKeyAuthorizedGroup{
			{GroupID: 2, Group: otherImageGroup, Priority: 1},
		},
	}

	got, err := ResolveEffectiveAPIKeyGroup(apiKey, EffectiveGroupResolutionRequest{
		Method:   "POST",
		Endpoint: "/v1/images/generations",
	})

	require.NoError(t, err)
	require.Same(t, defaultGroup, got)
	require.Same(t, defaultGroup, apiKey.Group)
	require.Equal(t, int64(1), *apiKey.GroupID)
}

func TestResolveEffectiveAPIKeyGroupSelectsFirstAuthorizedImageGroup(t *testing.T) {
	defaultGroup := testEffectiveGroup(1, PlatformAnthropic, false, StatusActive)
	firstImageGroup := testEffectiveGroup(2, PlatformOpenAI, true, StatusActive)
	secondImageGroup := testEffectiveGroup(3, PlatformOpenAI, true, StatusActive)
	apiKey := &APIKey{
		GroupID: effectiveGroupPtrInt64(1),
		Group:   defaultGroup,
		AuthorizedGroups: []APIKeyAuthorizedGroup{
			{GroupID: 2, Group: firstImageGroup, Priority: 10},
			{GroupID: 3, Group: secondImageGroup, Priority: 20},
		},
	}

	got, err := ResolveEffectiveAPIKeyGroup(apiKey, EffectiveGroupResolutionRequest{
		Method:   "POST",
		Endpoint: "/v1/responses",
		BodyMap: map[string]any{
			"model": "gpt-5.4",
			"tools": []any{map[string]any{"type": "image_generation"}},
		},
	})

	require.NoError(t, err)
	require.Same(t, firstImageGroup, got)
	require.Same(t, defaultGroup, apiKey.Group)
	require.Equal(t, int64(1), *apiKey.GroupID)
}

func TestResolveEffectiveAPIKeyGroupReturnsErrorWhenNoImageGroupAuthorized(t *testing.T) {
	apiKey := &APIKey{
		GroupID: effectiveGroupPtrInt64(1),
		Group:   testEffectiveGroup(1, PlatformAnthropic, false, StatusActive),
		AuthorizedGroups: []APIKeyAuthorizedGroup{
			{GroupID: 2, Group: testEffectiveGroup(2, PlatformOpenAI, false, StatusActive), Priority: 1},
		},
	}

	got, err := ResolveEffectiveAPIKeyGroup(apiKey, EffectiveGroupResolutionRequest{
		Method:   "POST",
		Endpoint: "/v1/images/edits",
	})

	require.Nil(t, got)
	require.True(t, errors.Is(err, ErrImageGroupNotAuthorized))
	require.Equal(t, int64(1), *apiKey.GroupID)
}

func TestResolveEffectiveAPIKeyGroupSkipsUnavailableAuthorizedGroups(t *testing.T) {
	defaultGroup := testEffectiveGroup(1, PlatformAnthropic, false, StatusActive)
	candidate := testEffectiveGroup(5, PlatformOpenAI, true, StatusActive)
	apiKey := &APIKey{
		GroupID: effectiveGroupPtrInt64(1),
		Group:   defaultGroup,
		AuthorizedGroups: []APIKeyAuthorizedGroup{
			{GroupID: 2, Group: testEffectiveGroup(2, PlatformOpenAI, true, StatusDisabled), Priority: 1},
			{GroupID: 3, Group: testEffectiveGroup(3, PlatformAnthropic, true, StatusActive), Priority: 2},
			{GroupID: 4, Group: testEffectiveGroup(4, PlatformOpenAI, false, StatusActive), Priority: 3},
			{GroupID: 5, Group: candidate, Priority: 4},
		},
	}

	got, err := ResolveEffectiveAPIKeyGroup(apiKey, EffectiveGroupResolutionRequest{
		Method:         "POST",
		Endpoint:       "/v1/responses",
		RequestedModel: "gpt-image-2",
		Body:           []byte(`{"model":"gpt-image-2"}`),
	})

	require.NoError(t, err)
	require.Same(t, candidate, got)
	require.Same(t, defaultGroup, apiKey.Group)
	require.Equal(t, int64(1), *apiKey.GroupID)
}

func testEffectiveGroup(id int64, platform string, allowImage bool, status string) *Group {
	return &Group{
		ID:                   id,
		Platform:             platform,
		AllowImageGeneration: allowImage,
		Status:               status,
		Hydrated:             true,
	}
}

func effectiveGroupPtrInt64(v int64) *int64 {
	return &v
}

package service

import (
	"errors"
	"strings"
)

var ErrImageGroupNotAuthorized = errors.New("api key is not authorized for an active OpenAI image generation group")

type EffectiveGroupResolutionRequest struct {
	Method         string
	Endpoint       string
	RequestedModel string
	Body           []byte
	BodyMap        map[string]any
}

func ResolveEffectiveAPIKeyGroup(apiKey *APIKey, req EffectiveGroupResolutionRequest) (*Group, error) {
	if apiKey == nil {
		return nil, nil
	}
	candidates := effectiveAPIKeyGroupCandidates(apiKey)
	if isEffectiveImageGenerationIntent(req) {
		if isAuthorizedOpenAIImageGroup(apiKey.Group) {
			return apiKey.Group, nil
		}
		for _, group := range candidates {
			if isAuthorizedOpenAIImageGroup(group) {
				return group, nil
			}
		}
		return nil, ErrImageGroupNotAuthorized
	}

	requestedModel := strings.TrimSpace(req.RequestedModel)
	if requestedModel != "" {
		for _, group := range candidates {
			if isActiveAPIKeyGroup(group) && len(group.GetRoutingAccountIDs(requestedModel)) > 0 {
				return group, nil
			}
		}
	}
	if isActiveAPIKeyGroup(apiKey.Group) {
		return apiKey.Group, nil
	}
	for _, group := range candidates {
		if isActiveAPIKeyGroup(group) {
			return group, nil
		}
	}
	if apiKey.Group != nil {
		return apiKey.Group, nil
	}
	if len(candidates) > 0 {
		return candidates[0], nil
	}
	return nil, nil
}

func isEffectiveImageGenerationIntent(req EffectiveGroupResolutionRequest) bool {
	if strings.TrimSpace(req.Method) != "" && !strings.EqualFold(strings.TrimSpace(req.Method), "POST") {
		return false
	}
	if req.BodyMap != nil {
		return IsImageGenerationIntentMap(req.Endpoint, req.RequestedModel, req.BodyMap)
	}
	return IsImageGenerationIntent(req.Endpoint, req.RequestedModel, req.Body)
}

func isAuthorizedOpenAIImageGroup(group *Group) bool {
	return group != nil && group.Status == StatusActive && group.Platform == PlatformOpenAI && group.AllowImageGeneration
}

func effectiveAPIKeyGroupCandidates(apiKey *APIKey) []*Group {
	if apiKey == nil {
		return nil
	}
	seen := make(map[int64]struct{}, len(apiKey.AuthorizedGroups)+1)
	candidates := make([]*Group, 0, len(apiKey.AuthorizedGroups)+1)
	add := func(group *Group) {
		if group == nil {
			return
		}
		if group.ID > 0 {
			if _, ok := seen[group.ID]; ok {
				return
			}
			seen[group.ID] = struct{}{}
		}
		candidates = append(candidates, group)
	}
	for _, authorized := range apiKey.AuthorizedGroups {
		add(authorized.Group)
	}
	add(apiKey.Group)
	return candidates
}

func isActiveAPIKeyGroup(group *Group) bool {
	return group != nil && group.IsActive()
}

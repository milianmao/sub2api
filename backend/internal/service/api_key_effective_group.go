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
	if !isEffectiveImageGenerationIntent(req) {
		return apiKey.Group, nil
	}
	if isAuthorizedOpenAIImageGroup(apiKey.Group) {
		return apiKey.Group, nil
	}
	for _, authorized := range apiKey.AuthorizedGroups {
		if isAuthorizedOpenAIImageGroup(authorized.Group) {
			return authorized.Group, nil
		}
	}
	return nil, ErrImageGroupNotAuthorized
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

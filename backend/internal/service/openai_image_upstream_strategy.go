package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

type OpenAIImageUpstreamStrategy string

const (
	OpenAIImageUpstreamAuto            OpenAIImageUpstreamStrategy = "auto"
	OpenAIImageUpstreamOfficialImages  OpenAIImageUpstreamStrategy = "official_images"
	OpenAIImageUpstreamCodexResponses  OpenAIImageUpstreamStrategy = "codex_responses"
	OpenAIImageUpstreamChatGPTWebImage OpenAIImageUpstreamStrategy = "chatgpt_web_image"
)

const featureKeyOpenAIImageUpstream = "openai_image_upstream"

func parseOpenAIImageUpstreamStrategy(value string) (OpenAIImageUpstreamStrategy, bool) {
	switch OpenAIImageUpstreamStrategy(strings.ToLower(strings.TrimSpace(value))) {
	case "", OpenAIImageUpstreamAuto:
		return OpenAIImageUpstreamAuto, true
	case OpenAIImageUpstreamOfficialImages:
		return OpenAIImageUpstreamOfficialImages, true
	case OpenAIImageUpstreamCodexResponses:
		return OpenAIImageUpstreamCodexResponses, true
	case OpenAIImageUpstreamChatGPTWebImage:
		return OpenAIImageUpstreamChatGPTWebImage, true
	default:
		return "", false
	}
}

func NormalizeOpenAIImageUpstreamStrategy(value string) (string, error) {
	strategy, valid := parseOpenAIImageUpstreamStrategy(value)
	if !valid {
		return "", fmt.Errorf("invalid openai image upstream strategy %q", value)
	}
	return string(strategy), nil
}

func stringOverrideFromMap(values map[string]any, keys ...string) (string, bool) {
	if values == nil {
		return "", false
	}
	for _, key := range keys {
		if v, ok := values[key].(string); ok {
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}

func platformStringOverride(values map[string]any, key string, platform string) (string, bool) {
	if values == nil {
		return "", false
	}
	if v, ok := values[key].(string); ok {
		return strings.TrimSpace(v), true
	}
	raw, ok := values[key].(map[string]any)
	if !ok {
		return "", false
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return "", false
	}
	if v, ok := raw[platform].(string); ok {
		return strings.TrimSpace(v), true
	}
	return "", false
}

func (a *Account) OpenAIImageUpstreamOverride() (OpenAIImageUpstreamStrategy, bool, error) {
	if a == nil || a.Platform != PlatformOpenAI || a.Extra == nil {
		return "", false, nil
	}
	if value, ok := stringOverrideFromMap(a.Extra, featureKeyOpenAIImageUpstream); ok {
		strategy, valid := parseOpenAIImageUpstreamStrategy(value)
		if !valid {
			return "", true, fmt.Errorf("invalid openai image upstream strategy %q", value)
		}
		return strategy, true, nil
	}
	openaiConfig, _ := a.Extra[PlatformOpenAI].(map[string]any)
	if value, ok := stringOverrideFromMap(openaiConfig, featureKeyOpenAIImageUpstream); ok {
		strategy, valid := parseOpenAIImageUpstreamStrategy(value)
		if !valid {
			return "", true, fmt.Errorf("invalid openai image upstream strategy %q", value)
		}
		return strategy, true, nil
	}
	return "", false, nil
}

func (c *Channel) OpenAIImageUpstreamOverride(platform string) (OpenAIImageUpstreamStrategy, bool, error) {
	if c == nil {
		return "", false, nil
	}
	value, ok := platformStringOverride(c.FeaturesConfig, featureKeyOpenAIImageUpstream, platform)
	if !ok {
		return "", false, nil
	}
	strategy, valid := parseOpenAIImageUpstreamStrategy(value)
	if !valid {
		return "", true, fmt.Errorf("invalid openai image upstream strategy %q", value)
	}
	return strategy, true, nil
}

func (s *OpenAIGatewayService) resolveOpenAIImageUpstreamStrategy(ctx context.Context, account *Account, parsed *OpenAIImagesRequest) (OpenAIImageUpstreamStrategy, error) {
	if parsed != nil && strings.EqualFold(strings.TrimSpace(parsed.Model), openAICodexGPTImage2Model) {
		return OpenAIImageUpstreamCodexResponses, nil
	}
	if strategy, ok, err := account.OpenAIImageUpstreamOverride(); ok || err != nil {
		if err != nil {
			return "", err
		}
		return resolveOpenAIImageUpstreamAuto(account, strategy)
	}
	if s != nil && s.channelService != nil && parsed != nil && parsed.GroupID != nil {
		ch, err := s.channelService.GetChannelForGroup(ctx, *parsed.GroupID)
		if err != nil {
			slog.Warn("failed to resolve openai image upstream channel override", "group_id", *parsed.GroupID, "error", err)
		} else if strategy, ok, strategyErr := ch.OpenAIImageUpstreamOverride(PlatformOpenAI); ok || strategyErr != nil {
			if strategyErr != nil {
				return "", strategyErr
			}
			return resolveOpenAIImageUpstreamAuto(account, strategy)
		}
	}
	return resolveOpenAIImageUpstreamAuto(account, OpenAIImageUpstreamAuto)
}

func resolveOpenAIImageUpstreamAuto(account *Account, strategy OpenAIImageUpstreamStrategy) (OpenAIImageUpstreamStrategy, error) {
	if strategy == OpenAIImageUpstreamAuto || strategy == "" {
		if account != nil && account.Type == AccountTypeAPIKey {
			return OpenAIImageUpstreamOfficialImages, nil
		}
		return OpenAIImageUpstreamCodexResponses, nil
	}
	return strategy, nil
}

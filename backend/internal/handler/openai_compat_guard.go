package handler

import "github.com/Wei-Shaw/sub2api/internal/service"

func groupAllowsOpenAICompat(group *service.Group) bool {
	if group == nil {
		return false
	}
	switch group.Platform {
	case service.PlatformAnthropic, service.PlatformAntigravity:
		return group.AllowOpenAICompat
	default:
		return true
	}
}

package handler

import "github.com/Wei-Shaw/sub2api/internal/service"

func groupAllowsOpenAICompat(group *service.Group) bool {
	if group == nil {
		return false
	}
	switch group.Platform {
	case service.PlatformAnthropic, service.PlatformAntigravity:
		// Claude Code only 分组带 fallback 时允许进入 OpenAI 兼容端点：
	// 请求继续由账号选择阶段调度到 fallback 分组（上游语义）。
		if group.ClaudeCodeOnly && group.FallbackGroupID != nil {
			return true
		}
		return group.AllowOpenAICompat
	default:
		return true
	}
}

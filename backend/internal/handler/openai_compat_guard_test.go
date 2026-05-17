package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGatewayChatCompletions_RejectsWhenOpenAICompatDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"claude-sonnet-4-5","messages":[]}`))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID:      10,
		GroupID: &groupID,
		Group: &service.Group{
			Platform:              service.PlatformAnthropic,
			AllowOpenAICompat:     false,
			ClaudeCodeOnly:        false,
			AllowMessagesDispatch: false,
		},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 1})

	h := &GatewayHandler{}
	h.ChatCompletions(c)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Equal(t, "permission_error", gjson.GetBytes(w.Body.Bytes(), "error.type").String())
	require.Contains(t, gjson.GetBytes(w.Body.Bytes(), "error.message").String(), "OpenAI compatibility")
}

func TestGatewayResponses_RejectsWhenOpenAICompatDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"claude-sonnet-4-5","input":"hello"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(2)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID:      10,
		GroupID: &groupID,
		Group: &service.Group{
			Platform:          service.PlatformAnthropic,
			AllowOpenAICompat: false,
		},
	})
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 1})

	h := &GatewayHandler{}
	h.Responses(c)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Equal(t, "permission_error", gjson.GetBytes(w.Body.Bytes(), "error.code").String())
	require.Contains(t, gjson.GetBytes(w.Body.Bytes(), "error.message").String(), "OpenAI compatibility")
}

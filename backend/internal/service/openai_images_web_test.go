package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestParseOpenAIChatGPTWebImageSSE(t *testing.T) {
	body := []byte("data: {\"type\":\"conversation_detail_metadata\",\"conversation_id\":\"conv_123\"}\n\n" +
		"data: {\"message\":{\"author\":{\"role\":\"assistant\"},\"content\":{\"parts\":[\"done\"]}}}\n\n" +
		"data: {\"message\":{\"author\":{\"role\":\"tool\"},\"metadata\":{\"async_task_type\":\"image_gen\"},\"content\":{\"content_type\":\"multimodal_text\",\"parts\":[{\"asset_pointer\":\"file-service://file_abc\"},\"sediment://sed_xyz\"]}}}\n\n")

	got, err := parseOpenAIChatGPTWebImageSSE(body)

	require.NoError(t, err)
	require.Equal(t, "conv_123", got.ConversationID)
	require.Equal(t, []string{"file_abc"}, got.FileIDs)
	require.Equal(t, []string{"sed_xyz"}, got.SedimentIDs)
	require.Equal(t, "done", got.AssistantText)
	require.False(t, got.Blocked)
}

func TestParseOpenAIChatGPTWebImageSSEBlockedOnly(t *testing.T) {
	body := []byte("data: {\"type\":\"moderation_blocked\"}\n\n")

	got, err := parseOpenAIChatGPTWebImageSSE(body)

	require.NoError(t, err)
	require.True(t, got.Blocked)
}

func TestOpenAIChatGPTWebImageModelSlug(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "gpt image 2 uses web slug", model: "gpt-image-2", want: "gpt-5-3"},
		{name: "codex image 2 aliases itself", model: openAICodexGPTImage2Model, want: openAICodexGPTImage2Model},
		{name: "unknown defaults to auto", model: "unknown-model", want: "auto"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, openAIChatGPTWebImageModelSlug(tt.model))
		})
	}
}

func TestForwardOpenAIImagesChatGPTWebChallengeAllowsFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusForbidden, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader(`<html>Cloudflare challenge</html>`))},
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "oauth-token"}}
	parsed := &OpenAIImagesRequest{Endpoint: openAIImagesGenerationsEndpoint, Model: "gpt-image-2", Prompt: "draw", N: 1}

	result, err := svc.forwardOpenAIImagesChatGPTWeb(context.Background(), c, account, parsed, "")

	require.Nil(t, result)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Equal(t, http.StatusForbidden, failover.StatusCode)
	require.NotContains(t, err.Error(), "oauth-token")
}

func TestForwardOpenAIImagesChatGPTWebURLFormatReturnsDataURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	imageBytes := []byte("fake-png-bytes")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"conduit_token":"conduit_1"}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"conversation_id\":\"conv_1\"}\n\ndata: {\"message\":{\"author\":{\"role\":\"tool\"},\"metadata\":{\"async_task_type\":\"image_gen\"},\"content\":{\"content_type\":\"multimodal_text\",\"parts\":[{\"asset_pointer\":\"file-service://file_1\"}]}}}\n\n"))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"download_url":"https://download.local/image.png"}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(imageBytes))},
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "oauth-token"}}
	parsed := &OpenAIImagesRequest{Endpoint: openAIImagesGenerationsEndpoint, Model: "gpt-image-2", Prompt: "draw", N: 1, ResponseFormat: "url"}

	_, err := svc.forwardOpenAIImagesChatGPTWeb(context.Background(), c, account, parsed, "")

	require.NoError(t, err)
	url := gjson.GetBytes(rec.Body.Bytes(), "data.0.url").String()
	require.Equal(t, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(imageBytes), url)
	require.NotContains(t, url, "file://")
	require.NotContains(t, url, "localhost")
}

func TestForwardOpenAIImagesChatGPTWebStreamingEmitsCompleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	imageBytes := []byte("fake-png-bytes")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"conduit_token":"conduit_1"}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"conversation_id\":\"conv_1\"}\n\ndata: {\"message\":{\"author\":{\"role\":\"tool\"},\"metadata\":{\"async_task_type\":\"image_gen\"},\"content\":{\"content_type\":\"multimodal_text\",\"parts\":[{\"asset_pointer\":\"file-service://file_1\"}]}}}\n\n"))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"download_url":"https://download.local/image.png"}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(imageBytes))},
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "oauth-token"}}
	parsed := &OpenAIImagesRequest{Endpoint: openAIImagesGenerationsEndpoint, Model: "gpt-image-2", Prompt: "draw", N: 1, Stream: true}

	result, err := svc.forwardOpenAIImagesChatGPTWeb(context.Background(), c, account, parsed, "")

	require.NoError(t, err)
	require.True(t, result.Stream)
	require.Contains(t, rec.Body.String(), "event: image_generation.completed")
	require.Contains(t, rec.Body.String(), base64.StdEncoding.EncodeToString(imageBytes))
}

func TestForwardOpenAIImagesChatGPTWebNonStreamingReturnsBase64(t *testing.T) {
	gin.SetMode(gin.TestMode)
	imageBytes := []byte("fake-png-bytes")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"conduit_token":"conduit_1"}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_web_1"}}, Body: io.NopCloser(strings.NewReader("data: {\"conversation_id\":\"conv_1\"}\n\n" + "data: {\"message\":{\"author\":{\"role\":\"tool\"},\"metadata\":{\"async_task_type\":\"image_gen\"},\"content\":{\"content_type\":\"multimodal_text\",\"parts\":[{\"asset_pointer\":\"file-service://file_1\"}]}}}\n\n"))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"download_url":"https://download.local/image.png"}`))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(imageBytes))},
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	account := &Account{ID: 7, Name: "web", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "oauth-token"}}
	parsed := &OpenAIImagesRequest{Endpoint: openAIImagesGenerationsEndpoint, Model: "gpt-image-2", Prompt: "draw", N: 1, Size: "1024x1024", SizeTier: "1K"}

	result, err := svc.forwardOpenAIImagesChatGPTWeb(context.Background(), c, account, parsed, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, "gpt-image-2", result.Model)
	require.Equal(t, "chatgpt_web_image", result.UpstreamModel)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, base64.StdEncoding.EncodeToString(imageBytes), gjson.GetBytes(rec.Body.Bytes(), "data.0.b64_json").String())
	require.Len(t, upstream.requests, 4)
	require.Equal(t, "/backend-api/f/conversation/prepare", upstream.requests[0].URL.Path)
	require.Equal(t, "/backend-api/f/conversation", upstream.requests[1].URL.Path)
}

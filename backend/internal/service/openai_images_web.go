package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const (
	openAIChatGPTWebBaseURL             = "https://chatgpt.com"
	openAIChatGPTWebConversationPrepare = "/backend-api/f/conversation/prepare"
	openAIChatGPTWebConversation        = "/backend-api/f/conversation"
)

type openAIChatGPTWebImageClient struct {
	upstream HTTPUpstream
	account  *Account
	token    string
}

type openAIChatGPTWebImageOutput struct {
	B64JSON       string
	RevisedPrompt string
	OutputFormat  string
	Size          string
}

type openAIChatGPTWebImageSSEState struct {
	ConversationID string
	FileIDs        []string
	SedimentIDs    []string
	AssistantText  string
	Blocked        bool
}

func (s *OpenAIGatewayService) forwardOpenAIImagesChatGPTWeb(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	requestModel := strings.TrimSpace(parsed.Model)
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" {
		requestModel = mapped
	}
	if requestModel == "" {
		requestModel = "gpt-image-2"
	}
	if err := validateOpenAIImagesModel(requestModel); err != nil {
		return nil, err
	}
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	client := &openAIChatGPTWebImageClient{upstream: s.httpUpstream, account: account, token: token}
	outputs, headers, err := client.generate(ctx, parsed, requestModel)
	if err != nil {
		return nil, err
	}
	results := make([]openAIResponsesImageResult, 0, len(outputs))
	for _, output := range outputs {
		results = append(results, openAIResponsesImageResult{
			Result:        output.B64JSON,
			RevisedPrompt: output.RevisedPrompt,
			OutputFormat:  output.OutputFormat,
			Size:          output.Size,
			Model:         requestModel,
		})
	}
	body, err := buildOpenAIImagesAPIResponse(results, time.Now().Unix(), nil, openAIResponsesImageResult{Model: requestModel}, parsed.ResponseFormat)
	if err != nil {
		return nil, err
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
	return &OpenAIForwardResult{
		RequestID:        headers.Get("x-request-id"),
		Model:            requestModel,
		UpstreamModel:    "chatgpt_web_image",
		Stream:           parsed.Stream,
		ResponseHeaders:  headers.Clone(),
		Duration:         time.Since(startTime),
		ImageCount:       len(results),
		ImageSize:        parsed.SizeTier,
		ImageInputSize:   parsed.Size,
		ImageOutputSizes: openAIResponsesImageResultSizes(results),
	}, nil
}

func (c *openAIChatGPTWebImageClient) generate(ctx context.Context, parsed *OpenAIImagesRequest, requestModel string) ([]openAIChatGPTWebImageOutput, http.Header, error) {
	conduitToken, err := c.prepare(ctx, parsed, requestModel)
	if err != nil {
		return nil, nil, err
	}
	state, headers, err := c.startConversation(ctx, parsed, requestModel, conduitToken)
	if err != nil {
		return nil, nil, err
	}
	if state.Blocked {
		return nil, headers, &OpenAIImagesUpstreamError{StatusCode: http.StatusBadRequest, ErrorType: "content_policy_violation", Code: "content_policy_violation", Message: "Image generation was blocked by upstream moderation", UpstreamRequestID: headers.Get("x-request-id")}
	}
	downloadURLs, err := c.resolveDownloadURLs(ctx, state)
	if err != nil {
		return nil, headers, err
	}
	if len(downloadURLs) == 0 {
		return nil, headers, fmt.Errorf("chatgpt web image generation returned no image assets")
	}
	outputs := make([]openAIChatGPTWebImageOutput, 0, len(downloadURLs))
	for _, downloadURL := range downloadURLs {
		imageBytes, err := c.downloadBytes(ctx, downloadURL)
		if err != nil {
			return nil, headers, err
		}
		outputs = append(outputs, openAIChatGPTWebImageOutput{
			B64JSON:       base64.StdEncoding.EncodeToString(imageBytes),
			RevisedPrompt: state.AssistantText,
			OutputFormat:  "png",
			Size:          parsed.SizeTier,
		})
	}
	return outputs, headers, nil
}

func (c *openAIChatGPTWebImageClient) prepare(ctx context.Context, parsed *OpenAIImagesRequest, requestModel string) (string, error) {
	payload := map[string]any{
		"model":          openAIChatGPTWebImageModelSlug(requestModel),
		"prompt":         parsed.Prompt,
		"client_context": map[string]any{"request_id": newUUIDString()},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := c.newWebRequest(ctx, http.MethodPost, openAIChatGPTWebConversationPrepare, body)
	if err != nil {
		return "", err
	}
	resp, err := c.do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", openAIChatGPTWebStatusError(resp, respBody, "prepare chatgpt web image conversation failed")
	}
	conduitToken := strings.TrimSpace(gjson.GetBytes(respBody, "conduit_token").String())
	if conduitToken == "" {
		return "", fmt.Errorf("chatgpt web prepare response missing conduit_token")
	}
	return conduitToken, nil
}

func (c *openAIChatGPTWebImageClient) startConversation(ctx context.Context, parsed *OpenAIImagesRequest, requestModel, conduitToken string) (openAIChatGPTWebImageSSEState, http.Header, error) {
	payload := map[string]any{
		"action": "next",
		"messages": []map[string]any{{
			"id":      newUUIDString(),
			"author":  map[string]any{"role": "user"},
			"content": map[string]any{"content_type": "text", "parts": []string{parsed.Prompt}},
		}},
		"model":             openAIChatGPTWebImageModelSlug(requestModel),
		"conversation_id":   nil,
		"parent_message_id": newUUIDString(),
		"stream":            true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return openAIChatGPTWebImageSSEState{}, nil, err
	}
	req, err := c.newWebRequest(ctx, http.MethodPost, openAIChatGPTWebConversation, body)
	if err != nil {
		return openAIChatGPTWebImageSSEState{}, nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("X-Conduit-Token", conduitToken)
	resp, err := c.do(req)
	if err != nil {
		return openAIChatGPTWebImageSSEState{}, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, openAIImageMaxDownloadBytes))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return openAIChatGPTWebImageSSEState{}, resp.Header.Clone(), openAIChatGPTWebStatusError(resp, respBody, "start chatgpt web image conversation failed")
	}
	state, err := parseOpenAIChatGPTWebImageSSE(respBody)
	return state, resp.Header.Clone(), err
}

func (c *openAIChatGPTWebImageClient) resolveDownloadURLs(ctx context.Context, state openAIChatGPTWebImageSSEState) ([]string, error) {
	var urls []string
	for _, fileID := range state.FileIDs {
		downloadURL, err := c.resolveDownloadURL(ctx, fmt.Sprintf("/backend-api/files/%s/download", fileID))
		if err != nil {
			return nil, err
		}
		appendUniqueString(&urls, downloadURL)
	}
	if len(urls) > 0 || strings.TrimSpace(state.ConversationID) == "" {
		return urls, nil
	}
	for _, sedimentID := range state.SedimentIDs {
		downloadURL, err := c.resolveDownloadURL(ctx, fmt.Sprintf("/backend-api/conversation/%s/attachment/%s/download", state.ConversationID, sedimentID))
		if err != nil {
			return nil, err
		}
		appendUniqueString(&urls, downloadURL)
	}
	return urls, nil
}

func (c *openAIChatGPTWebImageClient) resolveDownloadURL(ctx context.Context, path string) (string, error) {
	req, err := c.newWebRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", openAIChatGPTWebStatusError(resp, body, "resolve chatgpt web image download url failed")
	}
	downloadURL := strings.TrimSpace(gjson.GetBytes(body, "download_url").String())
	if downloadURL == "" {
		return "", fmt.Errorf("chatgpt web image download response missing download_url")
	}
	return downloadURL, nil
}

func (c *openAIChatGPTWebImageClient) downloadBytes(ctx context.Context, downloadURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "image/*,*/*;q=0.8")
	req.Header.Set("User-Agent", openAIImageBackendUserAgent)
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		return nil, openAIChatGPTWebStatusError(resp, body, "download chatgpt web image bytes failed")
	}
	imageBytes, err := io.ReadAll(io.LimitReader(resp.Body, openAIImageMaxDownloadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(imageBytes) > openAIImageMaxDownloadBytes {
		return nil, fmt.Errorf("chatgpt web image download exceeds %d bytes", openAIImageMaxDownloadBytes)
	}
	return imageBytes, nil
}

func (c *openAIChatGPTWebImageClient) newWebRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	url := openAIChatGPTWebBaseURL + path
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", openAIImageBackendUserAgent)
	req.Header.Set("Origin", openAIChatGPTWebBaseURL)
	req.Header.Set("Referer", openAIChatGPTWebBaseURL+"/")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *openAIChatGPTWebImageClient) do(req *http.Request) (*http.Response, error) {
	proxyURL := ""
	accountID := int64(0)
	concurrency := 0
	if c.account != nil {
		accountID = c.account.ID
		concurrency = c.account.Concurrency
		if c.account.ProxyID != nil && c.account.Proxy != nil {
			proxyURL = c.account.Proxy.URL()
		}
	}
	return c.upstream.Do(req, proxyURL, accountID, concurrency)
}

func openAIChatGPTWebImageModelSlug(model string) string {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "gpt-image-2", openAICodexGPTImage2Model:
		return "gpt-5-3"
	default:
		return "auto"
	}
}

func openAIChatGPTWebStatusError(resp *http.Response, body []byte, fallback string) error {
	statusCode := 0
	requestID := ""
	if resp != nil {
		statusCode = resp.StatusCode
		requestID = strings.TrimSpace(resp.Header.Get("x-request-id"))
	}
	message := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(body))
	if message == "" {
		message = strings.TrimSpace(fallback)
	}
	if message == "" {
		message = fmt.Sprintf("chatgpt web image upstream request failed: status %d", statusCode)
	}
	if statusCode == http.StatusTooManyRequests || (statusCode == http.StatusForbidden && looksLikeOpenAIChallengeResponse(body)) {
		responseHeaders := http.Header(nil)
		if resp != nil {
			responseHeaders = resp.Header.Clone()
		}
		return &UpstreamFailoverError{StatusCode: statusCode, ResponseBody: body, ResponseHeaders: responseHeaders}
	}
	return &OpenAIImagesUpstreamError{StatusCode: statusCode, ErrorType: "upstream_error", Code: "upstream_error", Message: message, UpstreamRequestID: requestID}
}

func looksLikeOpenAIChallengeResponse(body []byte) bool {
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, "cloudflare") || strings.Contains(lower, "cf-chl") || strings.Contains(lower, "arkose") || strings.Contains(lower, "captcha") || strings.Contains(lower, "challenge")
}

func newUUIDString() string {
	return uuid.NewString()
}

var (
	openAIWebImageFilePointerPattern     = regexp.MustCompile(`file-service://([A-Za-z0-9_-]+)`)
	openAIWebImageSedimentPointerPattern = regexp.MustCompile(`sediment://([A-Za-z0-9_-]+)`)
)

func parseOpenAIChatGPTWebImageSSE(body []byte) (openAIChatGPTWebImageSSEState, error) {
	state := openAIChatGPTWebImageSSEState{}
	forEachOpenAISSEDataPayload(string(body), func(payload []byte) {
		if !gjson.ValidBytes(payload) {
			return
		}
		if cid := strings.TrimSpace(gjson.GetBytes(payload, "conversation_id").String()); cid != "" {
			state.ConversationID = cid
		}
		if cid := strings.TrimSpace(gjson.GetBytes(payload, "conversation.id").String()); cid != "" {
			state.ConversationID = cid
		}
		if gjson.GetBytes(payload, "blocked").Bool() || strings.EqualFold(gjson.GetBytes(payload, "type").String(), "moderation_blocked") {
			state.Blocked = true
		}
		message := gjson.GetBytes(payload, "message")
		if message.Exists() && message.Get("author.role").String() == "assistant" {
			parts := message.Get("content.parts")
			parts.ForEach(func(_, part gjson.Result) bool {
				if text := strings.TrimSpace(part.String()); text != "" {
					if state.AssistantText != "" {
						state.AssistantText += "\n"
					}
					state.AssistantText += text
				}
				return true
			})
		}
		extractOpenAIChatGPTWebImagePointers(payload, &state)
	})
	if len(bytes.TrimSpace(body)) > 0 && !state.Blocked && state.ConversationID == "" && len(state.FileIDs) == 0 && len(state.SedimentIDs) == 0 && state.AssistantText == "" {
		return state, fmt.Errorf("malformed chatgpt web image SSE")
	}
	return state, nil
}

func extractOpenAIChatGPTWebImagePointers(payload []byte, state *openAIChatGPTWebImageSSEState) {
	if state == nil {
		return
	}
	text := string(payload)
	for _, match := range openAIWebImageFilePointerPattern.FindAllStringSubmatch(text, -1) {
		if len(match) == 2 {
			appendUniqueString(&state.FileIDs, match[1])
		}
	}
	for _, match := range openAIWebImageSedimentPointerPattern.FindAllStringSubmatch(text, -1) {
		if len(match) == 2 {
			appendUniqueString(&state.SedimentIDs, match[1])
		}
	}
}

func appendUniqueString(values *[]string, value string) {
	value = strings.TrimSpace(value)
	if value == "" || values == nil {
		return
	}
	for _, existing := range *values {
		if existing == value {
			return
		}
	}
	*values = append(*values, value)
}

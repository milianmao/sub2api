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

type openAIChatGPTWebImageUploadRef struct {
	FileID      string
	FileName    string
	FileSize    int
	ContentType string
	Width       int
	Height      int
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
	if parsed.Stream {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Status(http.StatusOK)
		flusher, _ := c.Writer.(http.Flusher)
		for _, img := range results {
			eventName := openAIImagesStreamPrefix(parsed) + ".completed"
			payload := buildOpenAIImagesStreamCompletedPayload(eventName, img, parsed.ResponseFormat, time.Now().Unix(), nil)
			if err := s.writeOpenAIImagesStreamEvent(c, flusher, eventName, payload); err != nil {
				return nil, err
			}
		}
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
	refs, err := c.uploadReferences(ctx, c.account, c.token, parsed)
	if err != nil {
		return nil, nil, err
	}
	state, headers, err := c.startConversation(ctx, parsed, requestModel, conduitToken, refs)
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

func (c *openAIChatGPTWebImageClient) uploadReferences(ctx context.Context, account *Account, token string, parsed *OpenAIImagesRequest) ([]openAIChatGPTWebImageUploadRef, error) {
	if parsed == nil || len(parsed.Uploads) == 0 {
		return nil, nil
	}
	refs := make([]openAIChatGPTWebImageUploadRef, 0, len(parsed.Uploads))
	for _, upload := range parsed.Uploads {
		ref, err := c.uploadOne(ctx, account, token, upload)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func (c *openAIChatGPTWebImageClient) uploadOne(ctx context.Context, account *Account, token string, upload OpenAIImagesUpload) (openAIChatGPTWebImageUploadRef, error) {
	contentType := strings.TrimSpace(upload.ContentType)
	if contentType == "" {
		contentType = http.DetectContentType(upload.Data)
	}
	fileName := strings.TrimSpace(upload.FileName)
	if fileName == "" {
		fileName = "image.png"
	}
	metadata := map[string]any{
		"file_name": fileName,
		"file_size": len(upload.Data),
		"use_case":  "multimodal",
		"width":     upload.Width,
		"height":    upload.Height,
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		return openAIChatGPTWebImageUploadRef{}, err
	}
	metaReq, err := c.newWebRequest(ctx, http.MethodPost, "/backend-api/files", body)
	if err != nil {
		return openAIChatGPTWebImageUploadRef{}, err
	}
	metaResp, err := c.do(metaReq)
	if err != nil {
		return openAIChatGPTWebImageUploadRef{}, err
	}
	metaBody, _ := io.ReadAll(io.LimitReader(metaResp.Body, 2<<20))
	_ = metaResp.Body.Close()
	if metaResp.StatusCode < 200 || metaResp.StatusCode >= 300 {
		return openAIChatGPTWebImageUploadRef{}, openAIChatGPTWebStatusError(metaResp, metaBody, "create chatgpt web image upload failed")
	}
	fileID := strings.TrimSpace(gjson.GetBytes(metaBody, "file_id").String())
	uploadURL := strings.TrimSpace(gjson.GetBytes(metaBody, "upload_url").String())
	if fileID == "" || uploadURL == "" {
		return openAIChatGPTWebImageUploadRef{}, fmt.Errorf("chatgpt web image upload response missing file_id or upload_url")
	}

	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(upload.Data))
	if err != nil {
		return openAIChatGPTWebImageUploadRef{}, err
	}
	putReq.Header.Set("Content-Type", contentType)
	putReq.Header.Set("x-ms-blob-type", "BlockBlob")
	putReq.Header.Set("x-ms-version", "2020-04-08")
	putReq.Header.Set("User-Agent", openAIImageBackendUserAgent)
	putResp, err := c.do(putReq)
	if err != nil {
		return openAIChatGPTWebImageUploadRef{}, err
	}
	putBody, _ := io.ReadAll(io.LimitReader(putResp.Body, 2<<20))
	_ = putResp.Body.Close()
	if putResp.StatusCode < 200 || putResp.StatusCode >= 300 {
		return openAIChatGPTWebImageUploadRef{}, openAIChatGPTWebStatusError(putResp, putBody, "put chatgpt web image upload bytes failed")
	}

	uploadedReq, err := c.newWebRequest(ctx, http.MethodPost, fmt.Sprintf("/backend-api/files/%s/uploaded", fileID), []byte(`{}`))
	if err != nil {
		return openAIChatGPTWebImageUploadRef{}, err
	}
	uploadedResp, err := c.do(uploadedReq)
	if err != nil {
		return openAIChatGPTWebImageUploadRef{}, err
	}
	uploadedBody, _ := io.ReadAll(io.LimitReader(uploadedResp.Body, 2<<20))
	_ = uploadedResp.Body.Close()
	if uploadedResp.StatusCode < 200 || uploadedResp.StatusCode >= 300 {
		return openAIChatGPTWebImageUploadRef{}, openAIChatGPTWebStatusError(uploadedResp, uploadedBody, "mark chatgpt web image upload complete failed")
	}

	return openAIChatGPTWebImageUploadRef{FileID: fileID, FileName: fileName, FileSize: len(upload.Data), ContentType: contentType, Width: upload.Width, Height: upload.Height}, nil
}

func (c *openAIChatGPTWebImageClient) startConversation(ctx context.Context, parsed *OpenAIImagesRequest, requestModel, conduitToken string, refs []openAIChatGPTWebImageUploadRef) (openAIChatGPTWebImageSSEState, http.Header, error) {
	parts := make([]any, 0, len(refs)+1)
	attachments := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		parts = append(parts, map[string]any{"content_type": "image_asset_pointer", "asset_pointer": "file-service://" + ref.FileID, "width": ref.Width, "height": ref.Height, "size_bytes": ref.FileSize})
		attachments = append(attachments, map[string]any{"id": ref.FileID, "mimeType": ref.ContentType, "name": ref.FileName, "size": ref.FileSize, "width": ref.Width, "height": ref.Height})
	}
	parts = append(parts, parsed.Prompt)
	content := map[string]any{"content_type": "text", "parts": []string{parsed.Prompt}}
	if len(refs) > 0 {
		content = map[string]any{"content_type": "multimodal_text", "parts": parts}
	}
	metadata := map[string]any{"system_hints": []string{"picture_v2"}, "serialization_metadata": map[string]any{"custom_symbol_offsets": []any{}}}
	if len(attachments) > 0 {
		metadata["attachments"] = attachments
	}
	payload := map[string]any{
		"action": "next",
		"messages": []map[string]any{{
			"id":       newUUIDString(),
			"author":   map[string]any{"role": "user"},
			"content":  content,
			"metadata": metadata,
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
	case "gpt-image-2":
		return "gpt-5-3"
	case openAICodexGPTImage2Model:
		return openAICodexGPTImage2Model
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

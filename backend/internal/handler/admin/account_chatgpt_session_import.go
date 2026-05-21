package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ChatGPTSessionImportRequest struct {
	Content            string   `json:"content"`
	Contents           []string `json:"contents"`
	Notes              *string  `json:"notes"`
	GroupIDs           []int64  `json:"group_ids"`
	ProxyID            *int64   `json:"proxy_id"`
	Concurrency        *int     `json:"concurrency"`
	Priority           *int     `json:"priority"`
	RateMultiplier     *float64 `json:"rate_multiplier"`
	LoadFactor         *int     `json:"load_factor"`
	ExpiresAt          *int64   `json:"expires_at"`
	AutoPauseOnExpired *bool    `json:"auto_pause_on_expired"`
}

type ChatGPTSessionImportResult struct {
	Total   int                           `json:"total"`
	Created int                           `json:"created"`
	Failed  int                           `json:"failed"`
	Items   []ChatGPTSessionImportItem    `json:"items,omitempty"`
	Errors  []ChatGPTSessionImportMessage `json:"errors,omitempty"`
}

type ChatGPTSessionImportItem struct {
	Index     int    `json:"index"`
	Name      string `json:"name,omitempty"`
	Action    string `json:"action"`
	AccountID int64  `json:"account_id,omitempty"`
	Message   string `json:"message,omitempty"`
}

type ChatGPTSessionImportMessage struct {
	Index   int    `json:"index"`
	Name    string `json:"name,omitempty"`
	Message string `json:"message"`
}

type chatgptSessionImportEntry struct {
	Index int
	Value any
}

type chatgptSessionImportAccount struct {
	Name           string
	Platform       string
	AccountType    string
	Credentials    map[string]any
	Extra          map[string]any
	TokenExpiresAt *time.Time
}

func (h *AccountHandler) ImportChatGPTSession(c *gin.Context) {
	var req ChatGPTSessionImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.Concurrency != nil && *req.Concurrency < 0 {
		response.BadRequest(c, "concurrency must be >= 0")
		return
	}
	if req.Priority != nil && *req.Priority < 0 {
		response.BadRequest(c, "priority must be >= 0")
		return
	}
	if req.RateMultiplier != nil && *req.RateMultiplier < 0 {
		response.BadRequest(c, "rate_multiplier must be >= 0")
		return
	}
	if req.LoadFactor != nil && *req.LoadFactor > 10000 {
		response.BadRequest(c, "load_factor must be <= 10000")
		return
	}

	entries, err := parseChatGPTSessionImportEntries(req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if len(entries) == 0 {
		response.BadRequest(c, "请输入 ChatGPT Session JSON")
		return
	}

	executeAdminIdempotentJSON(c, "admin.accounts.import_chatgpt_session", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.importChatGPTSessions(ctx, req, entries)
	})
}

func (h *AccountHandler) importChatGPTSessions(ctx context.Context, req ChatGPTSessionImportRequest, entries []chatgptSessionImportEntry) (ChatGPTSessionImportResult, error) {
	result := ChatGPTSessionImportResult{
		Total: len(entries),
		Items: make([]ChatGPTSessionImportItem, 0, len(entries)),
	}

	concurrency := 3
	if req.Concurrency != nil {
		concurrency = *req.Concurrency
	}
	priority := 50
	if req.Priority != nil {
		priority = *req.Priority
	}

	for _, entry := range entries {
		item, err := normalizeChatGPTSessionImportEntry(entry)
		if err != nil {
			result.Failed++
			result.Items = append(result.Items, ChatGPTSessionImportItem{
				Index:   entry.Index,
				Action:  "failed",
				Message: err.Error(),
			})
			result.Errors = append(result.Errors, ChatGPTSessionImportMessage{
				Index:   entry.Index,
				Message: err.Error(),
			})
			continue
		}

		accountExpiresAt, autoPauseOnExpired, warnings, err := resolveChatGPTSessionImportExpiry(req, item)
		if err != nil {
			result.Failed++
			result.Items = append(result.Items, ChatGPTSessionImportItem{
				Index:   entry.Index,
				Name:    item.Name,
				Action:  "failed",
				Message: err.Error(),
			})
			result.Errors = append(result.Errors, ChatGPTSessionImportMessage{
				Index:   entry.Index,
				Name:    item.Name,
				Message: err.Error(),
			})
			continue
		}

		extra := cloneChatGPTImportMap(item.Extra)
		if len(warnings) > 0 {
			extra["warnings"] = append([]string(nil), warnings...)
		}

		payload := DataPayload{
			Accounts: []DataAccount{
				{
					Name:               item.Name,
					Notes:              req.Notes,
					Platform:           item.Platform,
					Type:               item.AccountType,
					Credentials:        cloneChatGPTImportMap(item.Credentials),
					Extra:              extra,
					Concurrency:        concurrency,
					Priority:           priority,
					RateMultiplier:     req.RateMultiplier,
					ExpiresAt:          accountExpiresAt,
					AutoPauseOnExpired: autoPauseOnExpired,
				},
			},
			Proxies: []DataProxy{},
		}

		importResult, importErr := h.importDataPayload(ctx, payload, dataImportOptions{
			GroupIDs:             append([]int64(nil), req.GroupIDs...),
			ProxyID:              req.ProxyID,
			LoadFactor:           req.LoadFactor,
			SkipDefaultGroupBind: true,
		})
		if importErr != nil {
			result.Failed++
			result.Items = append(result.Items, ChatGPTSessionImportItem{
				Index:   entry.Index,
				Name:    item.Name,
				Action:  "failed",
				Message: importErr.Error(),
			})
			result.Errors = append(result.Errors, ChatGPTSessionImportMessage{
				Index:   entry.Index,
				Name:    item.Name,
				Message: importErr.Error(),
			})
			continue
		}
		if importResult.AccountFailed > 0 {
			message := "导入失败"
			if len(importResult.Errors) > 0 {
				message = importResult.Errors[0].Message
			}
			result.Failed++
			result.Items = append(result.Items, ChatGPTSessionImportItem{
				Index:   entry.Index,
				Name:    item.Name,
				Action:  "failed",
				Message: message,
			})
			result.Errors = append(result.Errors, ChatGPTSessionImportMessage{
				Index:   entry.Index,
				Name:    item.Name,
				Message: message,
			})
			continue
		}

		result.Created++
		result.Items = append(result.Items, ChatGPTSessionImportItem{
			Index:     entry.Index,
			Name:      item.Name,
			Action:    "created",
			AccountID: int64(result.Created),
		})
	}

	return result, nil
}

func parseChatGPTSessionImportEntries(req ChatGPTSessionImportRequest) ([]chatgptSessionImportEntry, error) {
	contents := make([]string, 0, 1+len(req.Contents))
	if strings.TrimSpace(req.Content) != "" {
		contents = append(contents, req.Content)
	}
	for _, content := range req.Contents {
		if strings.TrimSpace(content) != "" {
			contents = append(contents, content)
		}
	}

	var entries []chatgptSessionImportEntry
	for _, content := range contents {
		values, err := parseChatGPTSessionImportContent(content)
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			entries = append(entries, chatgptSessionImportEntry{
				Index: len(entries) + 1,
				Value: value,
			})
		}
	}
	return entries, nil
}

func parseChatGPTSessionImportContent(content string) ([]any, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, nil
	}

	if looksLikeJSON(trimmed) {
		values, err := decodeChatGPTJSONStream(trimmed)
		if err != nil {
			if strings.Contains(trimmed, "\n") {
				if lineValues, lineErr := parseChatGPTSessionImportLines(trimmed); lineErr == nil {
					return lineValues, nil
				}
			}
			return nil, fmt.Errorf("JSON 解析失败: %w", err)
		}
		return flattenCodexImportValues(values), nil
	}

	return parseChatGPTSessionImportLines(trimmed)
}

func parseChatGPTSessionImportLines(content string) ([]any, error) {
	values := make([]any, 0)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if looksLikeJSON(line) {
			lineValues, err := decodeChatGPTJSONStream(line)
			if err != nil {
				return nil, fmt.Errorf("第 %d 行 JSON 解析失败: %w", len(values)+1, err)
			}
			values = append(values, flattenCodexImportValues(lineValues)...)
			continue
		}
		values = append(values, line)
	}
	return values, nil
}

func decodeChatGPTJSONStream(content string) ([]any, error) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	values := make([]any, 0, 1)
	for {
		var value any
		err := decoder.Decode(&value)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return nil, errors.New("空 JSON 内容")
	}
	return values, nil
}

func normalizeChatGPTSessionImportEntry(entry chatgptSessionImportEntry) (*chatgptSessionImportAccount, error) {
	now := time.Now().UTC()
	item := &chatgptSessionImportAccount{
		Platform:    service.PlatformOpenAI,
		AccountType: service.AccountTypeOAuth,
		Credentials: map[string]any{},
		Extra: map[string]any{
			"import_source": "chatgpt_session",
			"imported_at":   now.Format(time.RFC3339),
		},
	}

	switch raw := entry.Value.(type) {
	case map[string]any:
		accessToken := firstCodexString(raw,
			[]string{"accessToken"},
			[]string{"access_token"},
			[]string{"token"},
		)
		if accessToken == "" {
			return nil, errors.New("缺少 accessToken/access_token")
		}
		item.Credentials["access_token"] = accessToken

		refreshToken := firstCodexString(raw,
			[]string{"refreshToken"},
			[]string{"refresh_token"},
		)
		if refreshToken != "" {
			item.Credentials["refresh_token"] = refreshToken
			item.Credentials["client_id"] = openai.ClientID
		}

		idToken := firstCodexString(raw,
			[]string{"idToken"},
			[]string{"id_token"},
		)
		if idToken != "" {
			item.Credentials["id_token"] = idToken
			_ = enrichCodexImportAccountFromJWT(&codexImportAccount{
				Credentials: item.Credentials,
			}, idToken, false, now)
		}

		setCodexCredentialIfNotEmpty(item.Credentials, "email", firstCodexString(raw, []string{"email"}, []string{"user", "email"}))
		setCodexCredentialIfNotEmpty(item.Credentials, "chatgpt_account_id", firstCodexString(raw,
			[]string{"chatgpt_account_id"},
			[]string{"chatgptAccountId"},
			[]string{"account", "id"},
			[]string{"account", "account_id"},
		))
		setCodexCredentialIfNotEmpty(item.Credentials, "chatgpt_user_id", firstCodexString(raw,
			[]string{"chatgpt_user_id"},
			[]string{"chatgptUserId"},
			[]string{"user", "id"},
			[]string{"user_id"},
		))
		setCodexCredentialIfNotEmpty(item.Credentials, "plan_type", firstCodexString(raw,
			[]string{"plan_type"},
			[]string{"planType"},
			[]string{"account", "planType"},
			[]string{"account", "plan_type"},
		))
		setCodexCredentialIfNotEmpty(item.Credentials, "organization_id", firstCodexString(raw,
			[]string{"organization_id"},
			[]string{"organizationId"},
			[]string{"org_id"},
		))

		if sessionExpiresAt, ok := firstCodexTime(raw,
			[]string{"expires"},
			[]string{"expires_at"},
			[]string{"expiresAt"},
		); ok {
			item.TokenExpiresAt = &sessionExpiresAt
		}

		if err := enrichChatGPTSessionFromAccessToken(item, accessToken, now); err != nil {
			return nil, err
		}
	case string:
		accessToken := strings.TrimSpace(raw)
		if accessToken == "" {
			return nil, errors.New("缺少 accessToken/access_token")
		}
		item.Credentials["access_token"] = accessToken
		if err := enrichChatGPTSessionFromAccessToken(item, accessToken, now); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("第 %d 条格式不支持", entry.Index)
	}

	email := codexCredentialString(item.Credentials, "email")
	accountID := codexCredentialString(item.Credentials, "chatgpt_account_id")
	userID := codexCredentialString(item.Credentials, "chatgpt_user_id")
	item.Name = buildChatGPTSessionImportAccountName(email, accountID, userID, entry.Index)
	return item, nil
}

func enrichChatGPTSessionFromAccessToken(item *chatgptSessionImportAccount, token string, now time.Time) error {
	if item == nil {
		return nil
	}
	claims, err := decodeCodexJWTClaims(token)
	if err != nil {
		return nil
	}
	if claims.Exp > 0 {
		if now.Unix() > claims.Exp+codexImportClockSkewSeconds {
			return fmt.Errorf("access_token 已过期: %s", time.Unix(claims.Exp, 0).UTC().Format(time.RFC3339))
		}
		expiresAt := time.Unix(claims.Exp, 0).UTC()
		item.TokenExpiresAt = &expiresAt
		item.Credentials["expires_at"] = expiresAt.Format(time.RFC3339)
	}
	if codexCredentialString(item.Credentials, "email") == "" {
		setCodexCredentialIfNotEmpty(item.Credentials, "email", claims.Email)
	}
	if claims.OpenAIAuth != nil {
		if codexCredentialString(item.Credentials, "chatgpt_account_id") == "" {
			setCodexCredentialIfNotEmpty(item.Credentials, "chatgpt_account_id", claims.OpenAIAuth.ChatGPTAccountID)
		}
		if codexCredentialString(item.Credentials, "chatgpt_user_id") == "" {
			if userID := strings.TrimSpace(claims.OpenAIAuth.ChatGPTUserID); userID != "" {
				setCodexCredentialIfNotEmpty(item.Credentials, "chatgpt_user_id", userID)
			} else {
				setCodexCredentialIfNotEmpty(item.Credentials, "chatgpt_user_id", claims.OpenAIAuth.UserID)
			}
		}
		if codexCredentialString(item.Credentials, "plan_type") == "" {
			setCodexCredentialIfNotEmpty(item.Credentials, "plan_type", claims.OpenAIAuth.ChatGPTPlanType)
		}
		if orgID := strings.TrimSpace(claims.OpenAIAuth.POID); orgID != "" {
			if codexCredentialString(item.Credentials, "organization_id") == "" {
				setCodexCredentialIfNotEmpty(item.Credentials, "organization_id", orgID)
			}
		} else {
			if codexCredentialString(item.Credentials, "organization_id") == "" {
				for _, org := range claims.OpenAIAuth.Organizations {
					if org.IsDefault {
						setCodexCredentialIfNotEmpty(item.Credentials, "organization_id", org.ID)
						break
					}
				}
			}
			if codexCredentialString(item.Credentials, "organization_id") == "" && len(claims.OpenAIAuth.Organizations) > 0 {
				setCodexCredentialIfNotEmpty(item.Credentials, "organization_id", claims.OpenAIAuth.Organizations[0].ID)
			}
		}
	} else if err := enrichCodexImportAccountFromJWT(&codexImportAccount{Credentials: item.Credentials}, token, true, now); err != nil {
		return err
	}
	if expiresAtRaw, ok := item.Credentials["expires_at"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, expiresAtRaw); err == nil {
			parsed = parsed.UTC()
			item.TokenExpiresAt = &parsed
		}
	}
	return nil
}

func resolveChatGPTSessionImportExpiry(req ChatGPTSessionImportRequest, item *chatgptSessionImportAccount) (*int64, *bool, []string, error) {
	if item == nil {
		return nil, nil, nil, errors.New("导入项为空")
	}

	requestExpiresAt := req.ExpiresAt
	var derivedExpiresAt *int64
	if item.TokenExpiresAt != nil {
		unix := item.TokenExpiresAt.Unix()
		derivedExpiresAt = &unix
	}
	if requestExpiresAt != nil && *requestExpiresAt > 0 {
		if derivedExpiresAt == nil || *requestExpiresAt < *derivedExpiresAt {
			derivedExpiresAt = requestExpiresAt
		}
	}

	if _, hasRefreshToken := item.Credentials["refresh_token"]; !hasRefreshToken {
		if derivedExpiresAt == nil || *derivedExpiresAt <= 0 {
			return nil, nil, nil, errors.New("缺少 refresh_token，且无法推导过期时间")
		}
		autoPause := true
		if req.AutoPauseOnExpired != nil {
			autoPause = *req.AutoPauseOnExpired
		}
		warnings := []string{"缺少 refresh_token，已按可用过期时间导入"}
		return derivedExpiresAt, &autoPause, warnings, nil
	}

	if requestExpiresAt != nil && *requestExpiresAt > 0 {
		return requestExpiresAt, req.AutoPauseOnExpired, nil, nil
	}
	return nil, req.AutoPauseOnExpired, nil, nil
}

func buildChatGPTSessionImportAccountName(email, accountID, userID string, index int) string {
	for _, candidate := range []string{email, accountID, userID} {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			return candidate
		}
	}
	return fmt.Sprintf("ChatGPT Session 导入账号 %d", index)
}

func cloneChatGPTImportMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

package admin

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupChatGPTSessionImportRouter() (*gin.Engine, *stubAdminService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminSvc := newStubAdminService()

	h := NewAccountHandler(
		adminSvc,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	router.POST("/api/v1/admin/accounts/import/chatgpt-session", h.ImportChatGPTSession)
	return router, adminSvc
}

func TestParseChatGPTSessionImportEntriesSupportsSingleArrayLinesAndContents(t *testing.T) {
	req := ChatGPTSessionImportRequest{
		Content: strings.Join([]string{
			`{"accessToken":"token-1"}`,
			`{"accessToken":"token-2"}`,
		}, "\n"),
		Contents: []string{
			`[{"accessToken":"token-3"},{"accessToken":"token-4"}]`,
			`{"accessToken":"token-5"}`,
		},
	}

	entries, err := parseChatGPTSessionImportEntries(req)
	require.NoError(t, err)
	require.Len(t, entries, 5)
}

func TestNormalizeChatGPTSessionImportEntryExtractsPreferredFields(t *testing.T) {
	accessToken := buildChatGPTSessionTestJWT(t, time.Now().Add(time.Hour), map[string]any{
		"email": "claim@example.com",
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct-from-claim",
			"chatgpt_user_id":    "user-from-claim",
			"chatgpt_plan_type":  "team",
			"poid":               "org-from-claim",
		},
	})

	raw := map[string]any{
		"user": map[string]any{
			"id":    "user-from-json",
			"name":  "Session User",
			"email": "json@example.com",
		},
		"account": map[string]any{
			"id":       "acct-from-json",
			"planType": "plus",
		},
		"accessToken":  accessToken,
		"refreshToken": "refresh-token",
		"idToken":      buildChatGPTSessionTestJWT(t, time.Now().Add(2*time.Hour), map[string]any{"email": "id@example.com"}),
		"expires":      "2026-08-05T13:40:42.836Z",
	}

	item, err := normalizeChatGPTSessionImportEntry(chatgptSessionImportEntry{Index: 1, Value: raw})
	require.NoError(t, err)
	require.Equal(t, service.PlatformOpenAI, item.Platform)
	require.Equal(t, service.AccountTypeOAuth, item.AccountType)
	require.Equal(t, "json@example.com", item.Credentials["email"])
	require.Equal(t, "acct-from-json", item.Credentials["chatgpt_account_id"])
	require.Equal(t, "user-from-json", item.Credentials["chatgpt_user_id"])
	require.Equal(t, "plus", item.Credentials["plan_type"])
	require.Equal(t, "org-from-claim", item.Credentials["organization_id"])
	require.Equal(t, "refresh-token", item.Credentials["refresh_token"])
	require.Equal(t, "chatgpt_session", item.Extra["import_source"])
	require.NotNil(t, item.TokenExpiresAt)
}

func TestResolveChatGPTSessionImportExpiryAllowsTokenExpiryWithoutRefreshToken(t *testing.T) {
	tokenExpiresAt := time.Now().Add(time.Hour).UTC()
	item := &chatgptSessionImportAccount{
		Credentials:    map[string]any{"access_token": "access-token"},
		TokenExpiresAt: &tokenExpiresAt,
	}

	accountExpiresAt, autoPause, warnings, err := resolveChatGPTSessionImportExpiry(ChatGPTSessionImportRequest{}, item)
	require.NoError(t, err)
	require.NotNil(t, accountExpiresAt)
	require.Equal(t, tokenExpiresAt.Unix(), *accountExpiresAt)
	require.NotNil(t, autoPause)
	require.True(t, *autoPause)
	require.NotEmpty(t, warnings)
}

func TestResolveChatGPTSessionImportExpiryRequiresExpiryWithoutRefreshToken(t *testing.T) {
	item := &chatgptSessionImportAccount{
		Credentials: map[string]any{"access_token": "opaque-token"},
	}

	_, _, _, err := resolveChatGPTSessionImportExpiry(ChatGPTSessionImportRequest{}, item)
	require.Error(t, err)
	require.Contains(t, err.Error(), "无法推导过期时间")
}

func TestImportChatGPTSessionCreatesAccountsAndPropagatesBatchOptions(t *testing.T) {
	router, adminSvc := setupChatGPTSessionImportRouter()
	adminSvc.accounts = []service.Account{
		{
			ID:       99,
			Name:     "existing",
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
			Credentials: map[string]any{
				"email":              "dup@example.com",
				"chatgpt_account_id": "acct-dup",
				"chatgpt_user_id":    "user-dup",
			},
			Status: service.StatusActive,
		},
	}

	now := time.Now().Add(2 * time.Hour).UTC()
	accessToken := buildChatGPTSessionTestJWT(t, now, map[string]any{
		"email": "dup@example.com",
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": "acct-dup",
			"chatgpt_user_id":    "user-dup",
			"chatgpt_plan_type":  "plus",
			"poid":               "org-dup",
		},
	})
	notes := "batch-notes"
	proxyID := int64(23)
	concurrency := 7
	priority := 88
	rateMultiplier := 1.25
	loadFactor := 17
	expiresAt := now.Add(-30 * time.Minute).Unix()
	autoPauseOnExpired := false

	payload := ChatGPTSessionImportRequest{
		Content: `{"accessToken":"` + accessToken + `"}`,
		Contents: []string{
			`{"accessToken":"` + accessToken + `"}`,
		},
		Notes:              &notes,
		GroupIDs:           []int64{3, 4},
		ProxyID:            &proxyID,
		Concurrency:        &concurrency,
		Priority:           &priority,
		RateMultiplier:     &rateMultiplier,
		LoadFactor:         &loadFactor,
		ExpiresAt:          &expiresAt,
		AutoPauseOnExpired: &autoPauseOnExpired,
	}

	body, err := json.Marshal(payload)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/import/chatgpt-session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.createdAccounts, 2)
	for _, created := range adminSvc.createdAccounts {
		require.Equal(t, notes, derefString(created.Notes))
		require.Equal(t, &proxyID, created.ProxyID)
		require.Equal(t, concurrency, created.Concurrency)
		require.Equal(t, priority, created.Priority)
		require.Equal(t, &rateMultiplier, created.RateMultiplier)
		require.Equal(t, &loadFactor, created.LoadFactor)
		require.Equal(t, []int64{3, 4}, created.GroupIDs)
		require.NotNil(t, created.ExpiresAt)
		require.Equal(t, expiresAt, *created.ExpiresAt)
		require.NotNil(t, created.AutoPauseOnExpired)
		require.False(t, *created.AutoPauseOnExpired)
		require.True(t, created.SkipDefaultGroupBind)
		require.Equal(t, service.PlatformOpenAI, created.Platform)
		require.Equal(t, service.AccountTypeOAuth, created.Type)
		require.Equal(t, "chatgpt_session", created.Extra["import_source"])
	}
}

func TestImportChatGPTSessionRouteReturnsPerItemFailuresWithoutStoppingBatch(t *testing.T) {
	router, adminSvc := setupChatGPTSessionImportRouter()
	adminSvc.accounts = nil

	validToken := buildChatGPTSessionTestJWT(t, time.Now().Add(time.Hour), map[string]any{
		"email": "ok@example.com",
	})

	payload := ChatGPTSessionImportRequest{
		Content: strings.Join([]string{
			`{"accessToken":"` + validToken + `"}`,
			`{"accessToken":"opaque-token-without-expiry"}`,
		}, "\n"),
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/import/chatgpt-session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Code int                        `json:"code"`
		Data ChatGPTSessionImportResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Equal(t, 2, resp.Data.Total)
	require.Equal(t, 1, resp.Data.Created)
	require.Equal(t, 1, resp.Data.Failed)
	require.Len(t, resp.Data.Items, 2)
	require.Len(t, resp.Data.Errors, 1)
	require.Contains(t, resp.Data.Errors[0].Message, "无法推导过期时间")
}

func buildChatGPTSessionTestJWT(t *testing.T, exp time.Time, extraClaims map[string]any) string {
	t.Helper()
	header := map[string]any{
		"alg": "none",
		"typ": "JWT",
	}
	claims := map[string]any{
		"sub": "user-from-sub",
		"exp": exp.Unix(),
		"iat": time.Now().Unix(),
	}
	for k, v := range extraClaims {
		claims[k] = v
	}

	headerBytes, err := json.Marshal(header)
	require.NoError(t, err)
	claimBytes, err := json.Marshal(claims)
	require.NoError(t, err)

	return base64.RawURLEncoding.EncodeToString(headerBytes) + "." + base64.RawURLEncoding.EncodeToString(claimBytes) + "."
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

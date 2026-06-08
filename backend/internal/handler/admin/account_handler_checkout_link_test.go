package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

type checkoutLinkAdminService struct {
	*stubAdminService
	account service.Account
}

func (s *checkoutLinkAdminService) GetAccount(_ context.Context, id int64) (*service.Account, error) {
	if s.account.ID != id {
		return nil, errors.New("not found")
	}
	account := s.account
	return &account, nil
}

func setupCheckoutLinkRouter(adminSvc service.AdminService, oauthSvc *service.OpenAIOAuthService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, oauthSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/accounts/:id/checkout-link", handler.CreateCheckoutLink)
	return router
}

func TestAccountHandlerCreateCheckoutLink_RejectsNonOpenAIOAuthAccount(t *testing.T) {
	adminSvc := &checkoutLinkAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       43,
			Platform: service.PlatformAnthropic,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
		},
	}
	router := setupCheckoutLinkRouter(adminSvc, service.NewOpenAIOAuthService(nil, &checkoutLinkOAuthClient{}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/43/checkout-link", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAccountHandlerCreateCheckoutLink_RequiresAccessToken(t *testing.T) {
	adminSvc := &checkoutLinkAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       43,
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
		},
	}
	router := setupCheckoutLinkRouter(adminSvc, service.NewOpenAIOAuthService(nil, &checkoutLinkOAuthClient{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/43/checkout-link", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAccountHandlerCreateCheckoutLink_ReturnsPlainTextURL(t *testing.T) {
	checkoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"https://chatgpt.com/payments/checkout/session-1"}`))
	}))
	defer checkoutServer.Close()

	oldURL := service.SetChatGPTCheckoutURLForTest(checkoutServer.URL)
	defer service.SetChatGPTCheckoutURLForTest(oldURL)

	proxyID := int64(7)
	adminSvc := &checkoutLinkAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       43,
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
			ProxyID:  &proxyID,
			Credentials: map[string]any{
				"access_token": "access-token-1",
			},
		},
	}
	oauthSvc := service.NewOpenAIOAuthService(&checkoutLinkProxyRepo{}, &checkoutLinkOAuthClient{})
	oauthSvc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		require.Equal(t, "http://127.0.0.1:8080", proxyURL)
		return req.C(), nil
	})
	router := setupCheckoutLinkRouter(adminSvc, oauthSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/43/checkout-link", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	require.Equal(t, "https://chatgpt.com/payments/checkout/session-1", rec.Body.String())
}

func TestAccountHandlerCreateCheckoutLink_ForwardsAccountCookies(t *testing.T) {
	checkoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "foo=bar; oai-did=device-123", r.Header.Get("Cookie"))
		require.Equal(t, "device-123", r.Header.Get("oai-device-id"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"https://chatgpt.com/payments/checkout/session-1"}`))
	}))
	defer checkoutServer.Close()

	oldURL := service.SetChatGPTCheckoutURLForTest(checkoutServer.URL)
	defer service.SetChatGPTCheckoutURLForTest(oldURL)

	adminSvc := &checkoutLinkAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       43,
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"access_token": "access-token-1",
				"cookies":      "foo=bar; oai-did=device-123",
			},
		},
	}
	oauthSvc := service.NewOpenAIOAuthService(nil, &checkoutLinkOAuthClient{})
	oauthSvc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		return req.C(), nil
	})
	router := setupCheckoutLinkRouter(adminSvc, oauthSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/43/checkout-link", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	require.Equal(t, "https://chatgpt.com/payments/checkout/session-1", rec.Body.String())
}

func TestAccountHandlerCreateCheckoutLink_UsesCompatibleCredentialAliases(t *testing.T) {
	checkoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer token-from-account", r.Header.Get("Authorization"))
		require.Equal(t, "oai-did=device-123; foo=bar", r.Header.Get("Cookie"))
		require.Equal(t, "device-123", r.Header.Get("oai-device-id"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"https://chatgpt.com/payments/checkout/session-1"}`))
	}))
	defer checkoutServer.Close()

	oldURL := service.SetChatGPTCheckoutURLForTest(checkoutServer.URL)
	defer service.SetChatGPTCheckoutURLForTest(oldURL)

	adminSvc := &checkoutLinkAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:       43,
			Platform: service.PlatformOpenAI,
			Type:     service.AccountTypeOAuth,
			Status:   service.StatusActive,
			Credentials: map[string]any{
				"token":  "token-from-account",
				"cookie": "oai-did=device-123; foo=bar",
			},
		},
	}
	oauthSvc := service.NewOpenAIOAuthService(nil, &checkoutLinkOAuthClient{})
	oauthSvc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		return req.C(), nil
	})
	router := setupCheckoutLinkRouter(adminSvc, oauthSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/43/checkout-link", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "https://chatgpt.com/payments/checkout/session-1", rec.Body.String())
}

func TestAccountHandlerCreateCheckoutLink_ReturnsTrialEligibilityMessage(t *testing.T) {
	checkoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"amount_total":0,"currency":"usd"}`))
	}))
	defer checkoutServer.Close()

	oldURL := service.SetChatGPTCheckoutURLForTest(checkoutServer.URL)
	defer service.SetChatGPTCheckoutURLForTest(oldURL)

	adminSvc := &checkoutLinkAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:          43,
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeOAuth,
			Status:      service.StatusActive,
			Credentials: map[string]any{"access_token": "access-token-1"},
		},
	}
	oauthSvc := service.NewOpenAIOAuthService(nil, &checkoutLinkOAuthClient{})
	oauthSvc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) { return req.C(), nil })
	router := setupCheckoutLinkRouter(adminSvc, oauthSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/43/checkout-link", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	require.Equal(t, "账号具备 0 元试用资格", rec.Body.String())
}

func TestAccountHandlerCreateCheckoutLink_ReturnsAlreadySubscribedMessage(t *testing.T) {
	checkoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"detail":"already subscribed"}`))
	}))
	defer checkoutServer.Close()

	oldURL := service.SetChatGPTCheckoutURLForTest(checkoutServer.URL)
	defer service.SetChatGPTCheckoutURLForTest(oldURL)

	adminSvc := &checkoutLinkAdminService{
		stubAdminService: newStubAdminService(),
		account: service.Account{
			ID:          43,
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeOAuth,
			Status:      service.StatusActive,
			Credentials: map[string]any{"access_token": "access-token-1"},
		},
	}
	oauthSvc := service.NewOpenAIOAuthService(nil, &checkoutLinkOAuthClient{})
	oauthSvc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) { return req.C(), nil })
	router := setupCheckoutLinkRouter(adminSvc, oauthSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/43/checkout-link", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	require.Equal(t, "账号已订阅，无需生成支付链接", rec.Body.String())
}

type checkoutLinkOAuthClient struct{}

func (c *checkoutLinkOAuthClient) ExchangeCode(context.Context, string, string, string, string, string) (*openai.TokenResponse, error) {
	return nil, errors.New("not implemented")
}

func (c *checkoutLinkOAuthClient) RefreshToken(context.Context, string, string) (*openai.TokenResponse, error) {
	return nil, errors.New("not implemented")
}

func (c *checkoutLinkOAuthClient) RefreshTokenWithClientID(context.Context, string, string, string) (*openai.TokenResponse, error) {
	return nil, errors.New("not implemented")
}

type checkoutLinkProxyRepo struct{}

func (r *checkoutLinkProxyRepo) Create(context.Context, *service.Proxy) error {
	return errors.New("not implemented")
}

func (r *checkoutLinkProxyRepo) GetByID(_ context.Context, id int64) (*service.Proxy, error) {
	if id != 7 {
		return nil, service.ErrProxyNotFound
	}
	return &service.Proxy{
		ID:       id,
		Protocol: "http",
		Host:     "127.0.0.1",
		Port:     8080,
		Status:   service.StatusActive,
	}, nil
}

func (r *checkoutLinkProxyRepo) ListByIDs(context.Context, []int64) ([]service.Proxy, error) {
	return nil, errors.New("not implemented")
}

func (r *checkoutLinkProxyRepo) Update(context.Context, *service.Proxy) error {
	return errors.New("not implemented")
}

func (r *checkoutLinkProxyRepo) Delete(context.Context, int64) error {
	return errors.New("not implemented")
}

func (r *checkoutLinkProxyRepo) List(context.Context, pagination.PaginationParams) ([]service.Proxy, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *checkoutLinkProxyRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]service.Proxy, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *checkoutLinkProxyRepo) ListWithFiltersAndAccountCount(context.Context, pagination.PaginationParams, string, string, string) ([]service.ProxyWithAccountCount, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("not implemented")
}

func (r *checkoutLinkProxyRepo) ListActive(context.Context) ([]service.Proxy, error) {
	return nil, errors.New("not implemented")
}

func (r *checkoutLinkProxyRepo) ListActiveWithAccountCount(context.Context) ([]service.ProxyWithAccountCount, error) {
	return nil, errors.New("not implemented")
}

func (r *checkoutLinkProxyRepo) ExistsByHostPortAuth(context.Context, string, int, string, string) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *checkoutLinkProxyRepo) CountAccountsByProxyID(context.Context, int64) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *checkoutLinkProxyRepo) ListAccountSummariesByProxyID(context.Context, int64) ([]service.ProxyAccountSummary, error) {
	return nil, errors.New("not implemented")
}

func (r *checkoutLinkProxyRepo) SweepExpiredProxies(context.Context, time.Time) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *checkoutLinkProxyRepo) ListAllForFallback(context.Context) ([]service.Proxy, error) {
	return nil, errors.New("not implemented")
}

func (r *checkoutLinkProxyRepo) CountExpired(context.Context) (int64, error) {
	return 0, errors.New("not implemented")
}

func (r *checkoutLinkProxyRepo) CountExpiringSoon(context.Context, time.Time) (int64, error) {
	return 0, errors.New("not implemented")
}

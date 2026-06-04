package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

type checkoutLinkProxyAdminService struct {
	*stubAdminService
	proxy *service.Proxy
}

func (s *checkoutLinkProxyAdminService) GetProxy(_ context.Context, id int64) (*service.Proxy, error) {
	if s.proxy == nil || s.proxy.ID != id {
		return nil, service.ErrProxyNotFound
	}
	proxy := *s.proxy
	return &proxy, nil
}

func setupAdminCheckoutLinkRouter(adminSvc service.AdminService, oauthSvc *service.OpenAIOAuthService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewAccountHandler(adminSvc, nil, oauthSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router.POST("/api/v1/admin/chatgpt-plus-checkout", handler.CreateAdminCheckoutLink)
	return router
}

func TestAccountHandlerCreateAdminCheckoutLink_RequiresToken(t *testing.T) {
	router := setupAdminCheckoutLinkRouter(newStubAdminService(), service.NewOpenAIOAuthService(nil, &checkoutLinkOAuthClient{}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/chatgpt-plus-checkout", strings.NewReader(`{"proxy_source":"direct"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "access token is required")
}

func TestAccountHandlerCreateAdminCheckoutLink_DirectReturnsURLEnvelope(t *testing.T) {
	checkoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer access-token-1", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"https://chatgpt.com/payments/checkout/session-admin"}`))
	}))
	defer checkoutServer.Close()

	oldURL := service.SetChatGPTCheckoutURLForTest(checkoutServer.URL)
	defer service.SetChatGPTCheckoutURLForTest(oldURL)

	oauthSvc := service.NewOpenAIOAuthService(nil, &checkoutLinkOAuthClient{})
	oauthSvc.SetCheckoutCloudConfigForTest("", "")
	oauthSvc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		require.Empty(t, proxyURL)
		return req.C(), nil
	})
	router := setupAdminCheckoutLinkRouter(newStubAdminService(), oauthSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/chatgpt-plus-checkout", strings.NewReader(`{"access_token":"access-token-1","proxy_source":"direct"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"url":"https://chatgpt.com/payments/checkout/session-admin"`)
}

func TestAccountHandlerCreateAdminCheckoutLink_PoolUsesActiveProxy(t *testing.T) {
	checkoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"https://chatgpt.com/payments/checkout/session-pool"}`))
	}))
	defer checkoutServer.Close()

	oldURL := service.SetChatGPTCheckoutURLForTest(checkoutServer.URL)
	defer service.SetChatGPTCheckoutURLForTest(oldURL)

	adminSvc := &checkoutLinkProxyAdminService{
		stubAdminService: newStubAdminService(),
		proxy: &service.Proxy{ID: 7, Protocol: "http", Host: "127.0.0.1", Port: 8080, Status: service.StatusActive},
	}
	oauthSvc := service.NewOpenAIOAuthService(nil, &checkoutLinkOAuthClient{})
	oauthSvc.SetCheckoutCloudConfigForTest("", "")
	oauthSvc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		require.Equal(t, "http://127.0.0.1:8080", proxyURL)
		return req.C(), nil
	})
	router := setupAdminCheckoutLinkRouter(adminSvc, oauthSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/chatgpt-plus-checkout", strings.NewReader(`{"access_token":"access-token-1","proxy_source":"pool","proxy_id":7}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"url":"https://chatgpt.com/payments/checkout/session-pool"`)
}

func TestAccountHandlerCreateAdminCheckoutLink_RejectsInactiveProxy(t *testing.T) {
	adminSvc := &checkoutLinkProxyAdminService{
		stubAdminService: newStubAdminService(),
		proxy: &service.Proxy{ID: 7, Protocol: "http", Host: "127.0.0.1", Port: 8080, Status: "inactive"},
	}
	router := setupAdminCheckoutLinkRouter(adminSvc, service.NewOpenAIOAuthService(nil, &checkoutLinkOAuthClient{}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/chatgpt-plus-checkout", strings.NewReader(`{"access_token":"access-token-1","proxy_source":"pool","proxy_id":7}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "proxy must be active")
}

func TestAccountHandlerCreateAdminCheckoutLink_ReturnsMessageAsBadRequestWhenNoURL(t *testing.T) {
	checkoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"amount_total":0,"currency":"usd"}`))
	}))
	defer checkoutServer.Close()

	oldURL := service.SetChatGPTCheckoutURLForTest(checkoutServer.URL)
	defer service.SetChatGPTCheckoutURLForTest(oldURL)

	oauthSvc := service.NewOpenAIOAuthService(nil, &checkoutLinkOAuthClient{})
	oauthSvc.SetCheckoutCloudConfigForTest("", "")
	oauthSvc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		return req.C(), nil
	})
	router := setupAdminCheckoutLinkRouter(newStubAdminService(), oauthSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/chatgpt-plus-checkout", strings.NewReader(`{"access_token":"access-token-1","proxy_source":"direct"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "0 元试用资格")
}

func TestNormalizeExtractedProxyURL_DefaultsToHTTP(t *testing.T) {
	proxyURL, err := normalizeExtractedProxyURL("127.0.0.1:8080\n")
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:8080", proxyURL)
}

func TestAccountHandlerCreateAdminCheckoutLink_ExtractAPIUsesReturnedProxy(t *testing.T) {
	extractServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("socks5://user:pass@127.0.0.1:9000"))
	}))
	defer extractServer.Close()

	checkoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"https://chatgpt.com/payments/checkout/session-extract"}`))
	}))
	defer checkoutServer.Close()

	oldURL := service.SetChatGPTCheckoutURLForTest(checkoutServer.URL)
	defer service.SetChatGPTCheckoutURLForTest(oldURL)

	oauthSvc := service.NewOpenAIOAuthService(nil, &checkoutLinkOAuthClient{})
	oauthSvc.SetCheckoutCloudConfigForTest("", "")
	oauthSvc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		require.Equal(t, "socks5://user:pass@127.0.0.1:9000", proxyURL)
		return req.C(), nil
	})
	router := setupAdminCheckoutLinkRouter(newStubAdminService(), oauthSvc)

	rec := httptest.NewRecorder()
	body := `{"access_token":"access-token-1","proxy_source":"extract_api","extract_api_url":"` + extractServer.URL + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/chatgpt-plus-checkout", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"url":"https://chatgpt.com/payments/checkout/session-extract"`)
}

func TestAccountHandlerCreateAdminCheckoutLink_RejectsNonHTTPExtractAPI(t *testing.T) {
	router := setupAdminCheckoutLinkRouter(newStubAdminService(), service.NewOpenAIOAuthService(nil, &checkoutLinkOAuthClient{}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/chatgpt-plus-checkout", strings.NewReader(`{"access_token":"access-token-1","proxy_source":"extract_api","extract_api_url":"ftp://example.com/get-proxy"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "extract_api_url must use http or https")
}

func TestNormalizeExtractedProxyURL_RejectsInvalidValue(t *testing.T) {
	_, err := normalizeExtractedProxyURL("not-a-proxy")
	require.Error(t, err)
	require.Equal(t, infraerrors.Code(err), http.StatusBadRequest)
}

func TestAccountHandlerCreateAdminCheckoutLink_DoesNotEchoTokenOnUpstreamFailure(t *testing.T) {
	oauthSvc := service.NewOpenAIOAuthService(nil, &checkoutLinkOAuthClient{})
	oauthSvc.SetPrivacyClientFactory(func(proxyURL string) (*req.Client, error) {
		return nil, errors.New("boom access-token-1")
	})
	router := setupAdminCheckoutLinkRouter(newStubAdminService(), oauthSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/chatgpt-plus-checkout", strings.NewReader(`{"access_token":"access-token-1","proxy_source":"direct"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.NotEqual(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "access-token-1")
}

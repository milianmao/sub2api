package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
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

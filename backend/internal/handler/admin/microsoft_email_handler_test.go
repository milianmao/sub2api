package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type microsoftEmailHandlerRepoStub struct {
	accounts []*service.MicrosoftEmailAccount
}

func (r *microsoftEmailHandlerRepoStub) List(ctx context.Context, filter service.MicrosoftEmailListFilter) ([]*service.MicrosoftEmailAccount, int, error) {
	return r.accounts, len(r.accounts), nil
}

func (r *microsoftEmailHandlerRepoStub) GetByID(ctx context.Context, id int64) (*service.MicrosoftEmailAccount, error) {
	for _, account := range r.accounts {
		if account.ID == id {
			return account, nil
		}
	}
	return nil, service.ErrMicrosoftEmailNotFound
}

func (r *microsoftEmailHandlerRepoStub) GetByEmail(ctx context.Context, email string) (*service.MicrosoftEmailAccount, error) {
	return nil, service.ErrMicrosoftEmailNotFound
}

func (r *microsoftEmailHandlerRepoStub) Create(ctx context.Context, account *service.MicrosoftEmailAccount) (*service.MicrosoftEmailAccount, error) {
	return account, nil
}

func (r *microsoftEmailHandlerRepoStub) UpdateCredentials(ctx context.Context, id int64, input service.MicrosoftEmailCredentialUpdate) (*service.MicrosoftEmailAccount, error) {
	return nil, nil
}

func (r *microsoftEmailHandlerRepoStub) UpdateCheckResult(ctx context.Context, id int64, status string, checkedAt time.Time, lastErr *string) error {
	return nil
}

func (r *microsoftEmailHandlerRepoStub) UpdateFetchResult(ctx context.Context, id int64, fetchedAt time.Time, status *string, lastErr *string) error {
	return nil
}

func (r *microsoftEmailHandlerRepoStub) Delete(ctx context.Context, id int64) error {
	return nil
}

func (r *microsoftEmailHandlerRepoStub) BatchDelete(ctx context.Context, ids []int64) (int, error) {
	return len(ids), nil
}

type microsoftEmailHandlerGraphStub struct{}

func (g microsoftEmailHandlerGraphStub) RefreshAccessToken(ctx context.Context, clientID, refreshToken string) (string, error) {
	return "access-token", nil
}

func (g microsoftEmailHandlerGraphStub) ListRecentMessages(ctx context.Context, accessToken string, limit int) ([]service.MicrosoftGraphMessage, error) {
	return []service.MicrosoftGraphMessage{{
		Subject:     "Microsoft verification 123456",
		From:        "account-security-noreply@accountprotection.microsoft.com",
		ReceivedAt:  time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC),
		BodyPreview: "Use 123456 to verify your account.",
		BodyText:    "Full body must not be returned 123456",
	}}, nil
}

func TestMicrosoftEmailHandler_ListMasksSensitiveFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &microsoftEmailHandlerRepoStub{accounts: []*service.MicrosoftEmailAccount{{
		ID:           1,
		Email:        "alice@example.com",
		Password:     "raw-password",
		ClientID:     "client-id-secret",
		RefreshToken: "raw-refresh-token",
		Status:       service.MicrosoftEmailStatusActive,
	}}}
	h := NewMicrosoftEmailHandler(service.NewMicrosoftEmailService(repo, microsoftEmailHandlerGraphStub{}))
	router := gin.New()
	router.GET("/microsoft-emails", h.List)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/microsoft-emails", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "raw-password")
	require.NotContains(t, rec.Body.String(), "raw-refresh-token")
	require.NotContains(t, rec.Body.String(), "client-id-secret")

	var envelope struct {
		Data struct {
			Items []struct {
				Password     string `json:"password"`
				ClientID     string `json:"client_id"`
				RefreshToken string `json:"refresh_token"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data.Items, 1)
	require.NotEmpty(t, envelope.Data.Items[0].Password)
	require.NotEmpty(t, envelope.Data.Items[0].ClientID)
	require.NotEmpty(t, envelope.Data.Items[0].RefreshToken)
}

func TestMicrosoftEmailHandler_FetchCodeReturnsLimitedFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &microsoftEmailHandlerRepoStub{accounts: []*service.MicrosoftEmailAccount{{
		ID:           1,
		Email:        "alice@example.com",
		Password:     "raw-password",
		ClientID:     "client-id-secret",
		RefreshToken: "raw-refresh-token",
		Status:       service.MicrosoftEmailStatusActive,
	}}}
	h := NewMicrosoftEmailHandler(service.NewMicrosoftEmailService(repo, microsoftEmailHandlerGraphStub{}))
	router := gin.New()
	router.POST("/microsoft-emails/:id/fetch-code", h.FetchCode)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/microsoft-emails/1/fetch-code", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "Full body must not be returned")
	require.NotContains(t, rec.Body.String(), "raw-password")
	require.NotContains(t, rec.Body.String(), "raw-refresh-token")

	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.ElementsMatch(t, []string{"email", "code", "source", "subject", "from", "received_at", "snippet", "error"}, mapKeys(envelope.Data))
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

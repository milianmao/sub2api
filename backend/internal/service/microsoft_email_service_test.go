package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeMicrosoftEmailRepo struct {
	byEmail map[string]*MicrosoftEmailAccount
	byID    map[int64]*MicrosoftEmailAccount
	nextID  int64
}

func newFakeMicrosoftEmailRepo() *fakeMicrosoftEmailRepo {
	return &fakeMicrosoftEmailRepo{byEmail: map[string]*MicrosoftEmailAccount{}, byID: map[int64]*MicrosoftEmailAccount{}, nextID: 1}
}

func (r *fakeMicrosoftEmailRepo) List(ctx context.Context, filter MicrosoftEmailListFilter) ([]*MicrosoftEmailAccount, int, error) {
	items := make([]*MicrosoftEmailAccount, 0, len(r.byID))
	for _, item := range r.byID {
		items = append(items, item)
	}
	return items, len(items), nil
}
func (r *fakeMicrosoftEmailRepo) GetByID(ctx context.Context, id int64) (*MicrosoftEmailAccount, error) {
	if item, ok := r.byID[id]; ok {
		return item, nil
	}
	return nil, ErrMicrosoftEmailNotFound
}
func (r *fakeMicrosoftEmailRepo) GetByEmail(ctx context.Context, email string) (*MicrosoftEmailAccount, error) {
	if item, ok := r.byEmail[email]; ok {
		return item, nil
	}
	return nil, ErrMicrosoftEmailNotFound
}
func (r *fakeMicrosoftEmailRepo) Create(ctx context.Context, account *MicrosoftEmailAccount) (*MicrosoftEmailAccount, error) {
	copy := *account
	copy.ID = r.nextID
	r.nextID++
	r.byID[copy.ID] = &copy
	r.byEmail[copy.Email] = &copy
	return &copy, nil
}
func (r *fakeMicrosoftEmailRepo) UpdateCredentials(ctx context.Context, id int64, input MicrosoftEmailCredentialUpdate) (*MicrosoftEmailAccount, error) {
	item, ok := r.byID[id]
	if !ok {
		return nil, ErrMicrosoftEmailNotFound
	}
	item.Password = input.Password
	item.ClientID = input.ClientID
	item.RefreshToken = input.RefreshToken
	return item, nil
}
func (r *fakeMicrosoftEmailRepo) UpdateCheckResult(ctx context.Context, id int64, status string, checkedAt time.Time, lastErr *string) error {
	item, ok := r.byID[id]
	if !ok {
		return ErrMicrosoftEmailNotFound
	}
	item.Status = status
	item.LastCheckAt = &checkedAt
	item.LastError = lastErr
	return nil
}
func (r *fakeMicrosoftEmailRepo) UpdateFetchResult(ctx context.Context, id int64, fetchedAt time.Time, status *string, lastErr *string) error {
	item, ok := r.byID[id]
	if !ok {
		return ErrMicrosoftEmailNotFound
	}
	item.LastFetchAt = &fetchedAt
	if status != nil {
		item.Status = *status
	}
	item.LastError = lastErr
	return nil
}
func (r *fakeMicrosoftEmailRepo) Delete(ctx context.Context, id int64) error {
	delete(r.byID, id)
	return nil
}
func (r *fakeMicrosoftEmailRepo) BatchDelete(ctx context.Context, ids []int64) (int, error) {
	count := 0
	for _, id := range ids {
		if _, ok := r.byID[id]; ok {
			delete(r.byID, id)
			count++
		}
	}
	return count, nil
}

type fakeMicrosoftGraphClient struct {
	tokenErr   error
	messageErr error
	messages   []MicrosoftGraphMessage
}

func (c fakeMicrosoftGraphClient) RefreshAccessToken(ctx context.Context, clientID, refreshToken string) (string, error) {
	if c.tokenErr != nil {
		return "", c.tokenErr
	}
	return "access-token", nil
}
func (c fakeMicrosoftGraphClient) ListRecentMessages(ctx context.Context, accessToken string, limit int) ([]MicrosoftGraphMessage, error) {
	if c.messageErr != nil {
		return nil, c.messageErr
	}
	return c.messages, nil
}

func TestMicrosoftEmailService_ImportTXT_CreatesAndUpdatesAccounts(t *testing.T) {
	repo := newFakeMicrosoftEmailRepo()
	svc := NewMicrosoftEmailService(repo, fakeMicrosoftGraphClient{})

	res, err := svc.ImportTXT(context.Background(), "user@example.com----pass----client----refresh\n")
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	require.Equal(t, 0, res.Failed)

	res, err = svc.ImportTXT(context.Background(), "user@example.com----pass2----client2----refresh2\n")
	require.NoError(t, err)
	require.Equal(t, 0, res.Created)
	require.Equal(t, 1, res.Updated)
	stored, err := repo.GetByEmail(context.Background(), "user@example.com")
	require.NoError(t, err)
	require.Equal(t, "client2", stored.ClientID)
}

func TestMicrosoftEmailService_ImportTXT_ReportsLineErrors(t *testing.T) {
	svc := NewMicrosoftEmailService(newFakeMicrosoftEmailRepo(), fakeMicrosoftGraphClient{})
	res, err := svc.ImportTXT(context.Background(), "bad-line\ninvalid-email----p----c----r\n")
	require.NoError(t, err)
	require.Equal(t, 2, res.Total)
	require.Equal(t, 2, res.Failed)
	require.Len(t, res.Errors, 2)
}

func TestMaskMicrosoftEmailAccount_HidesSecrets(t *testing.T) {
	masked := MaskMicrosoftEmailAccount(&MicrosoftEmailAccount{
		Email: "user@example.com", Password: "secret-password", ClientID: "1234567890", RefreshToken: "refresh-secret",
	})
	require.NotContains(t, masked.Password, "secret-password")
	require.NotContains(t, masked.RefreshToken, "refresh-secret")
	require.Equal(t, "1234****7890", masked.ClientID)
}

func TestExtractMicrosoftVerificationCode_PrefersKeywordMessage(t *testing.T) {
	messages := []MicrosoftGraphMessage{
		{Subject: "Newsletter 999999", BodyPreview: "not a login code", ReceivedAt: time.Now().Add(-time.Minute)},
		{Subject: "Your verification code", BodyPreview: "Use code 123456 to continue", ReceivedAt: time.Now()},
	}
	result := ExtractMicrosoftVerificationCode(messages)
	require.Equal(t, "123456", result.Code)
	require.Equal(t, "body", result.Source)
}

func TestMicrosoftEmailService_Check_UpdatesInvalidOnRefreshFailure(t *testing.T) {
	repo := newFakeMicrosoftEmailRepo()
	created, err := repo.Create(context.Background(), &MicrosoftEmailAccount{Email: "u@example.com", ClientID: "client", RefreshToken: "bad"})
	require.NoError(t, err)
	svc := NewMicrosoftEmailService(repo, fakeMicrosoftGraphClient{tokenErr: errors.New("invalid_grant")})

	res, err := svc.Check(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, MicrosoftEmailStatusInvalid, res.Status)
	stored, err := repo.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, MicrosoftEmailStatusInvalid, stored.Status)
}

func TestMicrosoftEmailService_Check_RedactsAccountSecretsFromRefreshFailure(t *testing.T) {
	repo := newFakeMicrosoftEmailRepo()
	password := "actual-password-secret"
	refreshToken := "actual-refresh-token-secret"
	clientID := "actual-client-id-secret"
	created, err := repo.Create(context.Background(), &MicrosoftEmailAccount{
		Email:        "u@example.com",
		Password:     password,
		ClientID:     clientID,
		RefreshToken: refreshToken,
	})
	require.NoError(t, err)
	svc := NewMicrosoftEmailService(repo, fakeMicrosoftGraphClient{tokenErr: errors.New("invalid refresh token " + refreshToken + " for password " + password + " and client " + clientID)})

	res, err := svc.Check(context.Background(), created.ID)
	require.NoError(t, err)
	require.NotNil(t, res.LastError)
	require.NotContains(t, *res.LastError, password)
	require.NotContains(t, *res.LastError, refreshToken)
	require.NotContains(t, *res.LastError, clientID)
	require.Contains(t, *res.LastError, "[redacted]")
	require.Contains(t, *res.LastError, "invalid refresh token")

	stored, err := repo.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.LastError)
	require.NotContains(t, *stored.LastError, password)
	require.NotContains(t, *stored.LastError, refreshToken)
	require.NotContains(t, *stored.LastError, clientID)
	require.Contains(t, *stored.LastError, "[redacted]")
	require.Contains(t, *stored.LastError, "invalid refresh token")
}

func TestMicrosoftEmailService_FetchCode_ReturnsCodeMetadataAndDoesNotStoreBody(t *testing.T) {
	repo := newFakeMicrosoftEmailRepo()
	created, err := repo.Create(context.Background(), &MicrosoftEmailAccount{Email: "u@example.com", ClientID: "client", RefreshToken: "refresh", Status: MicrosoftEmailStatusError, LastError: stringPtr("old error")})
	require.NoError(t, err)
	receivedAt := time.Date(2026, 5, 22, 10, 30, 0, 0, time.UTC)
	svc := NewMicrosoftEmailService(repo, fakeMicrosoftGraphClient{messages: []MicrosoftGraphMessage{{
		Subject:     "Microsoft verification code 654321",
		From:        "account-security-noreply@accountprotection.microsoft.com",
		ReceivedAt:  receivedAt,
		BodyPreview: "Use 654321 to sign in",
		BodyText:    "Full body contains 654321 and must not be persisted",
	}}})

	res, err := svc.FetchCode(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "u@example.com", res.Email)
	require.Equal(t, "654321", res.Code)
	require.Equal(t, "subject", res.Source)
	require.Equal(t, "Microsoft verification code 654321", res.Subject)
	require.Equal(t, "account-security-noreply@accountprotection.microsoft.com", res.From)
	require.Equal(t, receivedAt, res.ReceivedAt)
	require.Equal(t, "Use 654321 to sign in", res.Snippet)
	require.Empty(t, res.Error)

	payload, err := json.Marshal(res)
	require.NoError(t, err)
	var responseFields map[string]any
	require.NoError(t, json.Unmarshal(payload, &responseFields))
	require.ElementsMatch(t, []string{"email", "code", "source", "subject", "from", "received_at", "snippet", "error"}, mapKeys(responseFields))
	require.NotContains(t, responseFields, "Message")
	require.NotContains(t, responseFields, "message")
	require.NotContains(t, responseFields, "BodyText")
	require.NotContains(t, responseFields, "body_text")
	require.NotContains(t, string(payload), "Full body")

	stored, err := repo.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, MicrosoftEmailStatusActive, stored.Status)
	require.NotNil(t, stored.LastFetchAt)
	require.Nil(t, stored.LastError)
	require.NotContains(t, stored.Password, "Full body")
	require.NotContains(t, stored.RefreshToken, "Full body")
}

func TestMicrosoftEmailService_FetchCode_CodeNotFoundDoesNotInvalidateActiveAccount(t *testing.T) {
	repo := newFakeMicrosoftEmailRepo()
	created, err := repo.Create(context.Background(), &MicrosoftEmailAccount{Email: "u@example.com", ClientID: "client", RefreshToken: "refresh", Status: MicrosoftEmailStatusActive})
	require.NoError(t, err)
	svc := NewMicrosoftEmailService(repo, fakeMicrosoftGraphClient{messages: []MicrosoftGraphMessage{{Subject: "Welcome", BodyPreview: "No verification code here", ReceivedAt: time.Now()}}})

	res, err := svc.FetchCode(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "u@example.com", res.Email)
	require.Empty(t, res.Code)
	require.Equal(t, "code_not_found", res.Error)
	payload, err := json.Marshal(res)
	require.NoError(t, err)
	var responseFields map[string]any
	require.NoError(t, json.Unmarshal(payload, &responseFields))
	require.NotContains(t, responseFields, "FetchedAt")
	require.NotContains(t, responseFields, "LastError")
	require.NotContains(t, responseFields, "fetched_at")
	require.NotContains(t, responseFields, "last_error")

	stored, err := repo.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, MicrosoftEmailStatusActive, stored.Status)
	require.NotNil(t, stored.LastFetchAt)
	require.NotNil(t, stored.LastError)
	require.Equal(t, "code_not_found", *stored.LastError)
}

func TestMicrosoftEmailService_FetchCode_RedactsRefreshFailureSecrets(t *testing.T) {
	repo := newFakeMicrosoftEmailRepo()
	password := "actual-password-secret"
	refreshToken := "actual-refresh-token-secret"
	clientID := "actual-client-id-secret"
	created, err := repo.Create(context.Background(), &MicrosoftEmailAccount{Email: "u@example.com", Password: password, ClientID: clientID, RefreshToken: refreshToken, Status: MicrosoftEmailStatusActive})
	require.NoError(t, err)
	svc := NewMicrosoftEmailService(repo, fakeMicrosoftGraphClient{tokenErr: errors.New("invalid refresh token " + refreshToken + " for password " + password + " and client " + clientID)})

	res, err := svc.FetchCode(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "u@example.com", res.Email)
	require.Empty(t, res.Code)
	require.NotEmpty(t, res.Error)
	require.NotContains(t, res.Error, password)
	require.NotContains(t, res.Error, refreshToken)
	require.NotContains(t, res.Error, clientID)
	require.Contains(t, res.Error, "[redacted]")

	stored, err := repo.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, MicrosoftEmailStatusInvalid, stored.Status)
	require.NotNil(t, stored.LastFetchAt)
	require.NotNil(t, stored.LastError)
	require.Equal(t, res.Error, *stored.LastError)
}

func TestMicrosoftEmailService_FetchCode_RedactsMessageFetchFailureSecrets(t *testing.T) {
	repo := newFakeMicrosoftEmailRepo()
	password := "actual-password-secret"
	refreshToken := "actual-refresh-token-secret"
	clientID := "actual-client-id-secret"
	created, err := repo.Create(context.Background(), &MicrosoftEmailAccount{Email: "u@example.com", Password: password, ClientID: clientID, RefreshToken: refreshToken, Status: MicrosoftEmailStatusActive})
	require.NoError(t, err)
	svc := NewMicrosoftEmailService(repo, fakeMicrosoftGraphClient{messageErr: errors.New("graph failed for access_token using " + refreshToken + " " + clientID + " " + password)})

	res, err := svc.FetchCode(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "u@example.com", res.Email)
	require.Empty(t, res.Code)
	require.NotEmpty(t, res.Error)
	require.NotContains(t, res.Error, password)
	require.NotContains(t, res.Error, refreshToken)
	require.NotContains(t, res.Error, clientID)
	require.NotContains(t, res.Error, "access_token")
	require.Contains(t, res.Error, "[redacted]")

	stored, err := repo.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, MicrosoftEmailStatusError, stored.Status)
	require.NotNil(t, stored.LastFetchAt)
	require.NotNil(t, stored.LastError)
	require.Equal(t, res.Error, *stored.LastError)
}

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func stringPtr(value string) *string {
	return &value
}

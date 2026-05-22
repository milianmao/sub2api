package service

import (
	"context"
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

type fakeMicrosoftGraphClient struct {
	tokenErr error
	messages []MicrosoftGraphMessage
}

func (c fakeMicrosoftGraphClient) RefreshAccessToken(ctx context.Context, clientID, refreshToken string) (string, error) {
	if c.tokenErr != nil {
		return "", c.tokenErr
	}
	return "access-token", nil
}
func (c fakeMicrosoftGraphClient) ListRecentMessages(ctx context.Context, accessToken string, limit int) ([]MicrosoftGraphMessage, error) {
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

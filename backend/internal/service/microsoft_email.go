package service

import (
	"context"
	"errors"
	"time"
)

const (
	MicrosoftEmailStatusActive  = "active"
	MicrosoftEmailStatusInvalid = "invalid"
	MicrosoftEmailStatusError   = "error"
)

var ErrMicrosoftEmailNotFound = errors.New("microsoft email account not found")

type MicrosoftEmailAccount struct {
	ID           int64
	Email        string
	Password     string
	ClientID     string
	RefreshToken string
	Status       string
	LastCheckAt  *time.Time
	LastFetchAt  *time.Time
	LastError    *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type MicrosoftEmailListFilter struct {
	Email  string
	Status string
	Limit  int
	Offset int
}

type MicrosoftEmailCredentialUpdate struct {
	Password     string
	ClientID     string
	RefreshToken string
}

type MicrosoftEmailRepository interface {
	List(ctx context.Context, filter MicrosoftEmailListFilter) ([]*MicrosoftEmailAccount, int, error)
	GetByID(ctx context.Context, id int64) (*MicrosoftEmailAccount, error)
	GetByEmail(ctx context.Context, email string) (*MicrosoftEmailAccount, error)
	Create(ctx context.Context, account *MicrosoftEmailAccount) (*MicrosoftEmailAccount, error)
	UpdateCredentials(ctx context.Context, id int64, input MicrosoftEmailCredentialUpdate) (*MicrosoftEmailAccount, error)
	UpdateCheckResult(ctx context.Context, id int64, status string, checkedAt time.Time, lastErr *string) error
	UpdateFetchResult(ctx context.Context, id int64, fetchedAt time.Time, status *string, lastErr *string) error
	Delete(ctx context.Context, id int64) error
}

type MicrosoftGraphClient interface {
	RefreshAccessToken(ctx context.Context, clientID, refreshToken string) (string, error)
	ListRecentMessages(ctx context.Context, accessToken string, limit int) ([]MicrosoftGraphMessage, error)
}

type MicrosoftGraphMessage struct {
	Subject     string
	From        string
	ReceivedAt  time.Time
	BodyPreview string
	BodyText    string
}

type MicrosoftEmailImportResult struct {
	Total   int
	Created int
	Updated int
	Failed  int
	Items   []MicrosoftEmailImportItem
	Errors  []MicrosoftEmailImportError
}

type MicrosoftEmailImportItem struct {
	Line    int
	Email   string
	Action  string
	Account *MicrosoftEmailAccount
}

type MicrosoftEmailImportError struct {
	Line  int
	Email string
	Error string
}

type MicrosoftEmailCheckResult struct {
	ID        int64
	Email     string
	Status    string
	CheckedAt time.Time
	LastError *string
}

type MicrosoftEmailFetchCodeResult struct {
	Email      string
	Code       string
	Source     string
	Subject    string
	From       string
	ReceivedAt time.Time
	Snippet    string
	Error      string
	FetchedAt  time.Time
	LastError  *string
}

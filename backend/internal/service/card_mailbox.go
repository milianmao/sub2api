package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	CardMailboxFetchStatusSuccess = "success"
	CardMailboxFetchStatusFailed  = "failed"
)

var (
	ErrCardMailboxNotFound          = infraerrors.NotFound("CARD_MAILBOX_NOT_FOUND", "card mailbox not found")
	ErrCardMailboxInvalidInput      = infraerrors.BadRequest("CARD_MAILBOX_INPUT_INVALID", "card mailbox input is invalid")
	ErrCardMailboxEmailRequired     = infraerrors.BadRequest("CARD_MAILBOX_EMAIL_REQUIRED", "email is required")
	ErrCardMailboxURLRequired       = infraerrors.BadRequest("CARD_MAILBOX_URL_REQUIRED", "mailbox url is required")
	ErrCardMailboxFetchFailed       = infraerrors.ServiceUnavailable("CARD_MAILBOX_FETCH_FAILED", "failed to fetch mailbox")
	ErrCardMailboxCodeNotFound      = infraerrors.NotFound("CARD_MAILBOX_CODE_NOT_FOUND", "verification code not found")
	ErrCardMailboxRepositoryFailure = infraerrors.InternalServer("CARD_MAILBOX_REPOSITORY_FAILURE", "card mailbox repository operation failed")
	ErrCardMailboxDependencyMissing = infraerrors.InternalServer("CARD_MAILBOX_DEPENDENCY_MISSING", "card mailbox service dependency is missing")
)

type CardMailbox struct {
	ID            int64
	Email         string
	MailboxURL    string
	RawJSON       string
	LastCode      string
	LastStatus    string
	LastError     string
	LastFetchedAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type CardMailboxUpsertInput struct {
	Email      string
	MailboxURL string
	RawJSON    string
}

type CardMailboxLatestResultInput struct {
	LastCode      string
	LastStatus    string
	LastError     string
	LastFetchedAt *time.Time
}

type CardMailboxListFilter struct {
	Email  string
	Status string
	Limit  int
	Offset int
}

type CardMailboxExportItem struct {
	ID            int64     `json:"id"`
	Email         string    `json:"email"`
	MailboxURL    string    `json:"mailbox_url"`
	RawJSON       string    `json:"raw_json"`
	LastCode      string    `json:"last_code,omitempty"`
	LastStatus    string    `json:"last_status,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
	LastFetchedAt *time.Time `json:"last_fetched_at,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type CardMailboxImportResult struct {
	Imported int
	Failed   int
	Errors   []CardMailboxImportError
}

type CardMailboxImportError struct {
	Line    int
	Message string
}

type CardMailboxFetchResult struct {
	Email      string
	Code       string
	Status     string
	FetchedAt  time.Time
	Source     string
	Subject    string
	From       string
	ReceivedAt time.Time
	Snippet    string
}

type CardMailboxRepository interface {
	List(ctx context.Context, filter CardMailboxListFilter) ([]*CardMailbox, int, error)
	UpsertByEmail(ctx context.Context, input CardMailboxUpsertInput) (*CardMailbox, error)
	GetByID(ctx context.Context, id int64) (*CardMailbox, error)
	GetByIDs(ctx context.Context, ids []int64) ([]*CardMailbox, error)
	UpdateLatestResult(ctx context.Context, id int64, input CardMailboxLatestResultInput) error
	Delete(ctx context.Context, id int64) error
}

package repository

import (
	"context"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/cardmailboxcredential"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const defaultCardMailboxListLimit = 20

type cardMailboxRepository struct {
	client *dbent.Client
}

func NewCardMailboxRepository(client *dbent.Client) service.CardMailboxRepository {
	return &cardMailboxRepository{client: client}
}

func (r *cardMailboxRepository) List(ctx context.Context, filter service.CardMailboxListFilter) ([]*service.CardMailbox, int, error) {
	q := clientFromContext(ctx, r.client).CardMailboxCredential.Query()
	q = applyCardMailboxListFilter(q, filter)

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultCardMailboxListLimit
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := q.
		Order(dbent.Desc(cardmailboxcredential.FieldID)).
		Offset(offset).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}

	out := make([]*service.CardMailbox, 0, len(rows))
	for _, row := range rows {
		out = append(out, cardMailboxEntityToService(row))
	}
	return out, total, nil
}

func (r *cardMailboxRepository) UpsertByEmail(ctx context.Context, input service.CardMailboxUpsertInput) (*service.CardMailbox, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	mailboxURL := strings.TrimSpace(input.MailboxURL)
	client := clientFromContext(ctx, r.client)
	id, err := client.CardMailboxCredential.Create().
		SetEmail(email).
		SetMailboxURL(mailboxURL).
		SetRawJSON(strings.TrimSpace(input.RawJSON)).
		OnConflictColumns(cardmailboxcredential.FieldEmail).
		UpdateNewValues().
		ID(ctx)
	if err != nil {
		return nil, translateCardMailboxPersistenceError(err)
	}
	row, err := client.CardMailboxCredential.Get(ctx, id)
	if err != nil {
		return nil, translateCardMailboxPersistenceError(err)
	}
	return cardMailboxEntityToService(row), nil
}

func (r *cardMailboxRepository) GetByID(ctx context.Context, id int64) (*service.CardMailbox, error) {
	row, err := clientFromContext(ctx, r.client).CardMailboxCredential.Get(ctx, id)
	if err != nil {
		return nil, translateCardMailboxPersistenceError(err)
	}
	return cardMailboxEntityToService(row), nil
}

func (r *cardMailboxRepository) GetByIDs(ctx context.Context, ids []int64) ([]*service.CardMailbox, error) {
	if len(ids) == 0 {
		return []*service.CardMailbox{}, nil
	}
	rows, err := clientFromContext(ctx, r.client).CardMailboxCredential.Query().
		Where(cardmailboxcredential.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, translateCardMailboxPersistenceError(err)
	}
	items := make([]*service.CardMailbox, 0, len(rows))
	for _, row := range rows {
		items = append(items, cardMailboxEntityToService(row))
	}
	return items, nil
}

func (r *cardMailboxRepository) UpdateLatestResult(ctx context.Context, id int64, input service.CardMailboxLatestResultInput) error {
	updater := clientFromContext(ctx, r.client).CardMailboxCredential.UpdateOneID(id).
		SetNillableLastFetchedAt(input.LastFetchedAt)
	if strings.TrimSpace(input.LastCode) != "" {
		updater = updater.SetLastCode(input.LastCode)
	}
	if strings.TrimSpace(input.LastStatus) != "" {
		updater = updater.SetLastStatus(input.LastStatus)
	}
	if strings.TrimSpace(input.LastError) != "" {
		updater = updater.SetLastError(input.LastError)
	} else if input.LastStatus == service.CardMailboxFetchStatusSuccess {
		updater = updater.ClearLastError()
	}
	if err := updater.Exec(ctx); err != nil {
		return translateCardMailboxPersistenceError(err)
	}
	return nil
}

func (r *cardMailboxRepository) Delete(ctx context.Context, id int64) error {
	if err := clientFromContext(ctx, r.client).CardMailboxCredential.DeleteOneID(id).Exec(ctx); err != nil {
		return translateCardMailboxPersistenceError(err)
	}
	return nil
}

func applyCardMailboxListFilter(q *dbent.CardMailboxCredentialQuery, filter service.CardMailboxListFilter) *dbent.CardMailboxCredentialQuery {
	if q == nil {
		return q
	}
	if email := strings.TrimSpace(filter.Email); email != "" {
		q = q.Where(cardmailboxcredential.EmailContainsFold(email))
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		q = q.Where(cardmailboxcredential.LastStatusEQ(status))
	}
	return q
}

func cardMailboxEntityToService(row *dbent.CardMailboxCredential) *service.CardMailbox {
	if row == nil {
		return nil
	}
	return &service.CardMailbox{
		ID:            row.ID,
		Email:         row.Email,
		MailboxURL:    row.MailboxURL,
		RawJSON:       row.RawJSON,
		LastCode:      stringFromPtr(row.LastCode),
		LastStatus:    stringFromPtr(row.LastStatus),
		LastError:     stringFromPtr(row.LastError),
		LastFetchedAt: row.LastFetchedAt,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func stringFromPtr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func translateCardMailboxPersistenceError(err error) error {
	translated := translatePersistenceError(err, nil, nil)
	if dbent.IsNotFound(translated) {
		return service.ErrCardMailboxNotFound
	}
	return translated
}

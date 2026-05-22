package repository

import (
	"context"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/microsoftemailaccount"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const defaultMicrosoftEmailListLimit = 20

type microsoftEmailRepository struct {
	client *dbent.Client
}

func NewMicrosoftEmailRepository(client *dbent.Client) service.MicrosoftEmailRepository {
	return &microsoftEmailRepository{client: client}
}

func (r *microsoftEmailRepository) List(ctx context.Context, filter service.MicrosoftEmailListFilter) ([]*service.MicrosoftEmailAccount, int, error) {
	q := clientFromContext(ctx, r.client).MicrosoftEmailAccount.Query()
	q = applyMicrosoftEmailListFilter(q, filter)

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = defaultMicrosoftEmailListLimit
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := q.
		Order(dbent.Desc(microsoftemailaccount.FieldID)).
		Offset(offset).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}

	out := make([]*service.MicrosoftEmailAccount, 0, len(rows))
	for _, row := range rows {
		out = append(out, microsoftEmailEntityToService(row))
	}
	return out, total, nil
}

func (r *microsoftEmailRepository) GetByID(ctx context.Context, id int64) (*service.MicrosoftEmailAccount, error) {
	row, err := clientFromContext(ctx, r.client).MicrosoftEmailAccount.Query().
		Where(microsoftemailaccount.IDEQ(id)).
		Only(ctx)
	if err != nil {
		return nil, translateMicrosoftEmailPersistenceError(err)
	}
	return microsoftEmailEntityToService(row), nil
}

func (r *microsoftEmailRepository) GetByEmail(ctx context.Context, email string) (*service.MicrosoftEmailAccount, error) {
	row, err := clientFromContext(ctx, r.client).MicrosoftEmailAccount.Query().
		Where(microsoftemailaccount.EmailEQ(strings.TrimSpace(email))).
		Only(ctx)
	if err != nil {
		return nil, translateMicrosoftEmailPersistenceError(err)
	}
	return microsoftEmailEntityToService(row), nil
}

func (r *microsoftEmailRepository) Create(ctx context.Context, account *service.MicrosoftEmailAccount) (*service.MicrosoftEmailAccount, error) {
	if account == nil {
		account = &service.MicrosoftEmailAccount{}
	}
	status := strings.TrimSpace(account.Status)
	if status == "" {
		status = service.MicrosoftEmailStatusActive
	}

	row, err := clientFromContext(ctx, r.client).MicrosoftEmailAccount.Create().
		SetEmail(strings.TrimSpace(account.Email)).
		SetPassword(account.Password).
		SetClientID(account.ClientID).
		SetRefreshToken(account.RefreshToken).
		SetStatus(status).
		SetNillableLastCheckAt(account.LastCheckAt).
		SetNillableLastFetchAt(account.LastFetchAt).
		SetNillableLastError(account.LastError).
		Save(ctx)
	if err != nil {
		return nil, translateMicrosoftEmailPersistenceError(err)
	}
	return microsoftEmailEntityToService(row), nil
}

func (r *microsoftEmailRepository) UpdateCredentials(ctx context.Context, id int64, input service.MicrosoftEmailCredentialUpdate) (*service.MicrosoftEmailAccount, error) {
	row, err := clientFromContext(ctx, r.client).MicrosoftEmailAccount.UpdateOneID(id).
		SetPassword(input.Password).
		SetClientID(input.ClientID).
		SetRefreshToken(input.RefreshToken).
		SetStatus(service.MicrosoftEmailStatusActive).
		ClearLastError().
		Save(ctx)
	if err != nil {
		return nil, translateMicrosoftEmailPersistenceError(err)
	}
	return microsoftEmailEntityToService(row), nil
}

func (r *microsoftEmailRepository) UpdateCheckResult(ctx context.Context, id int64, status string, checkedAt time.Time, lastErr *string) error {
	updater := clientFromContext(ctx, r.client).MicrosoftEmailAccount.UpdateOneID(id).
		SetStatus(status).
		SetLastCheckAt(checkedAt)
	if lastErr != nil {
		updater = updater.SetLastError(*lastErr)
	} else {
		updater = updater.ClearLastError()
	}
	if err := updater.Exec(ctx); err != nil {
		return translateMicrosoftEmailPersistenceError(err)
	}
	return nil
}

func (r *microsoftEmailRepository) UpdateFetchResult(ctx context.Context, id int64, fetchedAt time.Time, status *string, lastErr *string) error {
	updater := clientFromContext(ctx, r.client).MicrosoftEmailAccount.UpdateOneID(id).
		SetLastFetchAt(fetchedAt)
	if status != nil {
		updater = updater.SetStatus(*status)
	}
	if lastErr != nil {
		updater = updater.SetLastError(*lastErr)
	} else {
		updater = updater.ClearLastError()
	}
	if err := updater.Exec(ctx); err != nil {
		return translateMicrosoftEmailPersistenceError(err)
	}
	return nil
}

func (r *microsoftEmailRepository) Delete(ctx context.Context, id int64) error {
	if err := clientFromContext(ctx, r.client).MicrosoftEmailAccount.DeleteOneID(id).Exec(ctx); err != nil {
		return translateMicrosoftEmailPersistenceError(err)
	}
	return nil
}

func (r *microsoftEmailRepository) BatchDelete(ctx context.Context, ids []int64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	return clientFromContext(ctx, r.client).MicrosoftEmailAccount.Delete().
		Where(microsoftemailaccount.IDIn(ids...)).
		Exec(ctx)
}

func applyMicrosoftEmailListFilter(q *dbent.MicrosoftEmailAccountQuery, filter service.MicrosoftEmailListFilter) *dbent.MicrosoftEmailAccountQuery {
	if q == nil {
		return q
	}
	if email := strings.TrimSpace(filter.Email); email != "" {
		q = q.Where(microsoftemailaccount.EmailContainsFold(email))
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		q = q.Where(microsoftemailaccount.StatusEQ(status))
	}
	return q
}

func microsoftEmailEntityToService(row *dbent.MicrosoftEmailAccount) *service.MicrosoftEmailAccount {
	if row == nil {
		return nil
	}
	return &service.MicrosoftEmailAccount{
		ID:           row.ID,
		Email:        row.Email,
		Password:     row.Password,
		ClientID:     row.ClientID,
		RefreshToken: row.RefreshToken,
		Status:       row.Status,
		LastCheckAt:  row.LastCheckAt,
		LastFetchAt:  row.LastFetchAt,
		LastError:    row.LastError,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func translateMicrosoftEmailPersistenceError(err error) error {
	translated := translatePersistenceError(err, nil, nil)
	if dbent.IsNotFound(translated) {
		return service.ErrMicrosoftEmailNotFound
	}
	return translated
}

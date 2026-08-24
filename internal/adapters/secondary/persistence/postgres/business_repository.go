package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

type RepositoryErrorKind string

const (
	RepositoryNotFound RepositoryErrorKind = "not_found"
	RepositoryConflict RepositoryErrorKind = "conflict"
	RepositoryInvalid  RepositoryErrorKind = "invalid"
)

type RepositoryError struct {
	Operation string
	Kind      RepositoryErrorKind
	Err       error
}

func (e *RepositoryError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("repository %s: %s", e.Operation, e.Kind)
	}
	return fmt.Sprintf("repository %s: %s: %v", e.Operation, e.Kind, e.Err)
}
func (e *RepositoryError) Unwrap() error { return e.Err }
func IsRepositoryKind(err error, kind RepositoryErrorKind) bool {
	var target *RepositoryError
	return errors.As(err, &target) && target.Kind == kind
}

type BusinessRepository struct{ adapter *Adapter }

func NewBusinessRepository(adapter *Adapter) *BusinessRepository {
	return &BusinessRepository{adapter: adapter}
}

func (r *BusinessRepository) GetByID(ctx context.Context, businessID string) (ports.BusinessRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.BusinessRecord{}, ErrPoolClosed
	}
	if businessID == "" {
		return ports.BusinessRecord{}, &RepositoryError{Operation: "business.get_by_id", Kind: RepositoryInvalid, Err: errors.New("business id is required")}
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.BusinessRecord{}, err
	}
	const query = `SELECT id::text, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at FROM businesses WHERE id = $1::uuid`
	var record ports.BusinessRecord
	if err := executor.QueryRow(ctx, query, businessID).Scan(&record.ID, &record.Name, &record.Slug, &record.Status, &record.VerticalType, &record.Timezone, &record.DefaultCurrency, &record.Locale, &record.CreatedAt, &record.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.BusinessRecord{}, &RepositoryError{Operation: "business.get_by_id", Kind: RepositoryNotFound, Err: err}
		}
		return ports.BusinessRecord{}, &RepositoryError{Operation: "business.get_by_id", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

var _ ports.BusinessRepository = (*BusinessRepository)(nil)

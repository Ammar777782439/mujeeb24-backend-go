package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

// PlatformBusinessRepository implements ports.PlatformBusinessLifecyclePort
// against Postgres. It manipulates the existing businesses table — Platform
// Admin updates the platform_status column without ever touching merchant-
// owned data (customer records, conversations, catalog, etc.).
//
// Per Platform Administration Contract §51: the Platform view of a business
// shows only identity + platform_status + subscription summary. NO merchant
// content columns are SELECTed here.
//
// Tenant isolation: Platform Admin operates on businesses.id directly —
// businesses are platform-owned records (a business IS a tenant). The
// Platform Admin is not a member of the business; they hold Platform Scope,
// which is the only authority that can transition a business's lifecycle.
type PlatformBusinessRepository struct {
	adapter *Adapter
}

func NewPlatformBusinessRepository(adapter *Adapter) *PlatformBusinessRepository {
	return &PlatformBusinessRepository{adapter: adapter}
}

const platformBusinessSelectColumns = `b.id::text, b.name, b.slug, b.status,
       (SELECT p.display_name || ' <' || p.email || '>'
          FROM business_memberships bm
          JOIN principals p ON p.id = bm.principal_id
         WHERE bm.business_id = b.id
           AND bm.role = 'owner'
           AND bm.status = 'active'
         ORDER BY bm.created_at ASC
         LIMIT 1) AS owner_identity_summary,
       b.created_at, b.updated_at`

const platformBusinessReturnColumns = `id::text, name, slug, status, NULL::text AS owner_identity_summary, created_at, updated_at`

// Create inserts a new business row with status='pending_setup'.
// Per Contract §9: the Platform Admin creates the business; the owner
// invitation is a SEPARATE step using the existing team invitation mechanism.
// The admin does NOT enter the owner's password.
func (r *PlatformBusinessRepository) Create(ctx context.Context, create ports.PlatformBusinessCreate) (ports.PlatformBusinessRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.PlatformBusinessRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(create.ID) == "" {
		return ports.PlatformBusinessRecord{}, invalidRepositoryInput("platform_business.create", "id is required")
	}
	if strings.TrimSpace(create.Name) == "" || strings.TrimSpace(create.Slug) == "" {
		return ports.PlatformBusinessRecord{}, invalidRepositoryInput("platform_business.create", "name and slug are required")
	}
	if strings.TrimSpace(create.VerticalType) == "" {
		return ports.PlatformBusinessRecord{}, invalidRepositoryInput("platform_business.create", "vertical_type is required")
	}
	if len(create.DefaultCurrency) != 3 {
		return ports.PlatformBusinessRecord{}, invalidRepositoryInput("platform_business.create", "default_currency must be 3 uppercase letters")
	}
	if strings.TrimSpace(create.Locale) == "" {
		return ports.PlatformBusinessRecord{}, invalidRepositoryInput("platform_business.create", "locale is required")
	}
	if strings.TrimSpace(create.Timezone) == "" {
		create.Timezone = "Asia/Aden"
	}
	if create.Now.IsZero() {
		create.Now = time.Now().UTC()
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PlatformBusinessRecord{}, err
	}
	var record ports.PlatformBusinessRecord
	err = executor.QueryRow(ctx,
		`INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at)
                 VALUES ($1::uuid, $2, $3, 'pending_setup', $4, $5, $6, $7, $8, $8)
                 RETURNING `+platformBusinessReturnColumns,
		create.ID, create.Name, create.Slug, create.VerticalType, create.Timezone, create.DefaultCurrency, create.Locale, create.Now,
	).Scan(&record.ID, &record.Name, &record.Slug, &record.PlatformStatus, &record.OwnerIdentitySummary, &record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		return ports.PlatformBusinessRecord{}, classifyRepositoryWriteError("platform_business.create", err)
	}
	return record, nil
}

func (r *PlatformBusinessRepository) List(ctx context.Context, filter ports.PlatformBusinessListFilter) (ports.PlatformBusinessPage, error) {
	if r == nil || r.adapter == nil {
		return ports.PlatformBusinessPage{}, ErrPoolClosed
	}
	if filter.Limit <= 0 || filter.Limit > 1000 {
		filter.Limit = 100
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PlatformBusinessPage{}, err
	}
	statusFilter := strings.ToLower(strings.TrimSpace(filter.Status))
	rows, err := executor.Query(ctx,
		`SELECT `+platformBusinessSelectColumns+`
                 FROM businesses b
                 WHERE ($1 = '' OR b.name ILIKE '%' || $1 || '%' OR b.slug ILIKE '%' || $1 || '%')
                   AND ($2 = '' OR b.status = $2)
                   AND ($3::timestamptz IS NULL OR b.created_at >= $3)
                   AND ($4::timestamptz IS NULL OR b.created_at <= $4)
                 ORDER BY b.created_at DESC, b.id DESC
                 LIMIT $5`,
		strings.TrimSpace(filter.Search), statusFilter, filter.CreatedFrom, filter.CreatedTo, filter.Limit,
	)
	if err != nil {
		return ports.PlatformBusinessPage{}, &RepositoryError{Operation: "platform_business.list", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.PlatformBusinessRecord, 0, filter.Limit)
	for rows.Next() {
		var record ports.PlatformBusinessRecord
		if err := rows.Scan(&record.ID, &record.Name, &record.Slug, &record.PlatformStatus, &record.OwnerIdentitySummary, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return ports.PlatformBusinessPage{}, &RepositoryError{Operation: "platform_business.list", Kind: RepositoryInvalid, Err: err}
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return ports.PlatformBusinessPage{}, &RepositoryError{Operation: "platform_business.list", Kind: RepositoryInvalid, Err: err}
	}
	return ports.PlatformBusinessPage{Items: items}, nil
}

func (r *PlatformBusinessRepository) GetByID(ctx context.Context, businessID string) (ports.PlatformBusinessRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.PlatformBusinessRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" {
		return ports.PlatformBusinessRecord{}, invalidRepositoryInput("platform_business.get", "business id is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PlatformBusinessRecord{}, err
	}
	var record ports.PlatformBusinessRecord
	err = executor.QueryRow(ctx,
		`SELECT `+platformBusinessSelectColumns+` FROM businesses b WHERE b.id = $1::uuid`,
		businessID,
	).Scan(&record.ID, &record.Name, &record.Slug, &record.PlatformStatus, &record.OwnerIdentitySummary, &record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.PlatformBusinessRecord{}, &RepositoryError{Operation: "platform_business.get", Kind: RepositoryNotFound, Err: err}
		}
		return ports.PlatformBusinessRecord{}, &RepositoryError{Operation: "platform_business.get", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

func (r *PlatformBusinessRepository) Suspend(ctx context.Context, businessID string, now time.Time) (ports.PlatformBusinessRecord, error) {
	return r.transition(ctx, "platform_business.suspend", businessID, now, []string{"active", "pending_setup"}, "suspended")
}

func (r *PlatformBusinessRepository) Reactivate(ctx context.Context, businessID string, now time.Time) (ports.PlatformBusinessRecord, error) {
	return r.transition(ctx, "platform_business.reactivate", businessID, now, []string{"suspended"}, "active")
}

func (r *PlatformBusinessRepository) Archive(ctx context.Context, businessID string, now time.Time) (ports.PlatformBusinessRecord, error) {
	// Per Contract §11: ACTIVE | SUSPENDED → ARCHIVED. ARCHIVED is terminal.
	return r.transition(ctx, "platform_business.archive", businessID, now, []string{"active", "suspended"}, "archived")
}

// transition applies a state machine transition filtered by current status.
// Returns RepositoryConflict if the current status is not in expectedFrom.
// Returns RepositoryNotFound if the business doesn't exist.
//
// Per Contract §11: ARCHIVED is terminal — no restore command in V1.
// Therefore the expectedFrom list for archive does NOT include "archived",
// making idempotent re-archive a hard error (not a silent no-op).
func (r *PlatformBusinessRepository) transition(ctx context.Context, op, businessID string, now time.Time, expectedFrom []string, target string) (ports.PlatformBusinessRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.PlatformBusinessRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" {
		return ports.PlatformBusinessRecord{}, invalidRepositoryInput(op, "business id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if len(expectedFrom) == 0 {
		return ports.PlatformBusinessRecord{}, invalidRepositoryInput(op, "expected source statuses are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PlatformBusinessRecord{}, err
	}
	// Build the IN list using the ANY($3::text[]) idiom for parameterized safety.
	var record ports.PlatformBusinessRecord
	err = executor.QueryRow(ctx,
		`UPDATE businesses SET status = $2, updated_at = $3
                 WHERE id = $1::uuid AND status = ANY($4::text[])
                 RETURNING `+platformBusinessReturnColumns,
		businessID, target, now, expectedFrom,
	).Scan(&record.ID, &record.Name, &record.Slug, &record.PlatformStatus, &record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Distinguish not-found vs invalid transition with a follow-up probe.
			var exists bool
			if checkErr := executor.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM businesses WHERE id = $1::uuid)`, businessID).Scan(&exists); checkErr != nil {
				return ports.PlatformBusinessRecord{}, &RepositoryError{Operation: op, Kind: RepositoryInvalid, Err: checkErr}
			}
			if !exists {
				return ports.PlatformBusinessRecord{}, &RepositoryError{Operation: op, Kind: RepositoryNotFound, Err: err}
			}
			return ports.PlatformBusinessRecord{}, &RepositoryError{
				Operation: op,
				Kind:      RepositoryConflict,
				Err:       fmt.Errorf("business %s is not in one of statuses %v (cannot transition to %s)", businessID, expectedFrom, target),
			}
		}
		return ports.PlatformBusinessRecord{}, &RepositoryError{Operation: op, Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

var _ ports.PlatformBusinessLifecyclePort = (*PlatformBusinessRepository)(nil)

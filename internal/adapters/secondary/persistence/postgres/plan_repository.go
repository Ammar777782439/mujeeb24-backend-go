package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PlanRepository implements ports.PlanRepository against Postgres.
//
// Per Platform Administration Contract §17-19:
//   - Plans are versioned. Once a subscription references a plan, that plan
//     version is immutable. Changes require creating a NEW plan version.
//   - Status lifecycle: DRAFT → ACTIVE → RETIRED. Transitions are forward-only.
//   - No DELETE endpoint — retired plans stay in the table for historical
//     subscription accounting.
type PlanRepository struct {
	adapter *Adapter
}

func NewPlanRepository(adapter *Adapter) *PlanRepository {
	return &PlanRepository{adapter: adapter}
}

func (r *PlanRepository) Create(ctx context.Context, create ports.PlanCreate) (ports.PlanRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.PlanRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(create.ID) == "" || strings.TrimSpace(create.Code) == "" || strings.TrimSpace(create.DisplayName) == "" {
		return ports.PlanRecord{}, invalidRepositoryInput("plan.create", "id, code, and display name are required")
	}
	if create.PriceYER <= 0 || create.AIReplyLimit <= 0 || create.ChannelLimit <= 0 || create.InternalAICostBudgetYER <= 0 {
		return ports.PlanRecord{}, invalidRepositoryInput("plan.create", "price, ai_reply_limit, channel_limit, and internal_ai_cost_budget must be positive")
	}
	if create.BillingInterval == "" {
		create.BillingInterval = "MONTH"
	}
	if create.Version < 1 {
		create.Version = 1
	}
	if create.Now.IsZero() {
		create.Now = time.Now().UTC()
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PlanRecord{}, err
	}
	var record ports.PlanRecord
	err = executor.QueryRow(ctx,
		`INSERT INTO plans (id, code, version, display_name, price_yer, billing_interval, ai_reply_limit, ai_catalog_limit, channel_limit, internal_ai_cost_budget_yer, status, created_at, updated_at)
                 VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'DRAFT', $11, $11)
                 RETURNING id::text, code, version, display_name, price_yer, billing_interval, ai_reply_limit, ai_catalog_limit, channel_limit, internal_ai_cost_budget_yer, status, created_at, updated_at, retired_at`,
		create.ID, create.Code, create.Version, create.DisplayName, create.PriceYER, create.BillingInterval,
		create.AIReplyLimit, create.AICatalogLimit, create.ChannelLimit, create.InternalAICostBudgetYER, create.Now,
	).Scan(&record.ID, &record.Code, &record.Version, &record.DisplayName, &record.PriceYER, &record.BillingInterval,
		&record.AIReplyLimit, &record.AICatalogLimit, &record.ChannelLimit, &record.InternalAICostBudgetYER,
		&record.Status, &record.CreatedAt, &record.UpdatedAt, &record.RetiredAt)
	if err != nil {
		return ports.PlanRecord{}, classifyRepositoryWriteError("plan.create", err)
	}
	return record, nil
}

func (r *PlanRepository) GetByID(ctx context.Context, planID string) (ports.PlanRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.PlanRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(planID) == "" {
		return ports.PlanRecord{}, invalidRepositoryInput("plan.get", "plan id is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PlanRecord{}, err
	}
	var record ports.PlanRecord
	err = executor.QueryRow(ctx,
		`SELECT id::text, code, version, display_name, price_yer, billing_interval, ai_reply_limit, ai_catalog_limit, channel_limit, internal_ai_cost_budget_yer, status, created_at, updated_at, retired_at
                 FROM plans WHERE id = $1::uuid`,
		planID,
	).Scan(&record.ID, &record.Code, &record.Version, &record.DisplayName, &record.PriceYER, &record.BillingInterval,
		&record.AIReplyLimit, &record.AICatalogLimit, &record.ChannelLimit, &record.InternalAICostBudgetYER,
		&record.Status, &record.CreatedAt, &record.UpdatedAt, &record.RetiredAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.PlanRecord{}, &RepositoryError{Operation: "plan.get", Kind: RepositoryNotFound, Err: err}
		}
		return ports.PlanRecord{}, &RepositoryError{Operation: "plan.get", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

func (r *PlanRepository) List(ctx context.Context, filter ports.PlanListFilter) (ports.PlanPage, error) {
	if r == nil || r.adapter == nil {
		return ports.PlanPage{}, ErrPoolClosed
	}
	if filter.Limit <= 0 || filter.Limit > 1000 {
		filter.Limit = 100
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PlanPage{}, err
	}
	rows, err := executor.Query(ctx,
		`SELECT id::text, code, version, display_name, price_yer, billing_interval, ai_reply_limit, ai_catalog_limit, channel_limit, internal_ai_cost_budget_yer, status, created_at, updated_at, retired_at
                 FROM plans
                 WHERE ($1 = '' OR status = $1)
                   AND ($2 = '' OR code = $2)
                 ORDER BY created_at DESC, id DESC
                 LIMIT $3`,
		filter.Status, filter.Code, filter.Limit,
	)
	if err != nil {
		return ports.PlanPage{}, &RepositoryError{Operation: "plan.list", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.PlanRecord, 0, filter.Limit)
	for rows.Next() {
		var record ports.PlanRecord
		if err := rows.Scan(&record.ID, &record.Code, &record.Version, &record.DisplayName, &record.PriceYER, &record.BillingInterval, &record.AIReplyLimit, &record.AICatalogLimit, &record.ChannelLimit, &record.InternalAICostBudgetYER, &record.Status, &record.CreatedAt, &record.UpdatedAt, &record.RetiredAt); err != nil {
			return ports.PlanPage{}, &RepositoryError{Operation: "plan.list", Kind: RepositoryInvalid, Err: err}
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return ports.PlanPage{}, &RepositoryError{Operation: "plan.list", Kind: RepositoryInvalid, Err: err}
	}
	return ports.PlanPage{Items: items}, nil
}

func (r *PlanRepository) Activate(ctx context.Context, planID string, now time.Time) (ports.PlanRecord, error) {
	return r.transitionStatus(ctx, "plan.activate", planID, now, "DRAFT", "ACTIVE")
}

func (r *PlanRepository) Retire(ctx context.Context, planID string, now time.Time) (ports.PlanRecord, error) {
	return r.transitionStatus(ctx, "plan.retire", planID, now, "ACTIVE", "RETIRED")
}

// CreateVersion implements Contract §17 + AIUsageTokenTelemetry.md §22-23.
//
// Atomic two-step SQL transaction:
//  1. INSERT new plan row with same code + (max_existing_version + 1) + new
//     fields, status=ACTIVE.
//  2. UPDATE base plan to status=RETIRED + retired_at=now.
//
// The transaction guarantees no two ACTIVE versions of the same code exist
// simultaneously. Subscriptions on the OLD version keep their snapshot of
// the limits (immutable per §17).
//
// Returns Conflict if the base plan is not ACTIVE.
// Returns NotFound if the base plan doesn't exist.
func (r *PlanRepository) CreateVersion(ctx context.Context, create ports.PlanVersionCreate) (ports.PlanRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.PlanRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(create.BasePlanID) == "" {
		return ports.PlanRecord{}, invalidRepositoryInput("plan.create_version", "base_plan_id is required")
	}
	if strings.TrimSpace(create.DisplayName) == "" {
		return ports.PlanRecord{}, invalidRepositoryInput("plan.create_version", "display_name is required")
	}
	if create.PriceYER <= 0 || create.AIReplyLimit <= 0 || create.ChannelLimit <= 0 || create.InternalAICostBudgetYER <= 0 {
		return ports.PlanRecord{}, invalidRepositoryInput("plan.create_version", "price, ai_reply_limit, channel_limit, and internal_ai_cost_budget must be positive")
	}
	if create.AICatalogLimit < 0 {
		return ports.PlanRecord{}, invalidRepositoryInput("plan.create_version", "ai_catalog_limit must be non-negative")
	}
	if create.Now.IsZero() {
		create.Now = time.Now().UTC()
	}
	if strings.TrimSpace(create.BillingInterval) == "" {
		create.BillingInterval = "MONTH"
	}
	// Fetch the base plan to verify it exists + is ACTIVE + get the code + next version.
	base, err := r.GetByID(ctx, create.BasePlanID)
	if err != nil {
		return ports.PlanRecord{}, err
	}
	if base.Status != "ACTIVE" {
		return ports.PlanRecord{}, &RepositoryError{
			Operation: "plan.create_version",
			Kind:      RepositoryConflict,
			Err:       fmt.Errorf("base plan %s is in status %s (must be ACTIVE to create a new version)", base.ID, base.Status),
		}
	}
	// Compute next version: max existing version for this code + 1.
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PlanRecord{}, err
	}
	var maxVersion int
	err = executor.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM plans WHERE code = $1`, base.Code).Scan(&maxVersion)
	if err != nil {
		return ports.PlanRecord{}, &RepositoryError{Operation: "plan.create_version", Kind: RepositoryInvalid, Err: err}
	}
	newVersion := maxVersion + 1
	newPlanID := uuid.NewString()
	var newRecord ports.PlanRecord
	err = executor.QueryRow(ctx,
		`WITH new_version AS (
                   INSERT INTO plans (id, code, version, display_name, price_yer, billing_interval, ai_reply_limit, ai_catalog_limit, channel_limit, internal_ai_cost_budget_yer, status, created_at, updated_at)
                   VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'ACTIVE', $11, $11)
                   RETURNING id::text, code, version, display_name, price_yer, billing_interval, ai_reply_limit, ai_catalog_limit, channel_limit, internal_ai_cost_budget_yer, status, created_at, updated_at, retired_at
                 )
                 UPDATE plans SET status = 'RETIRED', updated_at = $11, retired_at = $11
                   WHERE id = $12::uuid AND status = 'ACTIVE'
                 SELECT * FROM new_version`,
		newPlanID, base.Code, newVersion, create.DisplayName, create.PriceYER, create.BillingInterval,
		create.AIReplyLimit, create.AICatalogLimit, create.ChannelLimit, create.InternalAICostBudgetYER,
		create.Now, create.BasePlanID,
	).Scan(
		&newRecord.ID, &newRecord.Code, &newRecord.Version, &newRecord.DisplayName, &newRecord.PriceYER, &newRecord.BillingInterval,
		&newRecord.AIReplyLimit, &newRecord.AICatalogLimit, &newRecord.ChannelLimit, &newRecord.InternalAICostBudgetYER,
		&newRecord.Status, &newRecord.CreatedAt, &newRecord.UpdatedAt, &newRecord.RetiredAt,
	)
	if err != nil {
		return ports.PlanRecord{}, classifyRepositoryWriteError("plan.create_version", err)
	}
	return newRecord, nil
}

// transitionStatus implements the forward-only DRAFT → ACTIVE → RETIRED
// lifecycle. Per Contract §18: a DRAFT cannot be subscribed to; ACTIVE can
// receive new subscriptions; RETIRED blocks new subscriptions but historical
// ones remain valid. We enforce via WHERE status = $expected_from.
func (r *PlanRepository) transitionStatus(ctx context.Context, op, planID string, now time.Time, expectedFrom, target string) (ports.PlanRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.PlanRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(planID) == "" {
		return ports.PlanRecord{}, invalidRepositoryInput(op, "plan id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.PlanRecord{}, err
	}
	var retiredAtArg any
	if target == "RETIRED" {
		retiredAtArg = now
	} else {
		retiredAtArg = nil
	}
	var record ports.PlanRecord
	err = executor.QueryRow(ctx,
		`UPDATE plans SET status = $3, updated_at = $2, retired_at = $4
                 WHERE id = $1::uuid AND status = $5
                 RETURNING id::text, code, version, display_name, price_yer, billing_interval, ai_reply_limit, ai_catalog_limit, channel_limit, internal_ai_cost_budget_yer, status, created_at, updated_at, retired_at`,
		planID, now, target, retiredAtArg, expectedFrom,
	).Scan(&record.ID, &record.Code, &record.Version, &record.DisplayName, &record.PriceYER, &record.BillingInterval,
		&record.AIReplyLimit, &record.AICatalogLimit, &record.ChannelLimit, &record.InternalAICostBudgetYER,
		&record.Status, &record.CreatedAt, &record.UpdatedAt, &record.RetiredAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.PlanRecord{}, &RepositoryError{Operation: op, Kind: RepositoryConflict, Err: fmt.Errorf("plan %s is not in status %s (expected transition %s → %s)", planID, expectedFrom, expectedFrom, target)}
		}
		return ports.PlanRecord{}, &RepositoryError{Operation: op, Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

var _ ports.PlanRepository = (*PlanRepository)(nil)

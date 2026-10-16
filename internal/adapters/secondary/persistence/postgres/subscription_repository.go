package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SubscriptionRepository implements ports.SubscriptionRepository against Postgres.
//
// Per Platform Administration Contract V1 §22: there is NO Update method.
// Renewal creates a new subscription row. The historical row stays immutable.
//
// Per Contract §28: Activate transitions PENDING → ACTIVE. The activation
// happens atomically with payment recording in the same transaction (handled
// at the service layer; the repository methods are individual SQL statements
// that the service composes via Within()).
//
// Per Contract §11 / §29: Cancel / MarkExpired / Archive are terminal
// transitions — they fail with RepositoryConflict if the current status is
// already terminal, so idempotent re-cancellation is a hard error (not a
// silent no-op).
type SubscriptionRepository struct {
	adapter *Adapter
}

func NewSubscriptionRepository(adapter *Adapter) *SubscriptionRepository {
	return &SubscriptionRepository{adapter: adapter}
}

const subscriptionSelectColumns = `id::text, business_id::text, plan_id::text, (SELECT code FROM plans p WHERE p.id = plan_id), (SELECT version FROM plans p WHERE p.id = plan_id), price_yer_snapshot, amount_due_yer, period_start, period_end, status, ai_reply_limit, ai_catalog_limit, included_channel_limit, channel_ceiling, seat_limit, internal_ai_cost_budget_yer, cost_budget_override_yer, cost_budget_override_reason, cost_budget_override_by::text, cost_budget_override_at, cancelled_at, cancelled_reason, cancelled_by::text, activated_at, created_at, updated_at`

func scanSubscription(scanner interface {
	Scan(dest ...any) error
}, record *ports.SubscriptionRecord) error {
	return scanner.Scan(
		&record.ID, &record.BusinessID, &record.PlanID, &record.PlanCode, &record.PlanVersion,
		&record.PriceYERSnapshot, &record.AmountDueYER,
		&record.PeriodStart, &record.PeriodEnd, &record.Status,
		&record.AIReplyLimit, &record.AICatalogLimit, &record.ChannelLimit, &record.ChannelCeiling, &record.SeatLimit, &record.InternalAICostBudgetYER,
		&record.CostBudgetOverrideYER, &record.CostBudgetOverrideReason, &record.CostBudgetOverrideBy, &record.CostBudgetOverrideAt,
		&record.CancelledAt, &record.CancelledReason, &record.CancelledBy, &record.ActivatedAt,
		&record.CreatedAt, &record.UpdatedAt,
	)
}

func (r *SubscriptionRepository) Create(ctx context.Context, create ports.SubscriptionCreate) (ports.SubscriptionRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.SubscriptionRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(create.ID) == "" || strings.TrimSpace(create.BusinessID) == "" || strings.TrimSpace(create.PlanID) == "" {
		return ports.SubscriptionRecord{}, invalidRepositoryInput("subscription.create", "id, business_id, and plan_id are required")
	}
	if !create.PeriodEnd.After(create.PeriodStart) {
		return ports.SubscriptionRecord{}, invalidRepositoryInput("subscription.create", "period_end must be after period_start")
	}
	if create.AIReplyLimit <= 0 || create.ChannelLimit <= 0 || create.InternalAICostBudgetYER <= 0 {
		return ports.SubscriptionRecord{}, invalidRepositoryInput("subscription.create", "ai_reply_limit, channel_limit, and internal_ai_cost_budget must be positive")
	}
	if create.AICatalogLimit < 0 {
		return ports.SubscriptionRecord{}, invalidRepositoryInput("subscription.create", "ai_catalog_limit must be non-negative")
	}
	if create.PriceYERSnapshot <= 0 || create.AmountDueYER <= 0 {
		return ports.SubscriptionRecord{}, invalidRepositoryInput("subscription.create", "price snapshot and amount due must be positive")
	}
	if create.ChannelCeiling == 0 {
		create.ChannelCeiling = create.ChannelLimit
	}
	if create.SeatLimit == 0 {
		create.SeatLimit = 1
	}
	if create.ChannelCeiling < create.ChannelLimit || create.SeatLimit < 1 {
		return ports.SubscriptionRecord{}, invalidRepositoryInput("subscription.create", "channel ceiling and seat limit are invalid")
	}
	if create.Now.IsZero() {
		create.Now = time.Now().UTC()
	}
	var record ports.SubscriptionRecord
	err := r.adapter.Within(ctx, func(txCtx context.Context) error {
		executor, executorErr := r.adapter.Executor(txCtx)
		if executorErr != nil {
			return executorErr
		}
		// Serialize subscription creation per business so overlapping periods
		// cannot pass the existence check concurrently.
		if _, lockErr := executor.Exec(txCtx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, create.BusinessID); lockErr != nil {
			return lockErr
		}
		var overlaps bool
		if queryErr := executor.QueryRow(txCtx,
			`SELECT EXISTS (
			   SELECT 1 FROM subscriptions
			   WHERE business_id=$1::uuid AND status IN ('PENDING','ACTIVE')
			     AND tstzrange(period_start, period_end, '[)') && tstzrange($2, $3, '[)')
			 )`, create.BusinessID, create.PeriodStart, create.PeriodEnd).Scan(&overlaps); queryErr != nil {
			return queryErr
		}
		if overlaps {
			return &RepositoryError{Operation: "subscription.create", Kind: RepositoryConflict, Err: errors.New("subscription period overlaps an existing PENDING or ACTIVE period")}
		}
		return scanSubscription(executor.QueryRow(txCtx,
			`INSERT INTO subscriptions (id, business_id, plan_id, price_yer_snapshot, amount_due_yer, period_start, period_end, status, ai_reply_limit, ai_catalog_limit, channel_limit, included_channel_limit, channel_ceiling, seat_limit, internal_ai_cost_budget_yer, created_at, updated_at)
			 VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,'PENDING',$8,$9,$10,$10,$11,$12,$13,$14,$14)
			 RETURNING `+subscriptionSelectColumns,
			create.ID, create.BusinessID, create.PlanID, create.PriceYERSnapshot, create.AmountDueYER,
			create.PeriodStart, create.PeriodEnd, create.AIReplyLimit, create.AICatalogLimit,
			create.ChannelLimit, create.ChannelCeiling, create.SeatLimit, create.InternalAICostBudgetYER, create.Now), &record)
	})
	if err != nil {
		return ports.SubscriptionRecord{}, classifyRepositoryWriteError("subscription.create", err)
	}
	return record, nil
}

func (r *SubscriptionRepository) GetByID(ctx context.Context, subscriptionID string) (ports.SubscriptionRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.SubscriptionRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(subscriptionID) == "" {
		return ports.SubscriptionRecord{}, invalidRepositoryInput("subscription.get", "subscription id is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SubscriptionRecord{}, err
	}
	var record ports.SubscriptionRecord
	err = scanSubscription(executor.QueryRow(ctx,
		`SELECT `+subscriptionSelectColumns+` FROM subscriptions WHERE id = $1::uuid`,
		subscriptionID,
	), &record)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.SubscriptionRecord{}, &RepositoryError{Operation: "subscription.get", Kind: RepositoryNotFound, Err: err}
		}
		return ports.SubscriptionRecord{}, &RepositoryError{Operation: "subscription.get", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

func (r *SubscriptionRepository) List(ctx context.Context, filter ports.SubscriptionListFilter) (ports.SubscriptionPage, error) {
	if r == nil || r.adapter == nil {
		return ports.SubscriptionPage{}, ErrPoolClosed
	}
	if filter.Limit <= 0 || filter.Limit > 1000 {
		filter.Limit = 100
	}
	decoded, err := decodeSubscriptionCursor(filter.Cursor)
	if err != nil {
		return ports.SubscriptionPage{}, invalidRepositoryInput("subscription.list", err.Error())
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SubscriptionPage{}, err
	}
	var cursorAt any
	var cursorID any
	if decoded != nil {
		cursorAt, cursorID = decoded.CreatedAt, decoded.ID
	}
	rows, err := executor.Query(ctx,
		`SELECT `+subscriptionSelectColumns+`
                 FROM subscriptions
                 WHERE ($1 = '' OR business_id::text = $1)
                   AND ($2 = '' OR status = $2)
                   AND ($3::timestamptz IS NULL OR (created_at, id) < ($3, $4::uuid))
                 ORDER BY created_at DESC, id DESC
                 LIMIT $5`,
		strings.TrimSpace(filter.BusinessID), strings.TrimSpace(filter.Status), cursorAt, cursorID, filter.Limit+1,
	)
	if err != nil {
		return ports.SubscriptionPage{}, &RepositoryError{Operation: "subscription.list", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.SubscriptionRecord, 0, filter.Limit)
	for rows.Next() {
		var record ports.SubscriptionRecord
		if err := scanSubscription(rows, &record); err != nil {
			return ports.SubscriptionPage{}, &RepositoryError{Operation: "subscription.list", Kind: RepositoryInvalid, Err: err}
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return ports.SubscriptionPage{}, &RepositoryError{Operation: "subscription.list", Kind: RepositoryInvalid, Err: err}
	}
	page := ports.SubscriptionPage{Items: items}
	if len(items) > filter.Limit {
		page.HasMore = true
		page.Items = items[:filter.Limit]
		page.NextCursor = encodeSubscriptionCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

// Activate transitions PENDING → ACTIVE. Per Contract §28: a subscription
// becomes ACTIVE only after a payment is recorded (handled at the service
// layer which calls Append(payment) + Activate in the same transaction).
//
// Returns RepositoryConflict if the current status is not PENDING.
func (r *SubscriptionRepository) Activate(ctx context.Context, subscriptionID string, now time.Time) (ports.SubscriptionRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.SubscriptionRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(subscriptionID) == "" {
		return ports.SubscriptionRecord{}, invalidRepositoryInput("subscription.activate", "subscription id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SubscriptionRecord{}, err
	}
	var record ports.SubscriptionRecord
	err = scanSubscription(executor.QueryRow(ctx,
		`UPDATE subscriptions SET status = 'ACTIVE', activated_at = $2, updated_at = $2
		 WHERE id = $1::uuid AND status = 'PENDING' AND period_start <= $2 AND period_end > $2
		 RETURNING `+subscriptionSelectColumns,
		subscriptionID, now,
	), &record)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			current, getErr := r.GetByID(ctx, subscriptionID)
			if getErr != nil {
				return ports.SubscriptionRecord{}, &RepositoryError{Operation: "subscription.activate", Kind: RepositoryNotFound, Err: getErr}
			}
			return ports.SubscriptionRecord{}, &RepositoryError{
				Operation: "subscription.activate",
				Kind:      RepositoryConflict,
				Err:       fmt.Errorf("subscription %s is in status %s (cannot transition to ACTIVE)", subscriptionID, current.Status),
			}
		}
		return ports.SubscriptionRecord{}, &RepositoryError{Operation: "subscription.activate", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

// Cancel transitions PENDING|ACTIVE → CANCELLED. Per Contract §21: this is
// a Platform Admin action (not an auto-expiry). The cancelled_by is the
// platform super admin principal_id.
//
// Returns RepositoryConflict if already terminal (EXPIRED or CANCELLED).
func (r *SubscriptionRepository) Cancel(ctx context.Context, subscriptionID, reason, cancelledBy string, now time.Time) (ports.SubscriptionRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.SubscriptionRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(subscriptionID) == "" {
		return ports.SubscriptionRecord{}, invalidRepositoryInput("subscription.cancel", "subscription id is required")
	}
	if strings.TrimSpace(reason) == "" {
		return ports.SubscriptionRecord{}, invalidRepositoryInput("subscription.cancel", "cancellation reason is required")
	}
	if strings.TrimSpace(cancelledBy) == "" {
		return ports.SubscriptionRecord{}, invalidRepositoryInput("subscription.cancel", "cancelled_by is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var record ports.SubscriptionRecord
	err := r.adapter.Within(ctx, func(tx context.Context) error {
		executor, err := r.adapter.Executor(tx)
		if err != nil {
			return err
		}
		err = scanSubscription(executor.QueryRow(tx,
			`UPDATE subscriptions
                 SET status = 'CANCELLED', updated_at = $2,
                     cancelled_at = $2, cancelled_reason = $3, cancelled_by = $4::uuid
                 WHERE id = $1::uuid AND status IN ('PENDING', 'ACTIVE')
                 RETURNING `+subscriptionSelectColumns,
			subscriptionID, now, reason, cancelledBy,
		), &record)
		if err != nil {
			return err
		}
		_, err = executor.Exec(tx, `UPDATE subscription_addons
			SET status='CANCELLED', cancelled_at=$2, cancelled_by=$3::uuid,
				cancelled_reason='subscription cancelled', updated_at=$2
			WHERE subscription_id=$1::uuid AND status IN ('PENDING','ACTIVE')`, subscriptionID, now, cancelledBy)
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			current, getErr := r.GetByID(ctx, subscriptionID)
			if getErr != nil {
				return ports.SubscriptionRecord{}, &RepositoryError{Operation: "subscription.cancel", Kind: RepositoryNotFound, Err: getErr}
			}
			return ports.SubscriptionRecord{}, &RepositoryError{
				Operation: "subscription.cancel",
				Kind:      RepositoryConflict,
				Err:       fmt.Errorf("subscription %s is in status %s (cannot transition to CANCELLED)", subscriptionID, current.Status),
			}
		}
		return ports.SubscriptionRecord{}, &RepositoryError{Operation: "subscription.cancel", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

// MarkExpired transitions ACTIVE → EXPIRED. Used by the expiry worker.
// Per Contract §29: expiry preserves all business data.
//
// Returns RepositoryConflict if not ACTIVE — but since this is called by
// a worker, we treat EXPIRED as idempotent (the worker can re-attempt
// without error). Other statuses (PENDING/CANCELLED) are a hard conflict.
func (r *SubscriptionRepository) MarkExpired(ctx context.Context, subscriptionID string, now time.Time) (ports.SubscriptionRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.SubscriptionRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(subscriptionID) == "" {
		return ports.SubscriptionRecord{}, invalidRepositoryInput("subscription.mark_expired", "subscription id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var record ports.SubscriptionRecord
	err := r.adapter.Within(ctx, func(tx context.Context) error {
		executor, err := r.adapter.Executor(tx)
		if err != nil {
			return err
		}
		err = scanSubscription(executor.QueryRow(tx,
			`UPDATE subscriptions SET status = 'EXPIRED', updated_at = $2
		 WHERE id = $1::uuid AND status = 'ACTIVE' AND period_end <= $2
                 RETURNING `+subscriptionSelectColumns,
			subscriptionID, now,
		), &record)
		if err != nil {
			return err
		}
		_, err = executor.Exec(tx, `UPDATE subscription_addons
			SET status='CANCELLED', cancelled_at=$2,
				cancelled_reason='subscription period expired', updated_at=$2
			WHERE subscription_id=$1::uuid AND status IN ('PENDING','ACTIVE')`, subscriptionID, now)
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Idempotent: subscription is no longer ACTIVE (could already be EXPIRED).
			// Fetch current state to verify it's terminal — return NotFound if missing.
			current, getErr := r.GetByID(ctx, subscriptionID)
			if getErr != nil {
				return ports.SubscriptionRecord{}, getErr
			}
			if current.Status == "EXPIRED" {
				return current, nil
			}
			return ports.SubscriptionRecord{}, &RepositoryError{
				Operation: "subscription.mark_expired",
				Kind:      RepositoryConflict,
				Err:       fmt.Errorf("subscription %s is in status %s (cannot transition to EXPIRED)", subscriptionID, current.Status),
			}
		}
		return ports.SubscriptionRecord{}, &RepositoryError{Operation: "subscription.mark_expired", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

// ApplyCostBudgetOverride updates the per-subscription cost budget override.
// Per Contract §24 (AIUsageTokenTelemetry.md): this does NOT change plan
// version. The override is recorded with reason + override_by + timestamp
// for audit.
//
// Per Contract §47 (Platform Audit): every override writes a platform_audit_event.
// The audit is the responsibility of the service layer (handler facade) —
// the repository just persists the override fields.
func (r *SubscriptionRepository) ApplyCostBudgetOverride(ctx context.Context, subscriptionID string, newBudgetYER int, reason, overrideBy string, now time.Time) (ports.SubscriptionRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.SubscriptionRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(subscriptionID) == "" {
		return ports.SubscriptionRecord{}, invalidRepositoryInput("subscription.cost_budget_override", "subscription id is required")
	}
	if newBudgetYER <= 0 {
		return ports.SubscriptionRecord{}, invalidRepositoryInput("subscription.cost_budget_override", "new_budget_yer must be positive")
	}
	if strings.TrimSpace(reason) == "" {
		return ports.SubscriptionRecord{}, invalidRepositoryInput("subscription.cost_budget_override", "reason is required")
	}
	if strings.TrimSpace(overrideBy) == "" {
		return ports.SubscriptionRecord{}, invalidRepositoryInput("subscription.cost_budget_override", "override_by is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SubscriptionRecord{}, err
	}
	var record ports.SubscriptionRecord
	err = scanSubscription(executor.QueryRow(ctx,
		`UPDATE subscriptions
                 SET cost_budget_override_yer = $2,
                     cost_budget_override_reason = $3,
                     cost_budget_override_by = $4::uuid,
                     cost_budget_override_at = $5,
                     updated_at = $5
                 WHERE id = $1::uuid
                 RETURNING `+subscriptionSelectColumns,
		subscriptionID, newBudgetYER, reason, overrideBy, now,
	), &record)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.SubscriptionRecord{}, &RepositoryError{Operation: "subscription.cost_budget_override", Kind: RepositoryNotFound, Err: err}
		}
		return ports.SubscriptionRecord{}, &RepositoryError{Operation: "subscription.cost_budget_override", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

type subscriptionCursor struct {
	CreatedAt time.Time
	ID        string
}

func encodeSubscriptionCursor(record ports.SubscriptionRecord) string {
	return base64.RawURLEncoding.EncodeToString([]byte(record.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + record.ID))
}

func decodeSubscriptionCursor(value string) (*subscriptionCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid subscription cursor")
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid subscription cursor")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid subscription cursor")
	}
	if err := validateUUID(parts[1]); err != nil {
		return nil, fmt.Errorf("invalid subscription cursor")
	}
	return &subscriptionCursor{CreatedAt: createdAt, ID: parts[1]}, nil
}

func validateUUID(id string) error {
	return uuid.Validate(id)
}

var _ ports.SubscriptionRepository = (*SubscriptionRepository)(nil)

func (r *SubscriptionRepository) ExpireDue(ctx context.Context, now time.Time, limit int) (int, error) {
	if r == nil || r.adapter == nil {
		return 0, ErrPoolClosed
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return 0, err
	}
	var count int
	err = executor.QueryRow(ctx,
		`WITH due AS (
		   SELECT id FROM subscriptions
		   WHERE status='ACTIVE' AND period_end <= $1
		   ORDER BY period_end, id
		   FOR UPDATE SKIP LOCKED
		   LIMIT $2
		 ), expired AS (
		   UPDATE subscriptions s
		   SET status='EXPIRED', updated_at=$1
		   FROM due
		   WHERE s.id=due.id
		   RETURNING s.id
		 ), cancelled_addons AS (
		   UPDATE subscription_addons a
		   SET status='CANCELLED', cancelled_at=$1,
		       cancelled_reason='subscription period expired', updated_at=$1
		   FROM expired e
		   WHERE a.subscription_id=e.id AND a.status IN ('PENDING','ACTIVE')
		 ) SELECT COUNT(*)::int FROM expired`, now, limit).Scan(&count)
	if err != nil {
		return 0, &RepositoryError{Operation: "subscription.expire_due", Kind: RepositoryInvalid, Err: err}
	}
	return count, nil
}

var _ ports.SubscriptionExpiryRepository = (*SubscriptionRepository)(nil)

func (r *SubscriptionRepository) EnsureCatalogItemActivation(ctx context.Context, businessID, itemID string, now time.Time) error {
	if r == nil || r.adapter == nil {
		return ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(itemID) == "" {
		return invalidRepositoryInput("subscription.catalog_activation", "business and item ids are required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return err
	}
	if _, err = executor.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, businessID); err != nil {
		return &RepositoryError{Operation: "subscription.catalog_activation.lock", Kind: RepositoryInvalid, Err: err}
	}
	var limit int
	if err = executor.QueryRow(ctx,
		`SELECT ai_catalog_limit + COALESCE((SELECT sum(catalog_delta) FROM subscription_addons WHERE subscription_id=subscriptions.id AND status='ACTIVE'),0) FROM subscriptions
		 WHERE business_id=$1::uuid AND status='ACTIVE' AND period_start <= $2 AND period_end > $2`,
		businessID, now).Scan(&limit); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &RepositoryError{Operation: "subscription.catalog_activation", Kind: RepositoryConflict, Err: errors.New("an active subscription is required to activate a catalog item")}
		}
		return &RepositoryError{Operation: "subscription.catalog_activation", Kind: RepositoryInvalid, Err: err}
	}
	var active int
	if err = executor.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM catalog_items WHERE business_id=$1::uuid AND status='active' AND id<>$2::uuid`,
		businessID, itemID).Scan(&active); err != nil {
		return &RepositoryError{Operation: "subscription.catalog_activation", Kind: RepositoryInvalid, Err: err}
	}
	if active >= limit {
		return &RepositoryError{Operation: "subscription.catalog_activation", Kind: RepositoryConflict, Err: fmt.Errorf("active catalog product limit %d has been reached", limit)}
	}
	return nil
}

var _ ports.CatalogActivationEntitlement = (*SubscriptionRepository)(nil)

// CheckEntitlements implements Contract §34-35.
//
// Counts the business's active catalog items, active channel connections, and
// active team memberships. If any count exceeds the subscription's plan limits, returns a Conflict
// error so the activation flow can block the transition to ACTIVE.
//
// Per Contract §34: "لا يسمح النظام برفع عدد السجلات الداخلة في AI-active scope فوق الحد الخاص بالـPlan"
// Per Contract §35: "لا يسمح بوجود أكثر من قناة نشطة ضمن entitlement"
// Per Contract §34-35: "لا يوجد Silent Data Deletion" — we block, we don't delete.
func (r *SubscriptionRepository) CheckEntitlements(ctx context.Context, businessID string, catalogLimit, channelLimit, seatLimit int) error {
	if r == nil || r.adapter == nil {
		return ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" {
		return invalidRepositoryInput("subscription.check_entitlements", "business_id is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return err
	}
	// Only active catalog items are in the AI-facing commercial scope. Draft
	// and archived records remain stored without consuming a plan allowance.
	var catalogCount int
	err = executor.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM catalog_items WHERE business_id = $1::uuid AND status = 'active'`,
		businessID,
	).Scan(&catalogCount)
	if err != nil {
		return &RepositoryError{Operation: "subscription.check_entitlements.catalog", Kind: RepositoryInvalid, Err: err}
	}
	if catalogCount > catalogLimit {
		return &RepositoryError{
			Operation: "subscription.check_entitlements.catalog",
			Kind:      RepositoryConflict,
			Err:       fmt.Errorf("catalog item count %d exceeds plan limit %d — resolve the overrun before activation (Contract §34)", catalogCount, catalogLimit),
		}
	}
	// Count active channel connections for this business.
	// Per Contract §91: channel_connections uses existing statuses: pending,
	// active, disconnected, failed, reconnect_required, archived.
	// We count only status='active' per Contract §35.
	var channelCount int
	err = executor.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM channel_connections WHERE business_id = $1::uuid AND status = 'active'`,
		businessID,
	).Scan(&channelCount)
	if err != nil {
		return &RepositoryError{Operation: "subscription.check_entitlements.channels", Kind: RepositoryInvalid, Err: err}
	}
	if channelCount > channelLimit {
		return &RepositoryError{
			Operation: "subscription.check_entitlements.channels",
			Kind:      RepositoryConflict,
			Err:       fmt.Errorf("active channel count %d exceeds plan limit %d — resolve the overrun before activation (Contract §35)", channelCount, channelLimit),
		}
	}
	var seatCount int
	err = executor.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM business_memberships WHERE business_id = $1::uuid AND status = 'active'`,
		businessID,
	).Scan(&seatCount)
	if err != nil {
		return &RepositoryError{Operation: "subscription.check_entitlements.seats", Kind: RepositoryInvalid, Err: err}
	}
	if seatCount > seatLimit {
		return &RepositoryError{
			Operation: "subscription.check_entitlements.seats",
			Kind:      RepositoryConflict,
			Err:       fmt.Errorf("active seat count %d exceeds plan limit %d — resolve the overrun before activation", seatCount, seatLimit),
		}
	}
	return nil
}

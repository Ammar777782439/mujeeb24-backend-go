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

const subscriptionSelectColumns = `s.id::text, s.business_id::text, s.plan_id::text, p.code AS plan_code, p.version AS plan_version, s.period_start, s.period_end, s.status, s.ai_reply_limit, s.ai_catalog_limit, s.channel_limit, s.internal_ai_cost_budget_yer, s.cost_budget_override_yer, s.cost_budget_override_reason, s.cost_budget_override_by::text, s.cost_budget_override_at, s.cancelled_at, s.cancelled_reason, s.cancelled_by::text, s.created_at, s.updated_at`

func scanSubscription(scanner interface {
        Scan(dest ...any) error
}, record *ports.SubscriptionRecord) error {
        return scanner.Scan(
                &record.ID, &record.BusinessID, &record.PlanID, &record.PlanCode, &record.PlanVersion,
                &record.PeriodStart, &record.PeriodEnd, &record.Status,
                &record.AIReplyLimit, &record.AICatalogLimit, &record.ChannelLimit, &record.InternalAICostBudgetYER,
                &record.CostBudgetOverrideYER, &record.CostBudgetOverrideReason, &record.CostBudgetOverrideBy, &record.CostBudgetOverrideAt,
                &record.CancelledAt, &record.CancelledReason, &record.CancelledBy,
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
        if create.Now.IsZero() {
                create.Now = time.Now().UTC()
        }
        executor, err := r.adapter.Executor(ctx)
        if err != nil {
                return ports.SubscriptionRecord{}, err
        }
        // Per spec §3: INSERT then SELECT. The previous INSERT used
        // `RETURNING subscriptionSelectColumns` which references `s.*` and `p.*`
        // aliases — but those aliases are NOT defined in an INSERT context (they
        // require `FROM subscriptions s LEFT JOIN plans p`). PostgreSQL rejects
        // this with "missing FROM-clause entry for table s" / "p".
        //
        // Fix: INSERT with RETURNING id (just the PK — safe because it's a
        // column of the target table, no alias needed), then call GetByID to
        // fetch the full record with the JOIN. Both use the SAME executor
        // resolved from ctx — so if the caller is inside a transaction, both
        // the INSERT + SELECT participate in the same tx.
        var insertedID string
        err = executor.QueryRow(ctx,
                `INSERT INTO subscriptions (id, business_id, plan_id, period_start, period_end, status, ai_reply_limit, ai_catalog_limit, channel_limit, internal_ai_cost_budget_yer, created_at, updated_at)
                 VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, 'PENDING', $6, $7, $8, $9, $10, $10)
                 RETURNING id::text`,
                create.ID, create.BusinessID, create.PlanID, create.PeriodStart, create.PeriodEnd,
                create.AIReplyLimit, create.AICatalogLimit, create.ChannelLimit, create.InternalAICostBudgetYER,
                create.Now,
        ).Scan(&insertedID)
        if err != nil {
                return ports.SubscriptionRecord{}, classifyRepositoryWriteError("subscription.create", err)
        }
        if strings.TrimSpace(insertedID) == "" {
                return ports.SubscriptionRecord{}, &RepositoryError{Operation: "subscription.create", Kind: RepositoryInvalid, Err: fmt.Errorf("insert returned empty id")}
        }
        // Fetch the full record with the JOIN (plan_code + plan_version come from
        // the plans table — not available in the INSERT...RETURNING context).
        record, err := r.GetByID(ctx, insertedID)
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
        err = executor.QueryRow(ctx,
                `SELECT `+subscriptionSelectColumns+` FROM subscriptions s LEFT JOIN plans p ON p.id = s.plan_id WHERE s.id = $1::uuid`,
                subscriptionID,
        ).Scan(
                &record.ID, &record.BusinessID, &record.PlanID, &record.PlanCode, &record.PlanVersion,
                &record.PeriodStart, &record.PeriodEnd, &record.Status,
                &record.AIReplyLimit, &record.AICatalogLimit, &record.ChannelLimit, &record.InternalAICostBudgetYER,
                &record.CostBudgetOverrideYER, &record.CostBudgetOverrideReason, &record.CostBudgetOverrideBy, &record.CostBudgetOverrideAt,
                &record.CancelledAt, &record.CancelledReason, &record.CancelledBy,
                &record.CreatedAt, &record.UpdatedAt,
        )
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
                 FROM subscriptions s
                 LEFT JOIN plans p ON p.id = s.plan_id
                 WHERE ($1 = '' OR s.business_id::text = $1)
                   AND ($2 = '' OR s.status = $2)
                   AND ($3::timestamptz IS NULL OR (s.created_at, s.id) < ($3, $4::uuid))
                 ORDER BY s.created_at DESC, s.id DESC
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
	var updatedID string
	err = executor.QueryRow(ctx,
		`UPDATE subscriptions SET status = 'ACTIVE', updated_at = $2
		 WHERE id = $1::uuid AND status = 'PENDING'
		 RETURNING id::text`,
		subscriptionID, now,
	).Scan(&updatedID)
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
	return r.GetByID(ctx, updatedID)
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
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SubscriptionRecord{}, err
	}
	var updatedID string
	err = executor.QueryRow(ctx,
		`UPDATE subscriptions
		 SET status = 'CANCELLED', updated_at = $2,
		     cancelled_at = $2, cancelled_reason = $3, cancelled_by = $4::uuid
		 WHERE id = $1::uuid AND status IN ('PENDING', 'ACTIVE')
		 RETURNING id::text`,
		subscriptionID, now, reason, cancelledBy,
	).Scan(&updatedID)
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
	return r.GetByID(ctx, updatedID)
}

// MarkExpired transitions ACTIVE → EXPIRED. Used by the expiry worker.
// Per Contract §29: expiry preserves all business data.
//
// Returns RepositoryConflict if not ACTIVE — but since this is called by
// a worker, we treat EXPIRED as idempotent (the worker can re-attempt
// without error). Other statuses (PENDING/CANCELLED) are a hard conflict.
func (r *SubscriptionRepository) MarkExpired(ctx context.Context, subscriptionID string, now time.Time) (ports.SubscriptionRecord, error) {
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SubscriptionRecord{}, err
	}
	var updatedID string
	err = executor.QueryRow(ctx,
		`UPDATE subscriptions SET status = 'EXPIRED', updated_at = $2
		 WHERE id = $1::uuid AND status = 'ACTIVE'
		 RETURNING id::text`,
		subscriptionID, now,
	).Scan(&updatedID)
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
	return r.GetByID(ctx, updatedID)
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
	var updatedID string
	err = executor.QueryRow(ctx,
		`UPDATE subscriptions
		 SET cost_budget_override_yer = $2,
		     cost_budget_override_reason = $3,
		     cost_budget_override_by = $4::uuid,
		     cost_budget_override_at = $5,
		     updated_at = $5
		 WHERE id = $1::uuid
		 RETURNING id::text`,
		subscriptionID, newBudgetYER, reason, overrideBy, now,
	).Scan(&updatedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.SubscriptionRecord{}, &RepositoryError{Operation: "subscription.cost_budget_override", Kind: RepositoryNotFound, Err: err}
		}
		return ports.SubscriptionRecord{}, &RepositoryError{Operation: "subscription.cost_budget_override", Kind: RepositoryInvalid, Err: err}
	}
	return r.GetByID(ctx, updatedID)
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

// CheckEntitlements implements Contract §34-35.
//
// Counts the business's current catalog items (in catalog_items table) and
// active channel connections (in channel_connections table with status='active').
// If either count exceeds the subscription's plan limits, returns a Conflict
// error so the activation flow can block the transition to ACTIVE.
//
// Per Contract §34: "لا يسمح النظام برفع عدد السجلات الداخلة في AI-active scope فوق الحد الخاص بالـPlan"
// Per Contract §35: "لا يسمح بوجود أكثر من قناة نشطة ضمن entitlement"
// Per Contract §34-35: "لا يوجد Silent Data Deletion" — we block, we don't delete.
func (r *SubscriptionRepository) CheckEntitlements(ctx context.Context, businessID string, catalogLimit, channelLimit int) error {
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
        // Count catalog items for this business.
        // Per Contract §33: "AI Catalog Limit" limits the records in AI-active
        // scope, not the DB storage. For V1 without an AI-active flag, we count
        // all catalog_items as a proxy — the contract explicitly says the limit
        // is about AI-active scope, but without the flag, total count is the
        // closest available metric.
        var catalogCount int
        err = executor.QueryRow(ctx,
                `SELECT COUNT(*)::int FROM catalog_items WHERE business_id = $1::uuid`,
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
        return nil
}

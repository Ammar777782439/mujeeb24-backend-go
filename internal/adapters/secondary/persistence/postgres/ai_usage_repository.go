package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// AIUsageRepository implements ports.AIUsageRepository against Postgres.
//
// Per AIUsageTokenTelemetry.md §6, §30: ai_usage_records is APPEND ONLY.
// There is intentionally NO Update / Delete method.
//
// Per Contract §28: AppendRecord inserts the per-execution row + refreshes
// the per-subscription aggregate snapshot in the same SQL transaction so
// the platform admin sees up-to-date numbers.
type AIUsageRepository struct {
	adapter *Adapter
}

func NewAIUsageRepository(adapter *Adapter) *AIUsageRepository {
	return &AIUsageRepository{adapter: adapter}
}

const aiUsageRecordSelectColumns = `id::text, business_id::text, subscription_id::text, provider, model, input_tokens, cached_input_tokens, output_tokens, model_requests, tool_calls, final_ai_replies, provider_cost_yer, pricing_version, status, failure_code, correlation_id, started_at, completed_at`

func (r *AIUsageRepository) AppendRecord(ctx context.Context, input ports.AIUsageAppend) (ports.AIUsageRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.AIUsageRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(input.ID) == "" || strings.TrimSpace(input.BusinessID) == "" || strings.TrimSpace(input.SubscriptionID) == "" {
		return ports.AIUsageRecord{}, invalidRepositoryInput("ai_usage.append", "id, business_id, and subscription_id are required")
	}
	if strings.TrimSpace(input.Provider) == "" || strings.TrimSpace(input.Model) == "" {
		return ports.AIUsageRecord{}, invalidRepositoryInput("ai_usage.append", "provider and model are required")
	}
	if strings.TrimSpace(input.PricingVersion) == "" {
		return ports.AIUsageRecord{}, invalidRepositoryInput("ai_usage.append", "pricing_version is required")
	}
	if strings.TrimSpace(input.Status) == "" {
		return ports.AIUsageRecord{}, invalidRepositoryInput("ai_usage.append", "status is required")
	}
	if input.CompletedAt.Before(input.StartedAt) {
		return ports.AIUsageRecord{}, invalidRepositoryInput("ai_usage.append", "completed_at must be >= started_at")
	}
	if input.Now.IsZero() {
		input.Now = time.Now().UTC()
	}

	// P1-7 fix: wrap INSERT + aggregate UPSERT in a single transaction so
	// they commit atomically. Before this fix, the aggregate refresh was
	// best-effort — a failure left the per-record INSERT committed but the
	// snapshot stale. That violated Contract §28 ("append + aggregate
	// refresh happens atomically") and caused the platform admin's AI Usage
	// view to drift behind the actual records.
	//
	// The transaction is owned by Adapter.Within — it commits only if both
	// the INSERT and the UPSERT succeed; on any error, both roll back.
	var record ports.AIUsageRecord
	var refreshErr error
	if err := r.adapter.Within(ctx, func(txCtx context.Context) error {
		executor, err := r.adapter.Executor(txCtx)
		if err != nil {
			return err
		}
		err = executor.QueryRow(ctx,
			`INSERT INTO ai_usage_records (id, business_id, subscription_id, provider, model, input_tokens, cached_input_tokens, output_tokens, model_requests, tool_calls, final_ai_replies, provider_cost_yer, pricing_version, status, failure_code, correlation_id, started_at, completed_at)
                         VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NULLIF($15, ''), NULLIF($16, ''), $17, $18)
                         RETURNING `+aiUsageRecordSelectColumns,
			input.ID, input.BusinessID, input.SubscriptionID, input.Provider, input.Model,
			input.InputTokens, input.CachedInputTokens, input.OutputTokens,
			input.ModelRequests, input.ToolCalls, input.FinalAIReplies,
			input.ProviderCostYER, input.PricingVersion, input.Status,
			optionalStr(input.FailureCode), optionalStr(input.CorrelationID),
			input.StartedAt, input.CompletedAt,
		).Scan(
			&record.ID, &record.BusinessID, &record.SubscriptionID,
			&record.Provider, &record.Model,
			&record.InputTokens, &record.CachedInputTokens, &record.OutputTokens,
			&record.ModelRequests, &record.ToolCalls, &record.FinalAIReplies,
			&record.ProviderCostYER, &record.PricingVersion, &record.Status,
			&record.FailureCode, &record.CorrelationID,
			&record.StartedAt, &record.CompletedAt,
		)
		if err != nil {
			return classifyRepositoryWriteError("ai_usage.append", err)
		}
		// Refresh the aggregate snapshot inside the same transaction.
		// Per P1-7: the snapshot MUST reflect the just-inserted record
		// OR the INSERT rolls back. No silent stale-snapshot allowed.
		_, refreshErr = r.refreshAggregateInPlace(txCtx, executor, input.SubscriptionID, input.BusinessID, input.Now)
		if refreshErr != nil {
			return refreshErr
		}
		return nil
	}); err != nil {
		return ports.AIUsageRecord{}, err
	}
	return record, nil
}

// GetSubscriptionAIUsage reads the precomputed aggregate snapshot. If the
// snapshot row doesn't exist (e.g., subscription created but no AI activity
// yet), the repository returns a zero-aggregate with the subscription's
// baseline limits — these come from the subscriptions table itself.
//
// Per Contract §10-12: average_cost_per_reply, projected_remaining_cost,
// projected_total_cost, and budget_status are computed on-the-fly from
// the snapshot fields.
func (r *AIUsageRepository) GetSubscriptionAIUsage(ctx context.Context, subscriptionID string) (ports.SubscriptionAIUsageAggregate, error) {
	if r == nil || r.adapter == nil {
		return ports.SubscriptionAIUsageAggregate{}, ErrPoolClosed
	}
	if strings.TrimSpace(subscriptionID) == "" {
		return ports.SubscriptionAIUsageAggregate{}, invalidRepositoryInput("ai_usage.get", "subscription id is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SubscriptionAIUsageAggregate{}, err
	}
	// Read the snapshot row (LEFT JOIN with subscriptions to ensure we get
	// baseline limits even if no snapshot exists yet).
	var (
		agg                    ports.SubscriptionAIUsageAggregate
		internalBudgetOverride *int
		lastRecordedAt         *time.Time
	)
	err = executor.QueryRow(ctx,
		`SELECT s.id::text, s.business_id::text, s.ai_reply_limit, COALESCE(u.ai_replies_used, 0), COALESCE(u.input_tokens, 0), COALESCE(u.cached_input_tokens, 0), COALESCE(u.output_tokens, 0), COALESCE(u.model_requests, 0), COALESCE(u.tool_calls, 0), COALESCE(u.provider_cost_yer, 0), s.internal_ai_cost_budget_yer, s.cost_budget_override_yer, u.last_recorded_at
                 FROM subscriptions s
                 LEFT JOIN subscription_ai_usage u ON u.subscription_id = s.id
                 WHERE s.id = $1::uuid`,
		subscriptionID,
	).Scan(
		&agg.SubscriptionID, &agg.BusinessID, &agg.AIReplyLimit, &agg.AIRepliesUsed,
		&agg.InputTokens, &agg.CachedInputTokens, &agg.OutputTokens,
		&agg.ModelRequests, &agg.ToolCalls, &agg.ProviderCostYER,
		&agg.InternalCostBudgetYER, &internalBudgetOverride, &lastRecordedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.SubscriptionAIUsageAggregate{}, &RepositoryError{Operation: "ai_usage.get", Kind: RepositoryNotFound, Err: err}
		}
		return ports.SubscriptionAIUsageAggregate{}, &RepositoryError{Operation: "ai_usage.get", Kind: RepositoryInvalid, Err: err}
	}
	agg.CostBudgetOverrideYER = internalBudgetOverride
	agg.LastRecordedAt = lastRecordedAt
	// Apply override if present (Contract §24): the effective cost budget
	// is the override if set, else the plan's baseline.
	effectiveBudget := agg.InternalCostBudgetYER
	if internalBudgetOverride != nil {
		effectiveBudget = *internalBudgetOverride
	}
	// Compute derived fields per Contract §4, §10-12, §17.
	agg.AIRepliesRemaining = max0(agg.AIReplyLimit - agg.AIRepliesUsed)
	agg.CostRemainingYER = max0int(effectiveBudget - agg.ProviderCostYER)
	if agg.AIRepliesUsed > 0 {
		agg.AverageCostPerReplyYER = agg.ProviderCostYER / agg.AIRepliesUsed
	}
	agg.ProjectedRemainingCostYER = agg.AverageCostPerReplyYER * agg.AIRepliesRemaining
	agg.ProjectedTotalCostYER = agg.ProviderCostYER + agg.ProjectedRemainingCostYER
	agg.BudgetStatus = computeBudgetStatus(agg.ProviderCostYER, effectiveBudget)
	return agg, nil
}

// RefreshAggregate recomputes the per-subscription aggregate from the
// underlying ai_usage_records. Per Contract §28: this is called after every
// AppendRecord AND by the worker that batches AI usage reconciliation.
func (r *AIUsageRepository) RefreshAggregate(ctx context.Context, subscriptionID string, now time.Time) (ports.SubscriptionAIUsageAggregate, error) {
	if r == nil || r.adapter == nil {
		return ports.SubscriptionAIUsageAggregate{}, ErrPoolClosed
	}
	if strings.TrimSpace(subscriptionID) == "" {
		return ports.SubscriptionAIUsageAggregate{}, invalidRepositoryInput("ai_usage.refresh", "subscription id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SubscriptionAIUsageAggregate{}, err
	}
	// Need the business_id from the subscription for the upsert.
	var businessID string
	err = executor.QueryRow(ctx, `SELECT business_id::text FROM subscriptions WHERE id = $1::uuid`, subscriptionID).Scan(&businessID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.SubscriptionAIUsageAggregate{}, &RepositoryError{Operation: "ai_usage.refresh", Kind: RepositoryNotFound, Err: err}
		}
		return ports.SubscriptionAIUsageAggregate{}, &RepositoryError{Operation: "ai_usage.refresh", Kind: RepositoryInvalid, Err: err}
	}
	return r.refreshAggregateInPlace(ctx, executor, subscriptionID, businessID, now)
}

// refreshAggregateInPlace runs the SUM() aggregate + UPSERT in one go.
// Returns the computed aggregate (with derived fields).
func (r *AIUsageRepository) refreshAggregateInPlace(ctx context.Context, executor SQLExecutor, subscriptionID, businessID string, now time.Time) (ports.SubscriptionAIUsageAggregate, error) {
	// Upsert the aggregate snapshot.
	// The INSERT uses the latest record's completed_at as last_recorded_at
	// (NULL if no records yet).
	_, err := executor.Exec(ctx,
		`INSERT INTO subscription_ai_usage (subscription_id, business_id, ai_reply_limit, ai_replies_used, input_tokens, cached_input_tokens, output_tokens, model_requests, tool_calls, provider_cost_yer, last_recorded_at, updated_at)
                 SELECT
                   $1::uuid,
                   $2::uuid,
                   s.ai_reply_limit,
                   COALESCE(SUM(r.final_ai_replies), 0),
                   COALESCE(SUM(r.input_tokens), 0),
                   COALESCE(SUM(r.cached_input_tokens), 0),
                   COALESCE(SUM(r.output_tokens), 0),
                   COALESCE(SUM(r.model_requests), 0),
                   COALESCE(SUM(r.tool_calls), 0),
                   COALESCE(SUM(r.provider_cost_yer), 0),
                   (SELECT MAX(completed_at) FROM ai_usage_records WHERE subscription_id = $1::uuid),
                   $3
                 FROM subscriptions s
                 LEFT JOIN ai_usage_records r ON r.subscription_id = s.id
                 WHERE s.id = $1::uuid
                 GROUP BY s.ai_reply_limit
                 ON CONFLICT (subscription_id) DO UPDATE SET
                   ai_replies_used = EXCLUDED.ai_replies_used,
                   input_tokens = EXCLUDED.input_tokens,
                   cached_input_tokens = EXCLUDED.cached_input_tokens,
                   output_tokens = EXCLUDED.output_tokens,
                   model_requests = EXCLUDED.model_requests,
                   tool_calls = EXCLUDED.tool_calls,
                   provider_cost_yer = EXCLUDED.provider_cost_yer,
                   last_recorded_at = EXCLUDED.last_recorded_at,
                   updated_at = EXCLUDED.updated_at`,
		subscriptionID, businessID, now,
	)
	if err != nil {
		return ports.SubscriptionAIUsageAggregate{}, &RepositoryError{Operation: "ai_usage.refresh.upsert", Kind: RepositoryInvalid, Err: err}
	}
	// Now fetch the fresh aggregate via GetSubscriptionAIUsage.
	return r.GetSubscriptionAIUsage(ctx, subscriptionID)
}

func optionalStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

func max0int(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

// computeBudgetStatus implements Contract §17:
//
//	NORMAL   — consumed < 80% of budget
//	WARNING  — consumed >= 80% and < 100%
//	EXCEEDED — consumed >= 100%
//
// Per Contract §19: EXCEEDED triggers AI Cost Protection (no new Auto AI
// Execution starts if it would cause budget overrun — implemented in the
// AutoReply runtime guard, not here).
func computeBudgetStatus(consumedYER, budgetYER int) string {
	if budgetYER <= 0 {
		return "EXCEEDED"
	}
	if consumedYER <= 0 {
		return "NORMAL"
	}
	ratio := float64(consumedYER) / float64(budgetYER)
	if ratio >= 1.0 {
		return "EXCEEDED"
	}
	if ratio >= 0.8 {
		return "WARNING"
	}
	return "NORMAL"
}

var _ = fmt.Sprintf
var _ = math.MaxInt64

// ----------------------------------------------------------------------------
// Provider Pricing Versions — Contract §30
// ----------------------------------------------------------------------------

const aiProviderPricingSelectColumns = `id::text, provider, model, pricing_version, input_per_million_yer, cached_input_per_million_yer, output_per_million_yer, effective_from, effective_to, created_at`

// AIProviderPricingRepository implements ports.AIProviderPricingRepository.
type AIProviderPricingRepository struct {
	adapter *Adapter
}

func NewAIProviderPricingRepository(adapter *Adapter) *AIProviderPricingRepository {
	return &AIProviderPricingRepository{adapter: adapter}
}

func (r *AIProviderPricingRepository) CreatePricingVersion(ctx context.Context, create ports.AIProviderPricingCreate) (ports.AIProviderPricingVersion, error) {
	if r == nil || r.adapter == nil {
		return ports.AIProviderPricingVersion{}, ErrPoolClosed
	}
	if strings.TrimSpace(create.ID) == "" || strings.TrimSpace(create.Provider) == "" || strings.TrimSpace(create.Model) == "" || strings.TrimSpace(create.PricingVersion) == "" {
		return ports.AIProviderPricingVersion{}, invalidRepositoryInput("ai_pricing.create", "id, provider, model, and pricing_version are required")
	}
	if create.InputPerMillionYER <= 0 || create.OutputPerMillionYER <= 0 {
		return ports.AIProviderPricingVersion{}, invalidRepositoryInput("ai_pricing.create", "input_per_million_yer and output_per_million_yer must be positive")
	}
	if create.CachedInputPerMillionYER < 0 {
		return ports.AIProviderPricingVersion{}, invalidRepositoryInput("ai_pricing.create", "cached_input_per_million_yer must be non-negative")
	}
	if create.EffectiveFrom.IsZero() {
		create.EffectiveFrom = time.Now().UTC()
	}
	if create.Now.IsZero() {
		create.Now = time.Now().UTC()
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AIProviderPricingVersion{}, err
	}
	// Set effective_to = NULL on any previously-current version for the same
	// provider+model (i.e., supersede the previous pricing). Per Contract §30:
	// the new version's effective_from becomes "now".
	var record ports.AIProviderPricingVersion
	err = executor.QueryRow(ctx,
		`WITH superseded AS (
                   UPDATE ai_provider_pricing_versions
                     SET effective_to = $2
                     WHERE provider = $3 AND model = $4 AND effective_to IS NULL
                 )
                 INSERT INTO ai_provider_pricing_versions (id, provider, model, pricing_version, input_per_million_yer, cached_input_per_million_yer, output_per_million_yer, effective_from, created_at)
                 VALUES ($1::uuid, $3, $4, $5, $6, $7, $8, $2, $9)
                 RETURNING `+aiProviderPricingSelectColumns,
		create.ID, create.EffectiveFrom, create.Provider, create.Model, create.PricingVersion,
		create.InputPerMillionYER, create.CachedInputPerMillionYER, create.OutputPerMillionYER,
		create.Now,
	).Scan(
		&record.ID, &record.Provider, &record.Model, &record.PricingVersion,
		&record.InputPerMillionYER, &record.CachedInputPerMillionYER, &record.OutputPerMillionYER,
		&record.EffectiveFrom, &record.EffectiveTo, &record.CreatedAt,
	)
	if err != nil {
		return ports.AIProviderPricingVersion{}, classifyRepositoryWriteError("ai_pricing.create", err)
	}
	return record, nil
}

func (r *AIProviderPricingRepository) GetCurrentForProvider(ctx context.Context, provider, model string) (ports.AIProviderPricingVersion, error) {
	if r == nil || r.adapter == nil {
		return ports.AIProviderPricingVersion{}, ErrPoolClosed
	}
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(model) == "" {
		return ports.AIProviderPricingVersion{}, invalidRepositoryInput("ai_pricing.get_current", "provider and model are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AIProviderPricingVersion{}, err
	}
	var record ports.AIProviderPricingVersion
	err = executor.QueryRow(ctx,
		`SELECT `+aiProviderPricingSelectColumns+`
                 FROM ai_provider_pricing_versions
                 WHERE provider = $1 AND model = $2 AND effective_to IS NULL
                 ORDER BY effective_from DESC LIMIT 1`,
		provider, model,
	).Scan(
		&record.ID, &record.Provider, &record.Model, &record.PricingVersion,
		&record.InputPerMillionYER, &record.CachedInputPerMillionYER, &record.OutputPerMillionYER,
		&record.EffectiveFrom, &record.EffectiveTo, &record.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.AIProviderPricingVersion{}, &RepositoryError{Operation: "ai_pricing.get_current", Kind: RepositoryNotFound, Err: err}
		}
		return ports.AIProviderPricingVersion{}, &RepositoryError{Operation: "ai_pricing.get_current", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

func (r *AIProviderPricingRepository) GetByID(ctx context.Context, pricingID string) (ports.AIProviderPricingVersion, error) {
	if r == nil || r.adapter == nil {
		return ports.AIProviderPricingVersion{}, ErrPoolClosed
	}
	if strings.TrimSpace(pricingID) == "" {
		return ports.AIProviderPricingVersion{}, invalidRepositoryInput("ai_pricing.get", "pricing id is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AIProviderPricingVersion{}, err
	}
	var record ports.AIProviderPricingVersion
	err = executor.QueryRow(ctx,
		`SELECT `+aiProviderPricingSelectColumns+` FROM ai_provider_pricing_versions WHERE id = $1::uuid`,
		pricingID,
	).Scan(
		&record.ID, &record.Provider, &record.Model, &record.PricingVersion,
		&record.InputPerMillionYER, &record.CachedInputPerMillionYER, &record.OutputPerMillionYER,
		&record.EffectiveFrom, &record.EffectiveTo, &record.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.AIProviderPricingVersion{}, &RepositoryError{Operation: "ai_pricing.get", Kind: RepositoryNotFound, Err: err}
		}
		return ports.AIProviderPricingVersion{}, &RepositoryError{Operation: "ai_pricing.get", Kind: RepositoryInvalid, Err: err}
	}
	return record, nil
}

func (r *AIProviderPricingRepository) ListByProvider(ctx context.Context, provider string) ([]ports.AIProviderPricingVersion, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	if strings.TrimSpace(provider) == "" {
		return nil, invalidRepositoryInput("ai_pricing.list", "provider is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.Query(ctx,
		`SELECT `+aiProviderPricingSelectColumns+`
                 FROM ai_provider_pricing_versions
                 WHERE provider = $1
                 ORDER BY effective_from DESC`,
		provider,
	)
	if err != nil {
		return nil, &RepositoryError{Operation: "ai_pricing.list", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := []ports.AIProviderPricingVersion{}
	for rows.Next() {
		var record ports.AIProviderPricingVersion
		if err := rows.Scan(
			&record.ID, &record.Provider, &record.Model, &record.PricingVersion,
			&record.InputPerMillionYER, &record.CachedInputPerMillionYER, &record.OutputPerMillionYER,
			&record.EffectiveFrom, &record.EffectiveTo, &record.CreatedAt,
		); err != nil {
			return nil, &RepositoryError{Operation: "ai_pricing.list", Kind: RepositoryInvalid, Err: err}
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return nil, &RepositoryError{Operation: "ai_pricing.list", Kind: RepositoryInvalid, Err: err}
	}
	return items, nil
}

// compile-time assertion: AIUsageRepository + AIProviderPricingRepository
// satisfy their ports.
var _ ports.AIUsageRepository = (*AIUsageRepository)(nil)
var _ ports.AIProviderPricingRepository = (*AIProviderPricingRepository)(nil)

// uuid import marker — used indirectly via uuid.NewString in tests; here
// to keep the import alive in case future helpers need it.
var _ = uuid.Validate

// GetPlatformAIUsageOverview returns the platform-wide SUM across all
// ai_usage_records + the platform-wide budget totals (sum of all ACTIVE
// subscriptions' cost budgets). Per AIUsageTokenTelemetry.md §27.
//
// Per Item 6: the previous implementation only summed token totals +
// provider_cost_yer — it did NOT populate InternalCostBudgetYER or
// CostRemainingYER. The handler filled those fields with zero values
// just to satisfy the DTO schema, which was misleading (operators saw
// "active_budget=0" even when there were active subscriptions with
// non-zero budgets). The fix computes the real values from the
// subscriptions table.
func (r *AIUsageRepository) GetPlatformAIUsageOverview(ctx context.Context) (ports.SubscriptionAIUsageAggregate, error) {
	if r == nil || r.adapter == nil {
		return ports.SubscriptionAIUsageAggregate{}, ErrPoolClosed
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SubscriptionAIUsageAggregate{}, err
	}
	var agg ports.SubscriptionAIUsageAggregate
	// Per Item 6: the query now also computes the platform-wide budget
	// totals from the subscriptions table. The active budget for each
	// subscription is COALESCE(cost_budget_override_yer,
	// internal_ai_cost_budget_yer) — the override takes precedence
	// when set (per Contract §24). Remaining = MAX(budget - consumed, 0).
	err = executor.QueryRow(ctx,
		`SELECT
                   COALESCE(SUM(r.final_ai_replies), 0),
                   COALESCE(SUM(r.input_tokens), 0),
                   COALESCE(SUM(r.cached_input_tokens), 0),
                   COALESCE(SUM(r.output_tokens), 0),
                   COALESCE(SUM(r.model_requests), 0),
                   COALESCE(SUM(r.tool_calls), 0),
                   COALESCE(SUM(r.provider_cost_yer), 0),
                   COALESCE((SELECT SUM(COALESCE(s.cost_budget_override_yer, s.internal_ai_cost_budget_yer))
                             FROM subscriptions s WHERE s.status = 'ACTIVE'), 0)
                 FROM ai_usage_records r`,
	).Scan(
		&agg.AIRepliesUsed, &agg.InputTokens, &agg.CachedInputTokens,
		&agg.OutputTokens, &agg.ModelRequests, &agg.ToolCalls,
		&agg.ProviderCostYER, &agg.InternalCostBudgetYER,
	)
	if err != nil {
		return ports.SubscriptionAIUsageAggregate{}, &RepositoryError{Operation: "ai_usage.platform_overview", Kind: RepositoryInvalid, Err: err}
	}
	// Compute average cost per reply (§10).
	if agg.AIRepliesUsed > 0 {
		agg.AverageCostPerReplyYER = agg.ProviderCostYER / int(agg.AIRepliesUsed)
	}
	// Per Item 6: Consumed = actual provider cost (SUM of all records).
	// Remaining = MAX(Budget - Consumed, 0). Budget status per §17.
	agg.CostRemainingYER = max0int(agg.InternalCostBudgetYER - agg.ProviderCostYER)
	agg.BudgetStatus = computeBudgetStatus(agg.ProviderCostYER, agg.InternalCostBudgetYER)
	return agg, nil
}

// GetAIUsageByBusiness returns per-business aggregates grouped by business_id.
// Per AIUsageTokenTelemetry.md §34.
func (r *AIUsageRepository) GetAIUsageByBusiness(ctx context.Context, limit int) ([]ports.SubscriptionAIUsageAggregate, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.Query(ctx,
		`SELECT
                   business_id::text,
                   COALESCE(SUM(final_ai_replies), 0),
                   COALESCE(SUM(input_tokens), 0),
                   COALESCE(SUM(cached_input_tokens), 0),
                   COALESCE(SUM(output_tokens), 0),
                   COALESCE(SUM(model_requests), 0),
                   COALESCE(SUM(tool_calls), 0),
                   COALESCE(SUM(provider_cost_yer), 0)
                 FROM ai_usage_records
                 GROUP BY business_id
                 ORDER BY SUM(provider_cost_yer) DESC
                 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, &RepositoryError{Operation: "ai_usage.by_business", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := []ports.SubscriptionAIUsageAggregate{}
	for rows.Next() {
		var agg ports.SubscriptionAIUsageAggregate
		if err := rows.Scan(
			&agg.BusinessID, &agg.AIRepliesUsed, &agg.InputTokens,
			&agg.CachedInputTokens, &agg.OutputTokens,
			&agg.ModelRequests, &agg.ToolCalls, &agg.ProviderCostYER,
		); err != nil {
			return nil, &RepositoryError{Operation: "ai_usage.by_business", Kind: RepositoryInvalid, Err: err}
		}
		if agg.AIRepliesUsed > 0 {
			agg.AverageCostPerReplyYER = agg.ProviderCostYER / int(agg.AIRepliesUsed)
		}
		agg.BudgetStatus = "NORMAL"
		items = append(items, agg)
	}
	if err := rows.Err(); err != nil {
		return nil, &RepositoryError{Operation: "ai_usage.by_business", Kind: RepositoryInvalid, Err: err}
	}
	return items, nil
}

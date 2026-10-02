package ports

import (
	"context"
	"time"
)

// ----------------------------------------------------------------------------
// AI Usage Telemetry + Cost Budget — AIUsageTokenTelemetry.md §1-37
// ----------------------------------------------------------------------------
//
// Contract rules (CLOSED):
//
//   §1  — Two separate metrics:
//          - Merchant Entitlement = AI Replies (final replies only)
//          - Platform Consumption = Tokens + Model Requests + Tool Calls +
//            Provider Cost
//          NEVER merge these into one field.
//
//   §3  — A "Reply" counts ONLY when the system produces a Final AI
//          Response that becomes a billable response to the merchant/
//          customer. Tool calls, discovery calls, retries, validation,
//          prompt/context construction, and provider requests are NOT
//          independent AI Replies.
//
//   §4  — ai_replies_remaining = MAX(ai_reply_limit - ai_replies_used, 0).
//
//   §5  — Tokens do NOT have a "remaining quota" field. The contract
//          explicitly forbids "500,000 tokens included" / "1,000,000
//          tokens remaining" pricing — the merchant is billed on AI
//          Replies, not on tokens.
//
//   §6  — Each AI execution records an ai_usage_record with:
//          id, business_id, subscription_id, provider, model,
//          input_tokens, cached_input_tokens, output_tokens,
//          model_requests, tool_calls, final_ai_replies,
//          provider_cost_yer, pricing_version, status, failure_code,
//          correlation_id, started_at, completed_at.
//
//   §7  — Subscription ID is stored so historical subscriptions keep
//          their telemetry — even after a merchant moves to a new plan.
//
//   §8  — SubscriptionAIUsage aggregate (snapshot):
//          ai_reply_limit, ai_replies_used, ai_replies_remaining,
//          input_tokens, cached_input_tokens, output_tokens,
//          model_requests, tool_calls, provider_cost_yer.
//
//   §10 — average_cost_per_reply = provider_cost / ai_replies_used
//          (handle division-by-zero).
//
//   §11 — projected_remaining_cost = average_cost_per_reply ×
//          ai_replies_remaining.
//
//   §12 — projected_total_cost = provider_cost + projected_remaining_cost.
//
//   §17 — Budget status: NORMAL (<80%), WARNING (80-100%), EXCEEDED (≥100%).
//
//   §19 — EXCEEDED does NOT delete the subscription or data. It activates
//          "AI Cost Protection" — no new Auto AI Execution starts if it
//          would cause the budget to be exceeded. Human replies + merchant
//          dashboard + customer data + leads + orders continue.
//
//   §20 — Cost Protection rationale: 500 AI Replies can become economically
//          unviable if each reply takes many model requests. Replies alone
//          is insufficient — the Internal AI Cost Budget is the guardrail.
//
//   §21 — Cost Protection is NOT a round limit. The contract rejects
//          fixed "max 1 tool call" / "max 3 requests" caps. The AI is free
//          to use as many requests as needed — bounded by Subscription
//          Entitlement + Provider Limits + Platform Cost Protection +
//          Security.
//
//   §24 — Cost Budget Override is per-subscription (NOT plan version bump).
//          Old/new budget + reason + actor are recorded in platform_audit_events.
//
//   §30 — Provider Pricing is stored as ai_provider_pricing_versions; usage
//          records reference pricing_version so historical records stay
//          accurate when provider prices change.
//
//   §31 — Cost is computed from: input_tokens + cached_input_tokens +
//          output_tokens + provider pricing version — NOT from
//          (AI Replies × fixed cost).
//
//   §36 — Audit: ai.runtime.enabled / ai.runtime.disabled /
//          ai.cost_budget_changed / subscription.cost_budget_overridden /
//          plan.ai_reply_limit_changed / plan.internal_ai_cost_budget_changed /
//          provider.pricing_version_changed.
// ----------------------------------------------------------------------------

// AIUsageRecord is a single AI execution telemetry row.
//
// Per Contract §6: this is the per-execution record. It is append-only —
// no Update / Delete method exists on the repository (per §30, historical
// records must remain accurate even after pricing changes).
type AIUsageRecord struct {
	ID                string
	BusinessID        string
	SubscriptionID    string
	Provider          string
	Model             string
	InputTokens       int64
	CachedInputTokens int64
	OutputTokens      int64
	ModelRequests     int
	ToolCalls         int
	FinalAIReplies    int
	ProviderCostYER   int
	PricingVersion    string
	Status            string
	FailureCode       *string
	CorrelationID     *string
	StartedAt         time.Time
	CompletedAt       time.Time
}

// AIUsageAppend is the input to AppendRecord.
type AIUsageAppend struct {
	ID                string
	BusinessID        string
	SubscriptionID    string
	Provider          string
	Model             string
	InputTokens       int64
	CachedInputTokens int64
	OutputTokens      int64
	ModelRequests     int
	ToolCalls         int
	FinalAIReplies    int
	ProviderCostYER   int
	PricingVersion    string
	Status            string
	FailureCode       *string
	CorrelationID     *string
	StartedAt         time.Time
	CompletedAt       time.Time
	Now               time.Time
}

// SubscriptionAIUsageAggregate is the per-subscription snapshot (Contract §8).
//
// Per Contract §10-12: this is the view the platform admin sees:
//   - ai_replies_used / ai_replies_remaining
//   - tokens (input + cached + output)
//   - model_requests / tool_calls
//   - provider_cost_yer (actual accumulated)
//   - cost_budget_yer / cost_remaining_yer / average_cost_per_reply_yer
//   - projected_remaining_cost_yer / projected_total_cost_yer
//   - budget_status: NORMAL | WARNING | EXCEEDED
type SubscriptionAIUsageAggregate struct {
	SubscriptionID            string
	BusinessID                string
	AIReplyLimit              int
	AIRepliesUsed             int
	AIRepliesRemaining        int
	InputTokens               int64
	CachedInputTokens         int64
	OutputTokens              int64
	ModelRequests             int
	ToolCalls                 int
	ProviderCostYER           int
	InternalCostBudgetYER     int
	CostBudgetOverrideYER     *int
	CostRemainingYER          int
	AverageCostPerReplyYER    int
	ProjectedRemainingCostYER int
	ProjectedTotalCostYER     int
	BudgetStatus              string
	LastRecordedAt            *time.Time
}

// AIUsageRepository is the Platform-side AI usage telemetry port.
//
// Per Contract §30: usage records are append-only — no Update / Delete.
//
// Per Contract §28: AppendRecord + RefreshAggregate happens atomically —
// the worker that records an AI execution also refreshes the aggregate so
// the platform admin sees up-to-date numbers.
type AIUsageRepository interface {
	AppendRecord(ctx context.Context, append AIUsageAppend) (AIUsageRecord, error)
	GetSubscriptionAIUsage(ctx context.Context, subscriptionID string) (SubscriptionAIUsageAggregate, error)
	RefreshAggregate(ctx context.Context, subscriptionID string, now time.Time) (SubscriptionAIUsageAggregate, error)
	// GetPlatformAIUsageOverview returns the platform-wide aggregate across
	// ALL ai_usage_records. Per AIUsageTokenTelemetry.md §27: Total AI Replies,
	// Total Input/Cached/Output Tokens, Total Model Requests, Total Tool
	// Calls, Total Provider Cost, Average Cost / Reply.
	GetPlatformAIUsageOverview(ctx context.Context) (SubscriptionAIUsageAggregate, error)
	// GetAIUsageByBusiness returns per-business aggregates. Per §34.
	GetAIUsageByBusiness(ctx context.Context, limit int) ([]SubscriptionAIUsageAggregate, error)
}

// ----------------------------------------------------------------------------
// Provider Pricing Versions — Contract §30 (immutable historical pricing)
// ----------------------------------------------------------------------------

// AIProviderPricingVersion is a single provider's pricing snapshot.
// The pricing_version field is what AIUsageRecord.pricing_version references
// so historical records stay accurate when provider prices change.
type AIProviderPricingVersion struct {
	ID                       string
	Provider                 string
	Model                    string
	PricingVersion           string
	InputPerMillionYER       int
	CachedInputPerMillionYER int
	OutputPerMillionYER      int
	EffectiveFrom            time.Time
	EffectiveTo              *time.Time
	CreatedAt                time.Time
}

// AIProviderPricingCreate is the input to CreatePricingVersion.
type AIProviderPricingCreate struct {
	ID                       string
	Provider                 string
	Model                    string
	PricingVersion           string
	InputPerMillionYER       int
	CachedInputPerMillionYER int
	OutputPerMillionYER      int
	EffectiveFrom            time.Time
	Now                      time.Time
}

// AIProviderPricingRepository is the port for managing immutable provider
// pricing versions. Per Contract §30, versions are append-only — once
// created, a version is never edited. A new version with effective_from =
// now supersedes the previous one (whose effective_to is set).
type AIProviderPricingRepository interface {
	CreatePricingVersion(ctx context.Context, create AIProviderPricingCreate) (AIProviderPricingVersion, error)
	GetCurrentForProvider(ctx context.Context, provider, model string) (AIProviderPricingVersion, error)
	GetByID(ctx context.Context, pricingID string) (AIProviderPricingVersion, error)
	ListByProvider(ctx context.Context, provider string) ([]AIProviderPricingVersion, error)
}

package ports

import (
	"context"
	"time"
)

// ----------------------------------------------------------------------------
// Subscription Management — Platform Administration Contract V1 §20-36
// ----------------------------------------------------------------------------
//
// Contract rules (CLOSED):
//
//   §21 — Subscription States: PENDING → ACTIVE → EXPIRED
//                              PENDING → CANCELLED
//                              ACTIVE  → CANCELLED
//                              ACTIVE  → EXPIRED (auto on period_end)
//
//   §22 — No reuse of old subscriptions. Renewal creates a NEW period with a
//          NEW subscription_id. The historical subscription row stays immutable.
//
//   §23 — Billing interval is MONTH (calendar month).
//
//   §24 — No proration. Plan changes happen on renewal, never mid-period.
//
//   §25 — Manual payment model. Platform Admin records the payment after
//          verification OUTSIDE Mujeeb.
//
//   §27 — Payment Record is APPEND ONLY. No Update / Delete. Corrections go
//          through a new documented adjustment, not silent edits.
//
//   §28 — Activation flow: CreateSubscription → PENDING; RecordPayment → ACTIVE.
//          No subscription becomes ACTIVE without a recorded payment.
//
//   §29 — Expiry does NOT delete business data. Entitlements stop; the row stays.
//
//   §30 — Expired subscription preserves account + dashboard access; the user
//          can renew. Entitlements are gated by subscription.status.
//
//   §34 — Catalog limit downgrade: if the new plan's catalog limit is lower
//          than the merchant's current AI-active catalog count, the new
//          subscription does NOT become ACTIVE until the merchant resolves
//          the overrun. No silent data deletion.
//
//   §35 — Same rule for channel limit. No auto-disconnect.
//
//   §36 — APIs (registered as platformCreateSubscription / platformListSubscriptions /
//          platformGetSubscription / platformRecordPayment / platformCancelSubscription
//          in contract/platform_operations.go).
// ----------------------------------------------------------------------------

// SubscriptionRecord is a single subscription period row.
//
// The ID is server-generated UUID. Plan-related fields are SNAPSHOTED at
// creation time per Contract §17 — the Plan may be retired later, but the
// subscription retains the limits it was created with. The plan_id is kept
// as a reference for audit only.
type SubscriptionRecord struct {
	ID                       string
	BusinessID               string
	PlanID                   string
	PlanCode                 string
	PlanVersion              int
	PeriodStart              time.Time
	PeriodEnd                time.Time
	Status                   string
	AIReplyLimit             int
	AICatalogLimit           int
	ChannelLimit             int
	InternalAICostBudgetYER  int
	CostBudgetOverrideYER    *int
	CostBudgetOverrideReason *string
	CostBudgetOverrideBy     *string
	CostBudgetOverrideAt     *time.Time
	CancelledAt              *time.Time
	CancelledReason          *string
	CancelledBy              *string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// SubscriptionCreate is the input to Create. All plan-derived fields are
// snapshot from the plan at creation time — they become immutable for the
// lifetime of this subscription row.
type SubscriptionCreate struct {
	ID                      string
	BusinessID              string
	PlanID                  string
	PeriodStart             time.Time
	PeriodEnd               time.Time
	AIReplyLimit            int
	AICatalogLimit          int
	ChannelLimit            int
	InternalAICostBudgetYER int
	Now                     time.Time
}

// SubscriptionListFilter is the query input to List.
// Per Contract §13 / §36: list supports pagination + status filter.
type SubscriptionListFilter struct {
	BusinessID string
	Status     string
	Limit      int
	Cursor     string
}

type SubscriptionPage struct {
	Items      []SubscriptionRecord
	NextCursor string
	HasMore    bool
}

// SubscriptionRepository is the Platform-side subscription port.
//
// Per Contract §22: there is NO Update method. Renewal creates a new row.
// Per Contract §27: payment records are appended, never edited.
//
// Methods:
//   - Create        → inserts a new PENDING subscription.
//   - GetByID       → fetches a single subscription by ID (for audit + admin views).
//   - List          → paginated list with business_id + status filters.
//   - Activate      → transitions PENDING → ACTIVE. Returns Conflict if not PENDING.
//   - Cancel        → transitions PENDING|ACTIVE → CANCELLED. Returns Conflict if already terminal.
//   - MarkExpired   → transitions ACTIVE → EXPIRED. Used by the expiry worker.
//     Returns Conflict if not ACTIVE (idempotent on EXPIRED).
//   - ApplyCostBudgetOverride → updates the per-subscription cost budget override.
//     Per Contract §24 (AIUsageTokenTelemetry) — does NOT change plan version.
type SubscriptionRepository interface {
	Create(ctx context.Context, create SubscriptionCreate) (SubscriptionRecord, error)
	GetByID(ctx context.Context, subscriptionID string) (SubscriptionRecord, error)
	List(ctx context.Context, filter SubscriptionListFilter) (SubscriptionPage, error)
	Activate(ctx context.Context, subscriptionID string, now time.Time) (SubscriptionRecord, error)
	Cancel(ctx context.Context, subscriptionID, reason, cancelledBy string, now time.Time) (SubscriptionRecord, error)
	MarkExpired(ctx context.Context, subscriptionID string, now time.Time) (SubscriptionRecord, error)
	ApplyCostBudgetOverride(ctx context.Context, subscriptionID string, newBudgetYER int, reason, overrideBy string, now time.Time) (SubscriptionRecord, error)
	// CheckEntitlements verifies that the business's current catalog item count
	// and active channel count do not exceed the subscription's plan limits.
	// Per Contract §34-35: if the current state is higher than the new plan's
	// limits, the subscription does NOT become ACTIVE until the merchant resolves
	// the overrun. No silent data deletion.
	//
	// Returns nil if within limits. Returns Conflict error if catalog or channel
	// count exceeds the subscription's snapshotted limits.
	CheckEntitlements(ctx context.Context, businessID string, catalogLimit, channelLimit int) error
}

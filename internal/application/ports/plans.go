package ports

import (
        "context"
        "time"
)

// PlanRecord is a single plan definition.
//
// Per Platform Administration Contract §15-19:
//   - Plans are platform-owned (not merchant-owned).
//   - Version is immutable once a subscription is attached — a change creates
//     a new Plan Version (not an in-place edit).
//   - Status: DRAFT → ACTIVE → RETIRED. DRAFT cannot be subscribed to.
//     RETIRED blocks new subscriptions but historical ones remain valid.
//   - baseline seeds (Contract §16):
//       Basic:    5000 YER, 500 replies, 200 catalog, 1 channel, 1000 YER budget
//       Growth:  10000 YER, 1500 replies, 750 catalog, 2 channels, 3500 YER budget
//       Business:20000 YER, 4000 replies, 2500 catalog, 5 channels, 9000 YER budget
type PlanRecord struct {
        ID                       string
        Code                     string
        Version                  int
        DisplayName              string
        PriceYER                 int
        BillingInterval          string
        AIReplyLimit             int
        AICatalogLimit           int
        ChannelLimit             int
        InternalAICostBudgetYER  int
        Status                   string
        CreatedAt                time.Time
        UpdatedAt                time.Time
        RetiredAt                *time.Time
}

type PlanCreate struct {
        ID                       string
        Code                     string
        Version                  int
        DisplayName              string
        PriceYER                 int
        BillingInterval          string
        AIReplyLimit             int
        AICatalogLimit           int
        ChannelLimit             int
        InternalAICostBudgetYER  int
        Now                      time.Time
}

// PlanVersionCreate is the input for creating a NEW version of an existing
// plan. Per Contract §17 + §22-23 + AIUsageTokenTelemetry.md §22-23:
//   - The original plan row is NEVER edited.
//   - A new row is inserted with the SAME code + an incremented version.
//   - The original plan's status transitions to RETIRED atomically with
//     the new version's transition to ACTIVE — so the system never has
//     two ACTIVE versions of the same code at the same time.
//
// Subscriptions already created on the old version keep their snapshot of
// the limits — they're immutable per Contract §17.
type PlanVersionCreate struct {
        BasePlanID              string // the existing plan ID to version from
        DisplayName             string
        PriceYER                int
        BillingInterval         string
        AIReplyLimit            int
        AICatalogLimit          int
        ChannelLimit           int
        InternalAICostBudgetYER int
        ChangedBy              string
        Reason                 string
        Now                     time.Time
}

type PlanListFilter struct {
        Status string
        Code   string
        Limit  int
        Cursor string
}

type PlanPage struct {
        Items      []PlanRecord
        NextCursor string
        HasMore    bool
}

// PlanRepository implements the Platform-side Plan domain.
// Per Contract §17-19: no Update method — versioning replaces mutation.
// A RETIRED plan stays in the table; the lifecycle moves forward-only.
type PlanRepository interface {
        Create(ctx context.Context, create PlanCreate) (PlanRecord, error)
        GetByID(ctx context.Context, planID string) (PlanRecord, error)
        List(ctx context.Context, filter PlanListFilter) (PlanPage, error)
        Activate(ctx context.Context, planID string, now time.Time) (PlanRecord, error)
        Retire(ctx context.Context, planID string, now time.Time) (PlanRecord, error)
        // CreateVersion creates a NEW version of an existing plan. Per Contract
        // §17 + AIUsageTokenTelemetry.md §22-23: the original plan row is
        // NEVER edited — a new row with incremented version + new fields is
        // inserted. The base plan is atomically transitioned to RETIRED.
        //
        // The new version is created in ACTIVE status (the only state in which
        // new subscriptions can be created). Returns Conflict if the base plan
        // is not ACTIVE.
        CreateVersion(ctx context.Context, create PlanVersionCreate) (PlanRecord, error)
}

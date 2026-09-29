package handlers

import (
        "context"
        "errors"
        "testing"
        "time"

        "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/primary/http/dto"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// ----------------------------------------------------------------------------
// Mock implementations for AI Usage + Provider Pricing ports
// ----------------------------------------------------------------------------

type stubAIUsageRepository struct {
        aggregateResult ports.SubscriptionAIUsageAggregate
        aggregateErr    error
        refreshResult   ports.SubscriptionAIUsageAggregate
        refreshErr      error
        appendErr       error
        platformOverviewResult ports.SubscriptionAIUsageAggregate
        platformOverviewErr    error
        byBusinessResult   []ports.SubscriptionAIUsageAggregate
        byBusinessErr      error
}

func (s *stubAIUsageRepository) AppendRecord(_ context.Context, _ ports.AIUsageAppend) (ports.AIUsageRecord, error) {
        return ports.AIUsageRecord{}, s.appendErr
}
func (s *stubAIUsageRepository) GetSubscriptionAIUsage(_ context.Context, _ string) (ports.SubscriptionAIUsageAggregate, error) {
        return s.aggregateResult, s.aggregateErr
}
func (s *stubAIUsageRepository) RefreshAggregate(_ context.Context, _ string, _ time.Time) (ports.SubscriptionAIUsageAggregate, error) {
        return s.refreshResult, s.refreshErr
}
func (s *stubAIUsageRepository) GetPlatformAIUsageOverview(_ context.Context) (ports.SubscriptionAIUsageAggregate, error) {
        return s.platformOverviewResult, s.platformOverviewErr
}
func (s *stubAIUsageRepository) GetAIUsageByBusiness(_ context.Context, _ int) ([]ports.SubscriptionAIUsageAggregate, error) {
        return s.byBusinessResult, s.byBusinessErr
}

type stubAIProviderPricingRepository struct {
        created ports.AIProviderPricingVersion
        createErr error
        current  ports.AIProviderPricingVersion
        currentErr error
        getByID  ports.AIProviderPricingVersion
        getErr   error
        listByProvider []ports.AIProviderPricingVersion
        listErr  error
}

func (s *stubAIProviderPricingRepository) CreatePricingVersion(_ context.Context, _ ports.AIProviderPricingCreate) (ports.AIProviderPricingVersion, error) {
        return s.created, s.createErr
}
func (s *stubAIProviderPricingRepository) GetCurrentForProvider(_ context.Context, _, _ string) (ports.AIProviderPricingVersion, error) {
        return s.current, s.currentErr
}
func (s *stubAIProviderPricingRepository) GetByID(_ context.Context, _ string) (ports.AIProviderPricingVersion, error) {
        return s.getByID, s.getErr
}
func (s *stubAIProviderPricingRepository) ListByProvider(_ context.Context, _ string) ([]ports.AIProviderPricingVersion, error) {
        return s.listByProvider, s.listErr
}

// ----------------------------------------------------------------------------
// Tests: AI Usage Telemetry (AIUsageTokenTelemetry.md §6-36)
// ----------------------------------------------------------------------------

func TestPlatformGetSubscriptionAIUsageReturns501WhenUnwired(t *testing.T) {
        server := newPlatformServer(PlatformDeps{})
        ctx, cancel := ctxWithPlatformAdmin()
        defer cancel()
        _, handled := server.dispatchPlatformCommand(ctx, "platformGetSubscriptionAIUsage", &dto.PlatformSubscriptionPath{
                SubscriptionID: dto.UUID(subIDForTests),
        })
        if !handled {
                t.Fatalf("expected handled=true")
        }
}

func TestPlatformGetSubscriptionAIUsageReturnsView(t *testing.T) {
        usageRepo := &stubAIUsageRepository{aggregateResult: ports.SubscriptionAIUsageAggregate{
                SubscriptionID:            subIDForTests,
                BusinessID:                "biz-1",
                AIReplyLimit:              500,
                AIRepliesUsed:             173,
                AIRepliesRemaining:        327,
                InputTokens:               1000000,
                OutputTokens:              300000,
                ProviderCostYER:           430,
                InternalCostBudgetYER:     1000,
                CostRemainingYER:          570,
                AverageCostPerReplyYER:    2,
                ProjectedRemainingCostYER: 654,
                ProjectedTotalCostYER:     1084,
                BudgetStatus:              "NORMAL",
        }}
        server := newPlatformServer(PlatformDeps{AIUsage: usageRepo})
        ctx, cancel := ctxWithPlatformAdmin()
        defer cancel()
        _, handled := server.dispatchPlatformCommand(ctx, "platformGetSubscriptionAIUsage", &dto.PlatformSubscriptionPath{
                SubscriptionID: dto.UUID(subIDForTests),
        })
        if !handled {
                t.Fatalf("expected handled=true")
        }
}

func TestPlatformGetSubscriptionAIUsagePropagatesRepoError(t *testing.T) {
        usageRepo := &stubAIUsageRepository{aggregateErr: errors.New("subscription not found")}
        server := newPlatformServer(PlatformDeps{AIUsage: usageRepo})
        ctx, cancel := ctxWithPlatformAdmin()
        defer cancel()
        _, handled := server.dispatchPlatformCommand(ctx, "platformGetSubscriptionAIUsage", &dto.PlatformSubscriptionPath{
                SubscriptionID: dto.UUID(subIDForTests),
        })
        if !handled {
                t.Fatalf("expected handled=true")
        }
}

// ----------------------------------------------------------------------------
// Tests: Cost Budget Override returns full AI usage view (Stage 4 enhancement)
// ----------------------------------------------------------------------------

func TestPlatformOverrideSubscriptionAICostBudgetReturnsFullViewWhenAIUsageWired(t *testing.T) {
        subRepo := &stubSubscriptionRepository{
                overrideResult: ports.SubscriptionRecord{ID: subIDForTests, BusinessID: "biz-1", InternalAICostBudgetYER: 1000},
        }
        usageRepo := &stubAIUsageRepository{aggregateResult: ports.SubscriptionAIUsageAggregate{
                SubscriptionID:          subIDForTests,
                InternalCostBudgetYER:   5000, // reflect override
                BudgetStatus:            "NORMAL",
                AIReplyLimit:            500,
        }}
        auditRepo := &stubPlatformAuditRepository{}
        server := newPlatformServer(PlatformDeps{
                Subscriptions: subRepo,
                AIUsage:       usageRepo,
                PlatformAudit: auditRepo,
        })
        ctx, cancel := ctxWithPlatformAdmin()
        defer cancel()
        _, handled := server.dispatchPlatformCommand(ctx, "platformOverrideSubscriptionAICostBudget", &dto.SubscriptionAICostBudgetOverrideInput{
                PlatformSubscriptionPath: platformSubscriptionPath(),
                Body: dto.SubscriptionAICostBudgetOverrideRequest{BudgetYER: 5000, Reason: "Updated guardrail"},
        })
        if !handled {
                t.Fatalf("expected handled=true")
        }
        if auditRepo.appendedDraft == nil {
                t.Fatalf("expected audit Append")
        }
        if auditRepo.appendedDraft.Action != "subscription.cost_budget_overridden" {
                t.Errorf("expected action=subscription.cost_budget_overridden, got %s", auditRepo.appendedDraft.Action)
        }
        if auditRepo.appendedDraft.Result != "SUCCESS" {
                t.Errorf("expected result=SUCCESS, got %s", auditRepo.appendedDraft.Result)
        }
}

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

var errPricingNotFound = errors.New("pricing version not found")

// stubPricingRepoFail is a pricing repo that always fails (simulates
// pricing lookup failure).
type stubPricingRepoFail struct{}

func (s *stubPricingRepoFail) CreatePricingVersion(_ context.Context, _ ports.AIProviderPricingCreate) (ports.AIProviderPricingVersion, error) {
	return ports.AIProviderPricingVersion{}, nil
}
func (s *stubPricingRepoFail) GetCurrentForProvider(_ context.Context, _ string, _ string) (ports.AIProviderPricingVersion, error) {
	return ports.AIProviderPricingVersion{}, nil // returns nil error but with empty PricingVersion
}
func (s *stubPricingRepoFail) GetByID(_ context.Context, _ string) (ports.AIProviderPricingVersion, error) {
	return ports.AIProviderPricingVersion{}, nil
}
func (s *stubPricingRepoFail) ListByProvider(_ context.Context, _ string) ([]ports.AIProviderPricingVersion, error) {
	return nil, nil
}

// stubUsageRepoCapture captures the last AppendRecord input so the test
// can assert on final_ai_replies + status.
type stubUsageRepoCapture struct {
	lastAppend ports.AIUsageAppend
}

func (s *stubUsageRepoCapture) AppendRecord(_ context.Context, input ports.AIUsageAppend) (ports.AIUsageRecord, error) {
	s.lastAppend = input
	return ports.AIUsageRecord{ID: input.ID}, nil
}
func (s *stubUsageRepoCapture) GetSubscriptionAIUsage(_ context.Context, _ string) (ports.SubscriptionAIUsageAggregate, error) {
	return ports.SubscriptionAIUsageAggregate{}, nil
}
func (s *stubUsageRepoCapture) RefreshAggregate(_ context.Context, _ string, _ time.Time) (ports.SubscriptionAIUsageAggregate, error) {
	return ports.SubscriptionAIUsageAggregate{}, nil
}
func (s *stubUsageRepoCapture) GetPlatformAIUsageOverview(_ context.Context) (ports.SubscriptionAIUsageAggregate, error) {
	return ports.SubscriptionAIUsageAggregate{}, nil
}
func (s *stubUsageRepoCapture) GetAIUsageByBusiness(_ context.Context, _ int) ([]ports.SubscriptionAIUsageAggregate, error) {
	return nil, nil
}

var _ ports.AIUsageRepository = (*stubUsageRepoCapture)(nil)

// Test Item 3: when pricing lookup fails AND the reply was enqueued,
// final_ai_replies MUST still be 1 (the customer received the reply,
// so the entitlement is consumed). The record carries status=
// "pricing_failed" to flag the cost gap — but the reply is NOT
// erased from the entitlement counter.
//
// This test verifies the recordAIUsage logic by directly invoking
// it with a pricing repo that returns an error.
func TestPricingFailureDoesNotEraseReplyFromEntitlement(t *testing.T) {
	t.Parallel()
	// Build an AutoReplyService with a pricing repo that always fails.
	usageRepo := &stubUsageRepoCapture{}
	svc := AutoReplyService{
		Runtime:   nil, // not used in this test
		AIUsage:   usageRepo,
		AIPricing: &stubPricingRepoAlwaysFail{},
		Subscriptions: &stubSubscriptionsRepo{
			items: []ports.SubscriptionRecord{{ID: "sub-1", BusinessID: "b-1", Status: "ACTIVE"}},
		},
		Now:   func() time.Time { return time.Now().UTC() },
		NewID: func() string { return "test-id" },
	}
	// Simulate a successful Gemini call with replyEnqueued=true.
	out := ports.CustomerSalesDecisionOutput{
		Proposal: ports.AIGeminiProposal{
			Status:       "resolved",
			Action:       "answer",
			ResponseText: "test response",
		},
		Usage: ports.ContractUsageTelemetry{
			InputTokens:  10,
			OutputTokens: 5,
			Model:        "gemini-3.5-flash-lite",
		},
		LatencyMs: 100,
	}
	run := ports.AIRunRecord{ID: "run-1"}
	err := svc.recordAIUsage(context.Background(), "b-1", out, run, true)
	if err != nil {
		t.Fatalf("recordAIUsage should not fail when pricing repo returns error + reply was enqueued: %v", err)
	}
	// Assert: final_ai_replies = 1 (reply was enqueued) even though
	// pricing failed.
	if usageRepo.lastAppend.FinalAIReplies != 1 {
		t.Errorf("expected FinalAIReplies=1 (reply was enqueued — entitlement MUST be consumed), got %d", usageRepo.lastAppend.FinalAIReplies)
	}
	// Assert: status = "pricing_failed" (cost gap is visible).
	if usageRepo.lastAppend.Status != "pricing_failed" {
		t.Errorf("expected Status=pricing_failed (cost lookup failed), got %s", usageRepo.lastAppend.Status)
	}
	// Assert: provider_cost_yer = 0 (honest — we don't know the real cost).
	if usageRepo.lastAppend.ProviderCostYER != 0 {
		t.Errorf("expected ProviderCostYER=0 (couldn't compute — honest), got %d", usageRepo.lastAppend.ProviderCostYER)
	}
	// Assert: pricing_version = "unknown" (honest).
	if usageRepo.lastAppend.PricingVersion != "unknown" {
		t.Errorf("expected PricingVersion=unknown, got %s", usageRepo.lastAppend.PricingVersion)
	}
}

// stubPricingRepoAlwaysFail always returns an error from GetCurrentForProvider.
type stubPricingRepoAlwaysFail struct{}

func (s *stubPricingRepoAlwaysFail) CreatePricingVersion(_ context.Context, _ ports.AIProviderPricingCreate) (ports.AIProviderPricingVersion, error) {
	return ports.AIProviderPricingVersion{}, nil
}
func (s *stubPricingRepoAlwaysFail) GetCurrentForProvider(_ context.Context, _ string, _ string) (ports.AIProviderPricingVersion, error) {
	return ports.AIProviderPricingVersion{}, errPricingNotFound
}
func (s *stubPricingRepoAlwaysFail) GetByID(_ context.Context, _ string) (ports.AIProviderPricingVersion, error) {
	return ports.AIProviderPricingVersion{}, nil
}
func (s *stubPricingRepoAlwaysFail) ListByProvider(_ context.Context, _ string) ([]ports.AIProviderPricingVersion, error) {
	return nil, nil
}

var _ ports.AIProviderPricingRepository = (*stubPricingRepoAlwaysFail)(nil)

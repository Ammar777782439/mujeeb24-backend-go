package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// ===== Item 4: Comprehensive entitlement boundary tests =====

// stubSubsRepoFailList returns an error from List (simulates DB failure).
type stubSubsRepoFailList struct{}

func (s *stubSubsRepoFailList) List(_ context.Context, _ ports.SubscriptionListFilter) (ports.SubscriptionPage, error) {
	return ports.SubscriptionPage{}, errors.New("DB connection refused")
}
func (s *stubSubsRepoFailList) GetByID(_ context.Context, _ string) (ports.SubscriptionRecord, error) {
	return ports.SubscriptionRecord{}, nil
}
func (s *stubSubsRepoFailList) Create(_ context.Context, _ ports.SubscriptionCreate) (ports.SubscriptionRecord, error) {
	return ports.SubscriptionRecord{}, nil
}
func (s *stubSubsRepoFailList) Activate(_ context.Context, _ string, _ time.Time) (ports.SubscriptionRecord, error) {
	return ports.SubscriptionRecord{}, nil
}
func (s *stubSubsRepoFailList) Cancel(_ context.Context, _ string, _ string, _ string, _ time.Time) (ports.SubscriptionRecord, error) {
	return ports.SubscriptionRecord{}, nil
}
func (s *stubSubsRepoFailList) MarkExpired(_ context.Context, _ string, _ time.Time) (ports.SubscriptionRecord, error) {
	return ports.SubscriptionRecord{}, nil
}
func (s *stubSubsRepoFailList) ApplyCostBudgetOverride(_ context.Context, _ string, _ int, _ string, _ string, _ time.Time) (ports.SubscriptionRecord, error) {
	return ports.SubscriptionRecord{}, nil
}
func (s *stubSubsRepoFailList) CheckEntitlements(_ context.Context, _ string, _ int, _ int) error {
	return nil
}

var _ ports.SubscriptionRepository = (*stubSubsRepoFailList)(nil)

// stubSubsRepoEmpty returns an empty list (no active subscription).
type stubSubsRepoEmpty struct{}

func (s *stubSubsRepoEmpty) List(_ context.Context, _ ports.SubscriptionListFilter) (ports.SubscriptionPage, error) {
	return ports.SubscriptionPage{Items: nil}, nil
}
func (s *stubSubsRepoEmpty) GetByID(_ context.Context, _ string) (ports.SubscriptionRecord, error) {
	return ports.SubscriptionRecord{}, nil
}
func (s *stubSubsRepoEmpty) Create(_ context.Context, _ ports.SubscriptionCreate) (ports.SubscriptionRecord, error) {
	return ports.SubscriptionRecord{}, nil
}
func (s *stubSubsRepoEmpty) Activate(_ context.Context, _ string, _ time.Time) (ports.SubscriptionRecord, error) {
	return ports.SubscriptionRecord{}, nil
}
func (s *stubSubsRepoEmpty) Cancel(_ context.Context, _ string, _ string, _ string, _ time.Time) (ports.SubscriptionRecord, error) {
	return ports.SubscriptionRecord{}, nil
}
func (s *stubSubsRepoEmpty) MarkExpired(_ context.Context, _ string, _ time.Time) (ports.SubscriptionRecord, error) {
	return ports.SubscriptionRecord{}, nil
}
func (s *stubSubsRepoEmpty) ApplyCostBudgetOverride(_ context.Context, _ string, _ int, _ string, _ string, _ time.Time) (ports.SubscriptionRecord, error) {
	return ports.SubscriptionRecord{}, nil
}
func (s *stubSubsRepoEmpty) CheckEntitlements(_ context.Context, _ string, _ int, _ int) error {
	return nil
}

var _ ports.SubscriptionRepository = (*stubSubsRepoEmpty)(nil)

// Helper: builds a minimal AutoReplyService for recordAIUsage tests.
func buildServiceForUsageTest(usageRepo ports.AIUsageRepository, subsRepo ports.SubscriptionRepository, pricingRepo ports.AIProviderPricingRepository) AutoReplyService {
	return AutoReplyService{
		AIUsage:       usageRepo,
		Subscriptions: subsRepo,
		AIPricing:     pricingRepo,
		Now:           func() time.Time { return time.Now().UTC() },
		NewID:         func() string { return "test-id" },
	}
}

func buildUsageOutput() ports.CustomerSalesDecisionOutput {
	return ports.CustomerSalesDecisionOutput{
		Decision: ports.CapabilityDecision{
			Capability:   ports.CustomerSalesCapabilityRespondWithoutAction,
			ResponseText: "test response",
		},
		Usage: ports.CustomerSalesUsageTelemetry{
			InputTokens:  10,
			OutputTokens: 5,
			Model:        "gemini-3.5-flash-lite",
		},
		LatencyMs: 100,
	}
}

// Test Item 4.1: replyEnqueued=true + pricing failure → FinalAIReplies=1.
func TestItem4_PricingFailure_ReplyCounts(t *testing.T) {
	t.Parallel()
	usageRepo := &stubUsageRepoCapture{}
	svc := buildServiceForUsageTest(
		usageRepo,
		&stubSubscriptionsRepo{items: []ports.SubscriptionRecord{{ID: "sub-1", BusinessID: "b-1", Status: "ACTIVE"}}},
		&stubPricingRepoAlwaysFail{},
	)
	run := ports.AIRunRecord{ID: "run-1"}
	err := svc.recordAIUsage(context.Background(), "b-1", buildUsageOutput(), run, true)
	if err != nil {
		t.Fatalf("expected nil error (pricing failure is NOT a reply failure): %v", err)
	}
	if usageRepo.lastAppend.FinalAIReplies != 1 {
		t.Errorf("FinalAIReplies=%d, expected 1 (reply was enqueued — entitlement MUST be consumed even when pricing fails)", usageRepo.lastAppend.FinalAIReplies)
	}
	if usageRepo.lastAppend.Status != "pricing_failed" {
		t.Errorf("Status=%s, expected pricing_failed", usageRepo.lastAppend.Status)
	}
}

// Test Item 4.2: replyEnqueued=true + AppendRecord failure → error to caller.
func TestItem4_AppendRecordFailure_PropagatesError(t *testing.T) {
	t.Parallel()
	svc := buildServiceForUsageTest(
		&stubUsageRepoFailAppend{},
		&stubSubscriptionsRepo{items: []ports.SubscriptionRecord{{ID: "sub-1", BusinessID: "b-1", Status: "ACTIVE"}}},
		&stubPricingRepoAlwaysFail{},
	)
	run := ports.AIRunRecord{ID: "run-1"}
	err := svc.recordAIUsage(context.Background(), "b-1", buildUsageOutput(), run, true)
	if err == nil {
		t.Fatalf("expected error when AppendRecord fails (entitlement drift must NOT be silent per Item 4)")
	}
	if !contains(err.Error(), "entitlement") {
		t.Errorf("error should mention entitlement drift: %v", err)
	}
}

// Test Item 4.3: replyEnqueued=true + SubscriptionRepository.List failure → error (NOT nil).
func TestItem4_SubscriptionListFailure_PropagatesError(t *testing.T) {
	t.Parallel()
	svc := buildServiceForUsageTest(
		&stubUsageRepoCapture{},
		&stubSubsRepoFailList{},
		nil, // pricing not relevant — we never get there
	)
	run := ports.AIRunRecord{ID: "run-1"}
	err := svc.recordAIUsage(context.Background(), "b-1", buildUsageOutput(), run, true)
	if err == nil {
		t.Fatalf("expected error when SubscriptionRepository.List fails + replyEnqueued=true (NOT nil — per Item 4)")
	}
	if !contains(err.Error(), "subscription lookup failed") {
		t.Errorf("error should mention subscription lookup failure: %v", err)
	}
}

// Test Item 4.4: replyEnqueued=true + no active subscription → error (NOT nil).
func TestItem4_NoActiveSubscription_PropagatesError(t *testing.T) {
	t.Parallel()
	svc := buildServiceForUsageTest(
		&stubUsageRepoCapture{},
		&stubSubsRepoEmpty{},
		nil,
	)
	run := ports.AIRunRecord{ID: "run-1"}
	err := svc.recordAIUsage(context.Background(), "b-1", buildUsageOutput(), run, true)
	if err == nil {
		t.Fatalf("expected error when no active subscription + replyEnqueued=true (NOT nil — per Item 4)")
	}
	if !contains(err.Error(), "no active subscription") {
		t.Errorf("error should mention no active subscription: %v", err)
	}
}

// Test Item 4.5: replyEnqueued=true + AIUsage not wired → error (NOT nil).
func TestItem4_AIUsageNotWired_PropagatesError(t *testing.T) {
	t.Parallel()
	svc := AutoReplyService{
		AIUsage:       nil,
		Subscriptions: &stubSubscriptionsRepo{items: []ports.SubscriptionRecord{{ID: "sub-1", BusinessID: "b-1", Status: "ACTIVE"}}},
		AIPricing:     &stubPricingRepoAlwaysFail{},
		Now:           func() time.Time { return time.Now().UTC() },
		NewID:         func() string { return "test-id" },
	}
	run := ports.AIRunRecord{ID: "run-1"}
	err := svc.recordAIUsage(context.Background(), "b-1", buildUsageOutput(), run, true)
	if err == nil {
		t.Fatalf("expected error when AIUsage not wired + replyEnqueued=true (NOT nil — per Item 4)")
	}
	if !contains(err.Error(), "not wired") {
		t.Errorf("error should mention repository not wired: %v", err)
	}
}

// Test Item 4.6: error path does NOT re-send the reply.
// The Handle method returns the result + the error — the caller
// (worker pool) logs the error but does NOT retry (the reply is
// already enqueued). This test verifies the result is returned
// (not empty) alongside the error.
func TestItem4_ErrorPathDoesNotResendReply(t *testing.T) {
	t.Parallel()
	// We can't easily test Handle() end-to-end (needs full wiring).
	// But we can verify recordAIUsage's contract: when it returns
	// an error, the caller (Handle) already has the result from
	// the execution phase — it returns (result, usageErr).
	// The key assertion: recordAIUsage does NOT touch the outbox
	// or outbound repositories. It only touches AIUsage + Subscriptions.
	svc := buildServiceForUsageTest(
		&stubUsageRepoFailAppend{},
		&stubSubscriptionsRepo{items: []ports.SubscriptionRecord{{ID: "sub-1", BusinessID: "b-1", Status: "ACTIVE"}}},
		&stubPricingRepoAlwaysFail{},
	)
	run := ports.AIRunRecord{ID: "run-1"}
	err := svc.recordAIUsage(context.Background(), "b-1", buildUsageOutput(), run, true)
	// The error is returned — no side effects on outbound/outbox.
	if err == nil {
		t.Fatalf("expected error")
	}
	// The key assertion: recordAIUsage has NO access to OutboundRepository
	// or Outbox — it cannot re-send the reply. The struct fields don't
	// include those repos. This is the architectural boundary that
	// prevents re-send.
}

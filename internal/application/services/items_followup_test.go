package services

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// ===== Item 4: AI Reply Entitlement — persistence failure boundary =====

// stubUsageRepoFailAppend always fails AppendRecord (simulates DB
// persistence failure).
type stubUsageRepoFailAppend struct{}

func (s *stubUsageRepoFailAppend) AppendRecord(_ context.Context, _ ports.AIUsageAppend) (ports.AIUsageRecord, error) {
	return ports.AIUsageRecord{}, errors.New("simulated DB failure on AppendRecord")
}
func (s *stubUsageRepoFailAppend) GetSubscriptionAIUsage(_ context.Context, _ string) (ports.SubscriptionAIUsageAggregate, error) {
	return ports.SubscriptionAIUsageAggregate{}, nil
}
func (s *stubUsageRepoFailAppend) RefreshAggregate(_ context.Context, _ string, _ time.Time) (ports.SubscriptionAIUsageAggregate, error) {
	return ports.SubscriptionAIUsageAggregate{}, nil
}
func (s *stubUsageRepoFailAppend) GetPlatformAIUsageOverview(_ context.Context) (ports.SubscriptionAIUsageAggregate, error) {
	return ports.SubscriptionAIUsageAggregate{}, nil
}
func (s *stubUsageRepoFailAppend) GetAIUsageByBusiness(_ context.Context, _ int) ([]ports.SubscriptionAIUsageAggregate, error) {
	return nil, nil
}

var _ ports.AIUsageRepository = (*stubUsageRepoFailAppend)(nil)

// Test Item 4: reply produced → usage persistence failure → entitlement
// cannot silently remain unchanged. The error MUST propagate from
// recordAIUsage so the caller (Handle → worker pool → webhook) can see
// that entitlement may have drifted.
func TestPersistenceFailurePropagatesError(t *testing.T) {
	t.Parallel()
	svc := AutoReplyService{
		AIUsage:   &stubUsageRepoFailAppend{},
		AIPricing: &stubPricingRepoAlwaysFail{},
		Subscriptions: &stubSubscriptionsRepo{
			items: []ports.SubscriptionRecord{{ID: "sub-1", BusinessID: "b-1", Status: "ACTIVE"}},
		},
		Now:   func() time.Time { return time.Now().UTC() },
		NewID: func() string { return "test-id" },
	}
	out := ports.CustomerSalesDecisionOutput{
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
	run := ports.AIRunRecord{ID: "run-1"}
	err := svc.recordAIUsage(context.Background(), "b-1", out, run, true)
	if err == nil {
		t.Fatalf("expected error when AppendRecord fails (entitlement drift must NOT be silent per Item 4), got nil")
	}
	if !contains(err.Error(), "entitlement") {
		t.Errorf("error should mention entitlement drift, got: %v", err)
	}
}

// ===== Item 5: WorkerPool race — concurrent Submit + Stop =====

// Test Item 5 race: concurrent Submit + Stop must not panic.
// Run with `go test -race` to detect the race.
func TestWorkerPoolConcurrentSubmitStopNoPanic(t *testing.T) {
	pool := NewAutoReplyWorkerPool(2, 10)
	task := autoReplyTask{
		handler:          &fakeAutoReplyHandler{},
		executionTimeout: 1 * time.Second,
	}
	var submitCount int64
	var submitOK int64
	done := make(chan struct{})
	// Concurrent submitter goroutine.
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			atomic.AddInt64(&submitCount, 1)
			if pool.Submit(task) {
				atomic.AddInt64(&submitOK, 1)
			}
		}
	}()
	// Give the submitter a head start, then call Stop concurrently.
	time.Sleep(5 * time.Millisecond)
	pool.Stop()
	<-done
	// If we reached here without panic, the race is handled.
	// Some submits may have been dropped (queue full or stopped) —
	// that's OK. The key assertion: no panic.
	if submitCount != 100 {
		t.Logf("submitted %d tasks, %d accepted", submitCount, submitOK)
	}
}

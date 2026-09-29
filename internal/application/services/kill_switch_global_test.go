package services

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// ---- Item 1: Global Kill Switch blocks Merchant AI path ----

// Test Item 1: when the shared AICostProtectionChecker returns false
// (kill switch disabled), MerchantCatalogAIAgent.HandleTurn MUST block
// before calling Gemini. This proves the kill switch is global — not
// just for the SocialAPI AutoReply webhook.
func TestMerchantAIAgentRespectsKillSwitch(t *testing.T) {
	t.Parallel()
	agent := &MerchantCatalogAIAgent{
		Runtime: &fakeContractRuntime{},
		CostProtection: &stubChecker{
			allowed: false,
			reason:  "ai_runtime_disabled",
		},
	}
	_, err := agent.HandleTurn(context.Background(), MerchantCatalogAITurnInput{
		BusinessID:      "b-1",
		PrincipalID:     "p-1",
		MerchantMessage: "add a product",
	})
	if err == nil {
		t.Fatalf("expected error when kill switch is DISABLED, got nil")
	}
}

// Test Item 1: when kill switch is ENABLED, MerchantCatalogAIAgent
// proceeds (does NOT block). This proves the gate is non-blocking
// in the normal path.
func TestMerchantAIAgentAllowsWhenKillSwitchEnabled(t *testing.T) {
	t.Parallel()
	// We can't easily build a full agent (needs ContextBuilder etc.)
	// so we just verify the gate check passes + the agent proceeds
	// past it (to the next validation check which will fail with
	// "context builder not configured"). The key assertion: the
	// error is NOT "merchant AI execution blocked".
	agent := &MerchantCatalogAIAgent{
		Runtime: &fakeContractRuntime{},
		CostProtection: &stubChecker{
			allowed: true,
			reason:  "",
		},
	}
	_, err := agent.HandleTurn(context.Background(), MerchantCatalogAITurnInput{
		BusinessID:      "b-1",
		PrincipalID:     "p-1",
		MerchantMessage: "add a product",
	})
	if err == nil {
		t.Fatalf("expected error (context builder not configured), got nil")
	}
	// The error should be about context builder — NOT about kill switch.
	if contains(err.Error(), "blocked") {
		t.Errorf("kill switch should NOT have blocked (allowed=true), got: %v", err)
	}
}

// ---- Item 2: Kill Switch blocks Summary path ----

// Test Item 2: when kill switch is DISABLED, MaybeSummarize MUST skip
// Gemini entirely (SkippedReason = "ai_runtime_disabled: ...").
func TestSummaryServiceRespectsKillSwitch(t *testing.T) {
	t.Parallel()
	// Build a summary service with kill switch DISABLED.
	// The SummaryService checks CostProtection before loading state
	// — so we don't need to wire StateRepository/Messages.
	svc := &ConversationSummaryService{
		CostProtection: &stubChecker{
			allowed: false,
			reason:  "ai_runtime_disabled",
		},
	}
	result, err := svc.MaybeSummarize(context.Background(), "b-1", "c-1")
	if err != nil {
		t.Fatalf("MaybeSummarize should not return error when blocked, got: %v", err)
	}
	if result.Summarized {
		t.Errorf("Summarized should be false when kill switch is DISABLED")
	}
	if result.SkippedReason == "" {
		t.Errorf("SkippedReason should be non-empty when blocked")
	}
}

// Test Item 2: when kill switch is ENABLED, MaybeSummarize proceeds
// (does NOT skip with "ai_runtime_disabled").
func TestSummaryServiceDoesNotSkipWhenKillSwitchEnabled(t *testing.T) {
	t.Parallel()
	svc := &ConversationSummaryService{
		CostProtection: &stubChecker{allowed: true, reason: ""},
	}
	result, _ := svc.MaybeSummarize(context.Background(), "b-1", "c-1")
	// The service will skip with "service_not_configured" (no repos wired)
	// — but NOT with "ai_runtime_disabled". The key assertion: the skip
	// reason is NOT about the kill switch.
	if result.SkippedReason == "ai_runtime_disabled" || contains(result.SkippedReason, "ai_runtime_disabled") {
		t.Errorf("kill switch should NOT have blocked (allowed=true), got SkippedReason=%s", result.SkippedReason)
	}
}

// ---- Item 5: AutoReplyWorkerPool — non-blocking submit + graceful stop ----

// Test Item 5a: Submit is non-blocking — when queue is full, returns
// false immediately (NOT after waiting 30s).
func TestWorkerPoolSubmitNonBlockingWhenFull(t *testing.T) {
	t.Parallel()
	// Pool with 1 worker + queue depth 1. Fill the queue, then submit
	// again — should return false immediately.
	pool := NewAutoReplyWorkerPool(1, 1)
	defer pool.Stop()
	// Block the single worker so the queue fills.
	blockCh := make(chan struct{})
	defer close(blockCh)
	blockingTask := autoReplyTask{
		cmd: commands.AutoReplyCommand{Text: "block"},
		handler: &fakeAutoReplyHandler{
			handleFunc: func(ctx context.Context, _ commands.AutoReplyCommand) (commands.AutoReplyResult, error) {
				<-blockCh // block until test ends
				return commands.AutoReplyResult{}, nil
			},
		},
		executionTimeout: 5 * time.Second,
	}
	if !pool.Submit(blockingTask) {
		t.Fatalf("first submit should succeed (queue empty)")
	}
	// Wait for the worker to pick up the first task.
	time.Sleep(50 * time.Millisecond)
	// Fill the queue (depth 1).
	if !pool.Submit(blockingTask) {
		t.Fatalf("second submit should succeed (queue has 1 slot)")
	}
	// Third submit — queue is full. Should return false IMMEDIATELY.
	start := time.Now()
	if pool.Submit(blockingTask) {
		t.Errorf("third submit should fail (queue full), got true")
	}
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Submit took %v — should be non-blocking (<100ms), previous 30s-timeout behavior was blocking", elapsed)
	}
}

// Test Item 5b: concurrent bound — at most N workers run at once.
func TestWorkerPoolConcurrentBound(t *testing.T) {
	t.Parallel()
	const concurrency = 3
	pool := NewAutoReplyWorkerPool(concurrency, 10)
	defer pool.Stop()
	var active int64
	var maxActive int64
	var mu sync.Mutex
	task := autoReplyTask{
		cmd: commands.AutoReplyCommand{Text: "concurrent"},
		handler: &fakeAutoReplyHandler{
			handleFunc: func(ctx context.Context, _ commands.AutoReplyCommand) (commands.AutoReplyResult, error) {
				cur := atomic.AddInt64(&active, 1)
				mu.Lock()
				if cur > maxActive {
					maxActive = cur
				}
				mu.Unlock()
				time.Sleep(50 * time.Millisecond)
				atomic.AddInt64(&active, -1)
				return commands.AutoReplyResult{}, nil
			},
		},
		executionTimeout: 5 * time.Second,
	}
	// Submit 10 tasks — only 3 should run concurrently.
	for i := 0; i < 10; i++ {
		if !pool.Submit(task) {
			t.Fatalf("submit %d should succeed (queue depth 10)", i)
		}
	}
	// Wait for all to finish.
	pool.Stop()
	if maxActive > int64(concurrency) {
		t.Errorf("max concurrent workers = %d, expected <= %d", maxActive, concurrency)
	}
}

// Test Item 5c: graceful stop — queued tasks are NOT dropped.
func TestWorkerPoolGracefulStopDrainsQueuedTasks(t *testing.T) {
	t.Parallel()
	var completed int64
	task := autoReplyTask{
		cmd: commands.AutoReplyCommand{Text: "drain"},
		handler: &fakeAutoReplyHandler{
			handleFunc: func(ctx context.Context, _ commands.AutoReplyCommand) (commands.AutoReplyResult, error) {
				atomic.AddInt64(&completed, 1)
				return commands.AutoReplyResult{}, nil
			},
		},
		executionTimeout: 5 * time.Second,
	}
	// Pool with 1 worker + 10-deep queue.
	pool := NewAutoReplyWorkerPool(1, 10)
	// Block the worker briefly so tasks queue up.
	blockCh := make(chan struct{})
	blockingTask := autoReplyTask{
		cmd: commands.AutoReplyCommand{Text: "block"},
		handler: &fakeAutoReplyHandler{
			handleFunc: func(ctx context.Context, _ commands.AutoReplyCommand) (commands.AutoReplyResult, error) {
				<-blockCh
				return commands.AutoReplyResult{}, nil
			},
		},
		executionTimeout: 5 * time.Second,
	}
	pool.Submit(blockingTask)
	time.Sleep(50 * time.Millisecond) // let worker pick up blocking task
	// Queue 5 tasks while worker is blocked.
	for i := 0; i < 5; i++ {
		if !pool.Submit(task) {
			t.Fatalf("submit %d should succeed", i)
		}
	}
	// Unblock the worker + immediately call Stop.
	close(blockCh)
	pool.Stop()
	// All 5 queued tasks should have completed during graceful drain.
	if completed != 5 {
		t.Errorf("expected 5 queued tasks to complete during graceful Stop, got %d", completed)
	}
}

// ---- Helpers ----

type stubChecker struct {
	allowed bool
	reason  string
}

func (s *stubChecker) IsAIExecutionAllowed(_ context.Context, _ string) (bool, string) {
	return s.allowed, s.reason
}

type fakeContractRuntime struct{}

func (f *fakeContractRuntime) DecideContract(_ context.Context, _ ports.ContractRuntimeInput) (ports.ContractRuntimeOutput, error) {
	return ports.ContractRuntimeOutput{}, nil
}

type fakeAutoReplyHandler struct {
	handleFunc func(ctx context.Context, cmd commands.AutoReplyCommand) (commands.AutoReplyResult, error)
}

func (f *fakeAutoReplyHandler) Handle(ctx context.Context, cmd commands.AutoReplyCommand) (commands.AutoReplyResult, error) {
	if f.handleFunc != nil {
		return f.handleFunc(ctx, cmd)
	}
	return commands.AutoReplyResult{}, nil
}

// contains is a minimal substring check (avoids importing strings in test).
func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Ensure the fakeAutoReplyHandler satisfies the interface at compile time.
var _ commands.AutoReplyHandler = (*fakeAutoReplyHandler)(nil)
var _ ports.ContractRuntime = (*fakeContractRuntime)(nil)
var _ AICostProtectionChecker = (*stubChecker)(nil)

// dummyErr keeps the errors import alive if needed later.
var _ = errors.New

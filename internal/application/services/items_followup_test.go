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
		AIUsage: &stubUsageRepoFailAppend{},
		AIPricing: &stubPricingRepoAlwaysFail{},
		Subscriptions: &stubSubscriptionsRepo{
			items: []ports.SubscriptionRecord{{ID: "sub-1", BusinessID: "b-1", Status: "ACTIVE"}},
		},
		Now:   func() time.Time { return time.Now().UTC() },
		NewID: func() string { return "test-id" },
	}
	out := ports.ContractRuntimeOutput{
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
		handler: &fakeAutoReplyHandler{},
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

// ===== Item 7: Cache error classification =====

// stubRepoNotFound returns a RepositoryError with Kind=not_found.
type stubRepoNotFound struct{}

func (s *stubRepoNotFound) CreateVersion(_ context.Context, _ ports.AIConfigurationCreate) (ports.AIConfigurationVersion, error) {
	return ports.AIConfigurationVersion{}, nil
}
func (s *stubRepoNotFound) ActivateVersion(_ context.Context, _ string, _ time.Time) (ports.AIConfigurationVersion, error) {
	return ports.AIConfigurationVersion{}, nil
}
func (s *stubRepoNotFound) GetActiveVersion(_ context.Context, _ string) (ports.AIConfigurationVersion, error) {
	return ports.AIConfigurationVersion{}, &notFoundErr{}
}
func (s *stubRepoNotFound) GetVersionByID(_ context.Context, _ string) (ports.AIConfigurationVersion, error) {
	return ports.AIConfigurationVersion{}, nil
}
func (s *stubRepoNotFound) ListVersions(_ context.Context, _ string, _ int) ([]ports.AIConfigurationVersion, error) {
	return nil, nil
}

// notFoundErr implements kindedErrorCache (ErrorKind() == "not_found").
type notFoundErr struct{}

func (e *notFoundErr) Error() string         { return "not found" }
func (e *notFoundErr) ErrorKind() string     { return "not_found" }
func (e *notFoundErr) Unwrap() error         { return nil }

// stubCredNotFound returns not_found from GetActiveCredential.
type stubCredNotFound struct{}

func (s *stubCredNotFound) StoreCredential(_ context.Context, _ ports.AICredentialCreate) (ports.AICredentialRecord, error) {
	return ports.AICredentialRecord{}, nil
}
func (s *stubCredNotFound) GetActiveCredential(_ context.Context, _ string) (ports.AICredentialRecord, string, error) {
	return ports.AICredentialRecord{}, "", &notFoundErr{}
}
func (s *stubCredNotFound) GetCredentialByID(_ context.Context, _ string) (ports.AICredentialRecord, error) {
	return ports.AICredentialRecord{}, nil
}
func (s *stubCredNotFound) GetDecryptedKeyByID(_ context.Context, _ string) (string, error) { return "", nil }
func (s *stubCredNotFound) UpdateCredentialStatus(_ context.Context, _, _ string, _ *string, _ time.Time) (ports.AICredentialRecord, error) {
	return ports.AICredentialRecord{}, nil
}
func (s *stubCredNotFound) RevokeCredential(_ context.Context, _ string, _ time.Time) error { return nil }
func (s *stubCredNotFound) ListCredentials(_ context.Context, _ string) ([]ports.AICredentialRecord, error) {
	return nil, nil
}

// stubCredDBError returns a non-not_found error from GetActiveCredential.
type stubCredDBError struct{}

func (s *stubCredDBError) StoreCredential(_ context.Context, _ ports.AICredentialCreate) (ports.AICredentialRecord, error) {
	return ports.AICredentialRecord{}, nil
}
func (s *stubCredDBError) GetActiveCredential(_ context.Context, _ string) (ports.AICredentialRecord, string, error) {
	return ports.AICredentialRecord{}, "", errors.New("connection refused")
}
func (s *stubCredDBError) GetCredentialByID(_ context.Context, _ string) (ports.AICredentialRecord, error) {
	return ports.AICredentialRecord{}, nil
}
func (s *stubCredDBError) GetDecryptedKeyByID(_ context.Context, _ string) (string, error) { return "", nil }
func (s *stubCredDBError) UpdateCredentialStatus(_ context.Context, _, _ string, _ *string, _ time.Time) (ports.AICredentialRecord, error) {
	return ports.AICredentialRecord{}, nil
}
func (s *stubCredDBError) RevokeCredential(_ context.Context, _ string, _ time.Time) error { return nil }
func (s *stubCredDBError) ListCredentials(_ context.Context, _ string) ([]ports.AICredentialRecord, error) {
	return nil, nil
}

// Test Item 7a: NO_ACTIVE_VERSION → env fallback is allowed.
func TestCacheNoActiveVersionFallsBackToEnv(t *testing.T) {
	t.Parallel()
	cache := NewAIConfigurationCache(&stubRepoNotFound{}, &stubAICredRepo{
		activeRecord:  ports.AICredentialRecord{ID: "cred-1", Status: "VALID"},
		decryptedKey: "env-key-fallback",
	})
	cache.LoadFromEnv("env-key-fallback", "env-model", "https://env.example.com", 700, 12000, "")
	cache.Invalidate()
	cfg, err := cache.GetActiveConfig(context.Background())
	if err != nil {
		t.Fatalf("expected env fallback when no active version, got error: %v", err)
	}
	if cfg.Model != "env-model" {
		t.Errorf("expected env-model fallback, got %s", cfg.Model)
	}
}

// Test Item 7b: NO_ACTIVE_CREDENTIAL → NOT classified as NO_ACTIVE_VERSION.
// The error must surface — no env fallback for missing credential.
func TestCacheNoActiveCredentialDoesNotFallbackToEnv(t *testing.T) {
	t.Parallel()
	cache := NewAIConfigurationCache(&stubRepoNotFound{}, &stubCredNotFound{})
	cache.LoadFromEnv("env-key", "env-model", "https://env.example.com", 700, 12000, "")
	cache.Invalidate()
	_, err := cache.GetActiveConfig(context.Background())
	if err == nil {
		t.Fatalf("expected error when no active credential (NOT env fallback per Item 7), got nil")
	}
	if !errors.Is(err, ErrNoActiveCredential) {
		t.Errorf("expected ErrNoActiveCredential, got: %v", err)
	}
}

// Test Item 7c: DB_FAILURE → no stale env fallback.
func TestCacheDBFailureDoesNotFallbackToEnv(t *testing.T) {
	t.Parallel()
	cache := NewAIConfigurationCache(&stubRepoNotFound{}, &stubCredDBError{})
	cache.LoadFromEnv("env-key", "env-model", "https://env.example.com", 700, 12000, "")
	cache.Invalidate()
	_, err := cache.GetActiveConfig(context.Background())
	if err == nil {
		t.Fatalf("expected error on DB failure (NOT env fallback per Item 7), got nil")
	}
	if !errors.Is(err, ErrDBFailure) {
		t.Errorf("expected ErrDBFailure, got: %v", err)
	}
}

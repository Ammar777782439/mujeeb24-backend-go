package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// stubAbortRunRepo is a minimal in-memory AIRunRepository for testing the
// markAuthorizedOrAbort / markExecutingOrAbort / markCompletedOrAbort helpers.
//
// It tracks the current run status + timestamps so we can assert on the
// lifecycle chain. It can be configured to fail specific transitions by
// setting failOnStatus (the status the patch is trying to set).
type stubAbortRunRepo struct {
	mu      sync.Mutex
	runs    map[string]*ports.AIRunRecord
	failOn  map[string]error // failOn[status] = error to return when patch.Status == status
	newIDFn func() string
}

func newStubAbortRunRepo() *stubAbortRunRepo {
	return &stubAbortRunRepo{
		runs:    make(map[string]*ports.AIRunRecord),
		failOn:  make(map[string]error),
		newIDFn: func() string { return "test-run-id" },
	}
}

func (s *stubAbortRunRepo) seed(run ports.AIRunRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := run
	s.runs[run.ID] = &r
}

func (s *stubAbortRunRepo) get(runID string) ports.AIRunRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.runs[runID]; ok {
		return *r
	}
	return ports.AIRunRecord{}
}

func (s *stubAbortRunRepo) failOnStatus(status string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failOn[status] = err
}

// CreateRun seeds the run.
func (s *stubAbortRunRepo) CreateRun(_ context.Context, run ports.AIRunRecord) (ports.AIRunRecord, error) {
	s.seed(run)
	return run, nil
}

func (s *stubAbortRunRepo) GetRun(_ context.Context, _, runID string) (ports.AIRunRecord, error) {
	return s.get(runID), nil
}

func (s *stubAbortRunRepo) GetByIdempotencyKey(_ context.Context, _, _ string) (ports.AIRunRecord, error) {
	return ports.AIRunRecord{}, nil
}

// UpdateRunStatus is the method used by AIRunLifecycle.transition.
// It applies the patch (status + timestamps) and returns the updated record.
// If failOn[patch.Status] is set, returns that error instead.
func (s *stubAbortRunRepo) UpdateRunStatus(_ context.Context, _, runID string, patch ports.AIRunStatusPatch) (ports.AIRunRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return ports.AIRunRecord{}, errors.New("run not found")
	}
	if err, fail := s.failOn[patch.Status]; fail {
		return *r, err
	}
	r.Status = patch.Status
	if patch.ContextBuiltAt != nil {
		r.ContextBuiltAt = patch.ContextBuiltAt
	}
	if patch.RunningStartedAt != nil {
		r.RunningStartedAt = patch.RunningStartedAt
	}
	if patch.WaitingToolAt != nil {
		r.WaitingToolAt = patch.WaitingToolAt
	}
	if patch.ValidatingAt != nil {
		r.ValidatingAt = patch.ValidatingAt
	}
	if patch.AuthorizedAt != nil {
		r.AuthorizedAt = patch.AuthorizedAt
	}
	if patch.ExecutingAt != nil {
		r.ExecutingAt = patch.ExecutingAt
	}
	if patch.CompletedAt != nil {
		r.CompletedAt = patch.CompletedAt
	}
	if patch.FailedAt != nil {
		r.FailedAt = patch.FailedAt
	}
	if patch.FailureStage != nil {
		r.FailureStage = *patch.FailureStage
	}
	if patch.FailureCategory != nil {
		r.FailureCategory = *patch.FailureCategory
	}
	if patch.FailureReason != nil {
		r.FailureReason = *patch.FailureReason
	}
	return *r, nil
}

func (s *stubAbortRunRepo) ListRunsByConversation(_ context.Context, _, _ string, _ int) ([]ports.AIRunRecord, error) {
	return nil, nil
}

// Stubs for the rest of AIRunRepository (not used in these tests):
func (s *stubAbortRunRepo) CreateAttempt(_ context.Context, _ ports.AIRunAttemptRecord) (ports.AIRunAttemptRecord, error) {
	return ports.AIRunAttemptRecord{}, nil
}
func (s *stubAbortRunRepo) UpdateAttempt(_ context.Context, _ string, _ ports.AIRunAttemptPatch) (ports.AIRunAttemptRecord, error) {
	return ports.AIRunAttemptRecord{}, nil
}
func (s *stubAbortRunRepo) ListAttempts(_ context.Context, _ string) ([]ports.AIRunAttemptRecord, error) {
	return nil, nil
}
func (s *stubAbortRunRepo) CreateToolCall(_ context.Context, _ ports.AIToolCallRecord) (ports.AIToolCallRecord, error) {
	return ports.AIToolCallRecord{}, nil
}
func (s *stubAbortRunRepo) UpdateToolCall(_ context.Context, _ string, _ ports.AIToolCallPatch) (ports.AIToolCallRecord, error) {
	return ports.AIToolCallRecord{}, nil
}
func (s *stubAbortRunRepo) ListToolCalls(_ context.Context, _ string) ([]ports.AIToolCallRecord, error) {
	return nil, nil
}
func (s *stubAbortRunRepo) CreateGeminiInteraction(_ context.Context, _ ports.AIGeminiInteractionRecord) (ports.AIGeminiInteractionRecord, error) {
	return ports.AIGeminiInteractionRecord{}, nil
}
func (s *stubAbortRunRepo) ListGeminiInteractions(_ context.Context, _ string) ([]ports.AIGeminiInteractionRecord, error) {
	return nil, nil
}
func (s *stubAbortRunRepo) CreateCatalogBatch(_ context.Context, _ ports.AICatalogBatchRecord) (ports.AICatalogBatchRecord, error) {
	return ports.AICatalogBatchRecord{}, nil
}
func (s *stubAbortRunRepo) UpdateCatalogBatch(_ context.Context, _ string, _ ports.AICatalogBatchPatch) (ports.AICatalogBatchRecord, error) {
	return ports.AICatalogBatchRecord{}, nil
}
func (s *stubAbortRunRepo) ListCatalogBatches(_ context.Context, _ string) ([]ports.AICatalogBatchRecord, error) {
	return nil, nil
}
func (s *stubAbortRunRepo) RecordUsage(_ context.Context, _ ports.AIUsageTelemetryRecord) (ports.AIUsageTelemetryRecord, error) {
	return ports.AIUsageTelemetryRecord{}, nil
}
func (s *stubAbortRunRepo) RecordStageLatency(_ context.Context, _ ports.AIStageLatencyRecord) (ports.AIStageLatencyRecord, error) {
	return ports.AIStageLatencyRecord{}, nil
}

var _ ports.AIRunRepository = (*stubAbortRunRepo)(nil)

// ===== Test 10-A: Lifecycle happy path — validating → authorized → executing → completed =====
//
// Per spec §10 A: proves the full happy Action path ordering with timestamps.
// All three timestamps (authorized_at, executing_at, completed_at) MUST be
// NOT NULL + ordered authorized_at <= executing_at <= completed_at.
func TestLifecycleAbort_HappyPath_Ordering(t *testing.T) {
	repo := newStubAbortRunRepo()
	now := time.Now().UTC()
	run := ports.AIRunRecord{
		ID:           "run-happy",
		BusinessID:   "biz-1",
		Status:       ports.AIRunStatusValidating,
		ValidatingAt: &now,
	}
	repo.seed(run)

	svc := AutoReplyService{
		RunRepository: repo,
		Now:           func() time.Time { return time.Now().UTC() },
		NewID:         func() string { return "new-id" },
	}

	ctx := context.Background()
	// Step 1: validating → authorized
	updated, err := svc.markAuthorizedOrAbort(ctx, run)
	if err != nil {
		t.Fatalf("markAuthorizedOrAbort: unexpected error: %v", err)
	}
	if updated.AuthorizedAt == nil {
		t.Fatalf("markAuthorizedOrAbort: authorized_at is nil after successful transition")
	}
	run = updated

	// Step 2: authorized → executing
	updated, err = svc.markExecutingOrAbort(ctx, run)
	if err != nil {
		t.Fatalf("markExecutingOrAbort: unexpected error: %v", err)
	}
	if updated.ExecutingAt == nil {
		t.Fatalf("markExecutingOrAbort: executing_at is nil after successful transition")
	}
	run = updated

	// Step 3: executing → completed
	updated, err = svc.markCompletedOrAbort(ctx, run)
	if err != nil {
		t.Fatalf("markCompletedOrAbort: unexpected error: %v", err)
	}
	if updated.CompletedAt == nil {
		t.Fatalf("markCompletedOrAbort: completed_at is nil after successful transition")
	}
	run = updated

	// Ordering: authorized_at <= executing_at <= completed_at
	if run.AuthorizedAt.After(*run.ExecutingAt) {
		t.Fatalf("ordering violation: authorized_at (%v) > executing_at (%v)", run.AuthorizedAt, run.ExecutingAt)
	}
	if run.ExecutingAt.After(*run.CompletedAt) {
		t.Fatalf("ordering violation: executing_at (%v) > completed_at (%v)", run.ExecutingAt, run.CompletedAt)
	}
	if run.Status != ports.AIRunStatusCompleted {
		t.Fatalf("final status = %s, want completed", run.Status)
	}
	t.Logf("happy path OK: authorized_at=%v executing_at=%v completed_at=%v (ordered)",
		run.AuthorizedAt, run.ExecutingAt, run.CompletedAt)
}

// ===== Test 10-B: MarkAuthorized failure → no execution =====
//
// Per spec §10 B + §2: proves that a MarkAuthorized failure returns an
// error and the caller MUST NOT proceed to execution. The run is marked
// FAILED (via markFailedSafe) so it does not stay in a non-terminal state.
func TestLifecycleAbort_MarkAuthorizedFailure_NoExecution(t *testing.T) {
	repo := newStubAbortRunRepo()
	repo.failOnStatus(ports.AIRunStatusAuthorized, errors.New("DB unreachable for MarkAuthorized"))
	now := time.Now().UTC()
	run := ports.AIRunRecord{
		ID:           "run-auth-fail",
		BusinessID:   "biz-1",
		Status:       ports.AIRunStatusValidating,
		ValidatingAt: &now,
	}
	repo.seed(run)

	svc := AutoReplyService{
		RunRepository: repo,
		Now:           func() time.Time { return time.Now().UTC() },
		NewID:         func() string { return "new-id" },
	}

	ctx := context.Background()
	_, err := svc.markAuthorizedOrAbort(ctx, run)
	if err == nil {
		t.Fatalf("markAuthorizedOrAbort: expected error on MarkAuthorized failure, got nil")
	}
	if !strings.Contains(err.Error(), "lifecycle transition authorized failed") {
		t.Fatalf("markAuthorizedOrAbort: error message mismatch: got %q, want substring 'lifecycle transition authorized failed'", err.Error())
	}

	// Per spec §1 D: the run MUST be marked FAILED so it does not stay
	// in a non-terminal state. markFailedSafe was called inside
	// markAuthorizedOrAbort.
	finalRun := repo.get(run.ID)
	if finalRun.Status != ports.AIRunStatusFailed {
		t.Fatalf("after MarkAuthorized failure, run.Status = %s, want failed (markFailedSafe should have been called)", finalRun.Status)
	}
	if finalRun.FailureStage != ports.AIRunFailureStageAuthorization {
		t.Fatalf("FailureStage = %s, want %s", finalRun.FailureStage, ports.AIRunFailureStageAuthorization)
	}
	if finalRun.AuthorizedAt != nil {
		t.Fatalf("authorized_at should be nil (transition failed), got %v", finalRun.AuthorizedAt)
	}
	if finalRun.ExecutingAt != nil {
		t.Fatalf("executing_at should be nil (no execution happened), got %v", finalRun.ExecutingAt)
	}
	t.Logf("MarkAuthorized failure OK: run marked FAILED, no execution, authorized_at=nil executing_at=nil")
}

// ===== Test 10-C: MarkExecuting failure → no outbound =====
//
// Per spec §10 C + §2: proves that a MarkExecuting failure returns an
// error and the caller MUST NOT create an outbound message. The run is
// marked FAILED.
func TestLifecycleAbort_MarkExecutingFailure_NoOutbound(t *testing.T) {
	repo := newStubAbortRunRepo()
	repo.failOnStatus(ports.AIRunStatusExecuting, errors.New("DB unreachable for MarkExecuting"))
	now := time.Now().UTC()
	run := ports.AIRunRecord{
		ID:           "run-exec-fail",
		BusinessID:   "biz-1",
		Status:       ports.AIRunStatusAuthorized,
		AuthorizedAt: &now,
	}
	repo.seed(run)

	svc := AutoReplyService{
		RunRepository: repo,
		Now:           func() time.Time { return time.Now().UTC() },
		NewID:         func() string { return "new-id" },
	}

	ctx := context.Background()
	_, err := svc.markExecutingOrAbort(ctx, run)
	if err == nil {
		t.Fatalf("markExecutingOrAbort: expected error on MarkExecuting failure, got nil")
	}
	if !strings.Contains(err.Error(), "lifecycle transition executing failed") {
		t.Fatalf("markExecutingOrAbort: error message mismatch: got %q, want substring 'lifecycle transition executing failed'", err.Error())
	}

	// Per spec §1 D: the run MUST be marked FAILED.
	finalRun := repo.get(run.ID)
	if finalRun.Status != ports.AIRunStatusFailed {
		t.Fatalf("after MarkExecuting failure, run.Status = %s, want failed", finalRun.Status)
	}
	if finalRun.FailureStage != ports.AIRunFailureStageExecution {
		t.Fatalf("FailureStage = %s, want %s", finalRun.FailureStage, ports.AIRunFailureStageExecution)
	}
	if finalRun.ExecutingAt != nil {
		t.Fatalf("executing_at should be nil (transition failed), got %v", finalRun.ExecutingAt)
	}
	t.Logf("MarkExecuting failure OK: run marked FAILED, no outbound, executing_at=nil")
}

// ===== Test 10-D: MarkCompleted failure is NOT swallowed =====
//
// Per spec §10 D + §2: proves that a MarkCompleted failure returns an
// error to the caller (not swallowed via log-and-continue). The reply was
// already enqueued, so we do NOT call markFailedSafe (that would be
// misleading). Instead, the run stays in EXECUTING (non-terminal) and the
// error is returned so the caller can alert operators.
func TestLifecycleAbort_MarkCompletedFailure_NotSwallowed(t *testing.T) {
	repo := newStubAbortRunRepo()
	repo.failOnStatus(ports.AIRunStatusCompleted, errors.New("DB unreachable for MarkCompleted"))
	now := time.Now().UTC()
	run := ports.AIRunRecord{
		ID:           "run-completed-fail",
		BusinessID:   "biz-1",
		Status:       ports.AIRunStatusExecuting,
		AuthorizedAt: &now,
		ExecutingAt:  &now,
	}
	repo.seed(run)

	svc := AutoReplyService{
		RunRepository: repo,
		Now:           func() time.Time { return time.Now().UTC() },
		NewID:         func() string { return "new-id" },
	}

	ctx := context.Background()
	_, err := svc.markCompletedOrAbort(ctx, run)
	if err == nil {
		t.Fatalf("markCompletedOrAbort: expected error on MarkCompleted failure, got nil")
	}
	if !strings.Contains(err.Error(), "lifecycle transition completed failed") {
		t.Fatalf("markCompletedOrAbort: error message mismatch: got %q, want substring 'lifecycle transition completed failed'", err.Error())
	}
	if !strings.Contains(err.Error(), "manual reconciliation") {
		t.Fatalf("markCompletedOrAbort: error should mention manual reconciliation, got %q", err.Error())
	}

	// Per spec §2: the run stays in EXECUTING (not marked FAILED — the reply
	// succeeded, not failed). The operator must reconcile manually.
	finalRun := repo.get(run.ID)
	if finalRun.Status != ports.AIRunStatusExecuting {
		t.Fatalf("after MarkCompleted failure, run.Status = %s, want executing (do NOT mark failed — reply was already sent)", finalRun.Status)
	}
	if finalRun.CompletedAt != nil {
		t.Fatalf("completed_at should be nil (transition failed), got %v", finalRun.CompletedAt)
	}
	if finalRun.FailedAt != nil {
		t.Fatalf("failed_at should be nil (we do NOT mark failed on MarkCompleted failure — reply was already enqueued), got %v", finalRun.FailedAt)
	}
	t.Logf("MarkCompleted failure OK: run stays EXECUTING (not failed), error propagated, manual reconciliation needed")
}

// ===== Test 10-A (variant): full happy-path chain via the lifecycle itself =====
//
// This test exercises the *AIRunLifecycle directly (not via AutoReplyService
// helpers) to prove the state machine itself enforces the ordering
// received → context_built → running → validating → authorized → executing
// → completed with all 6 timestamps populated.
func TestLifecycleAbort_FullChain_AllTimestampsPopulated(t *testing.T) {
	repo := newStubAbortRunRepo()
	now := time.Now().UTC()
	run := ports.AIRunRecord{
		ID:         "run-full-chain",
		BusinessID: "biz-1",
		Status:     ports.AIRunStatusReceived,
		StartedAt:  now,
	}
	repo.seed(run)

	lc := NewAIRunLifecycle(repo)
	ctx := context.Background()

	// received → context_built
	r, err := lc.MarkContextBuilt(ctx, run.BusinessID, run.ID)
	if err != nil {
		t.Fatalf("MarkContextBuilt: %v", err)
	}
	if r.ContextBuiltAt == nil {
		t.Fatalf("context_built_at is nil")
	}
	// context_built → running
	r, err = lc.MarkRunning(ctx, run.BusinessID, run.ID)
	if err != nil {
		t.Fatalf("MarkRunning: %v", err)
	}
	if r.RunningStartedAt == nil {
		t.Fatalf("running_started_at is nil")
	}
	// running → validating
	r, err = lc.MarkValidating(ctx, run.BusinessID, run.ID)
	if err != nil {
		t.Fatalf("MarkValidating: %v", err)
	}
	if r.ValidatingAt == nil {
		t.Fatalf("validating_at is nil")
	}
	// validating → authorized
	r, err = lc.MarkAuthorized(ctx, run.BusinessID, run.ID)
	if err != nil {
		t.Fatalf("MarkAuthorized: %v", err)
	}
	if r.AuthorizedAt == nil {
		t.Fatalf("authorized_at is nil")
	}
	// authorized → executing
	r, err = lc.MarkExecuting(ctx, run.BusinessID, run.ID)
	if err != nil {
		t.Fatalf("MarkExecuting: %v", err)
	}
	if r.ExecutingAt == nil {
		t.Fatalf("executing_at is nil")
	}
	// executing → completed
	r, err = lc.MarkCompleted(ctx, run.BusinessID, run.ID)
	if err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}
	if r.CompletedAt == nil {
		t.Fatalf("completed_at is nil")
	}
	if r.Status != ports.AIRunStatusCompleted {
		t.Fatalf("final status = %s, want completed", r.Status)
	}

	// Ordering: started_at <= context_built_at <= running_started_at <=
	// validating_at <= authorized_at <= executing_at <= completed_at
	if r.ContextBuiltAt.Before(r.StartedAt) {
		t.Fatalf("ordering: context_built_at < started_at")
	}
	if r.RunningStartedAt.Before(*r.ContextBuiltAt) {
		t.Fatalf("ordering: running_started_at < context_built_at")
	}
	if r.ValidatingAt.Before(*r.RunningStartedAt) {
		t.Fatalf("ordering: validating_at < running_started_at")
	}
	if r.AuthorizedAt.Before(*r.ValidatingAt) {
		t.Fatalf("ordering: authorized_at < validating_at")
	}
	if r.ExecutingAt.Before(*r.AuthorizedAt) {
		t.Fatalf("ordering: executing_at < authorized_at")
	}
	if r.CompletedAt.Before(*r.ExecutingAt) {
		t.Fatalf("ordering: completed_at < executing_at")
	}
	t.Logf("full chain OK: started=%v context_built=%v running=%v validating=%v authorized=%v executing=%v completed=%v",
		r.StartedAt, r.ContextBuiltAt, r.RunningStartedAt, r.ValidatingAt, r.AuthorizedAt, r.ExecutingAt, r.CompletedAt)
}

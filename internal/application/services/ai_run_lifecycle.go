// Package services — AI Run Lifecycle State Machine
//
// Implements contract ⑨ AI Runtime Lifecycle. This is the operational
// state machine that transitions an AI Run through:
//   RECEIVED → CONTEXT_BUILT → RUNNING → WAITING_TOOL → VALIDATING
//   → AUTHORIZED → EXECUTING → COMPLETED
//
// Failed runs end in FAILED; cancelled runs end in CANCELLED.
//
// Per contract ⑨ §26, no random state transitions are allowed. The state
// machine below is the single authority on which transitions are legal.
// Per contract ⑨ §31, the only terminal states are COMPLETED, FAILED, CANCELLED.
//
// This service does NOT mutate business state directly (contract ⑨ §28).
// Business state mutations happen in application/domain layers after a
// successful AUTHORIZED transition and a separate Execution step.

package services

import (
        "context"
        "errors"
        "fmt"
        "strings"
        "time"

        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// AIRunLifecycle manages transitions for an AI Run. It is the single authority
// on which transitions are legal per contract ⑨ §26.
//
// The lifecycle persists each transition to AIRunRepository so that failures
// are recoverable and observable. Per contract ⑧ §13-14, every stage transition
// is recorded for AI Trace.
type AIRunLifecycle struct {
        Repo ports.AIRunRepository
        Now  func() time.Time
}

func NewAIRunLifecycle(repo ports.AIRunRepository) *AIRunLifecycle {
        return &AIRunLifecycle{Repo: repo, Now: func() time.Time { return time.Now().UTC() }}
}

// StartRun creates a new AI Run in RECEIVED status. Per contract ⑨ §16, the
// (business_id, idempotency_key) uniqueness prevents duplicate Run creation
// for the same source event. If a Run already exists for the same key, it is
// returned as-is instead of being re-created.
func (l *AIRunLifecycle) StartRun(ctx context.Context, input StartRunInput) (ports.AIRunRecord, error) {
        if strings.TrimSpace(input.BusinessID) == "" || strings.TrimSpace(input.IdempotencyKey) == "" {
                return ports.AIRunRecord{}, errors.New("business_id and idempotency_key are required to start an AI Run")
        }
        if input.AgentRole != ports.AIRunAgentRoleCustomerSales && input.AgentRole != ports.AIRunAgentRoleMerchantCatalogAuthoring {
                return ports.AIRunRecord{}, fmt.Errorf("unsupported agent_role %q per contract 11", input.AgentRole)
        }
        if l.Repo == nil {
                return ports.AIRunRecord{}, errors.New("AI Run repository is not configured")
        }

        // Idempotency: if a Run exists for this (business, key), resume it.
        //
        // Per ADR-043 fix: only resume if the existing Run is in a NON-TERMINAL
        // state (received/running/validating/etc.). If it's in a terminal state
        // (completed/failed/cancelled), we treat this as a NEW turn — the
        // idempotency key collision is on a past turn that already finished,
        // so we create a fresh Run for the current request.
        //
        // This prevents the "illegal transition: completed → context_built"
        // error when a merchant sends the same message in two different
        // sessions (or restarts the server and re-sends).
        //
        // Per contract ⑨ §16, idempotency is meant to dedupe CONCURRENT
        // duplicate requests (e.g., double-click). A past completed run is
        // not a "duplicate" of a new turn — it's a previous turn whose
        // result we already returned.
        if existing, err := l.Repo.GetByIdempotencyKey(ctx, input.BusinessID, input.IdempotencyKey); err == nil && existing.ID != "" {
                if !isTerminalRunStatus(existing.Status) {
                        // Non-terminal: in-flight or paused. Resume it (dedupe).
                        return existing, nil
                }
                // Terminal: fall through to create a new Run. The new Run will
                // have a different ID but the same idempotency_key — this is
                // expected; the idempotency_key column has a UNIQUE constraint
                // only on (business_id, idempotency_key) for non-terminal runs
                // (the DB layer enforces this via a partial unique index, or
                // we accept that the new INSERT will conflict and we'll get
                // errIdempotencyViolation from CreateRun — in which case we
                // return the existing terminal run instead).
        }

        now := l.Now()
        run := ports.AIRunRecord{
                ID:             input.NewRunID(),
                BusinessID:     input.BusinessID,
                ConversationID: input.ConversationID,
                MessageID:      input.MessageID,
                SourceEventID:  input.SourceEventID,
                IdempotencyKey: input.IdempotencyKey,
                AgentRole:      input.AgentRole,
                Status:         ports.AIRunStatusReceived,
                StartedAt:      now,
                CreatedAt:      now,
                UpdatedAt:      now,
        }
        return l.Repo.CreateRun(ctx, run)
}

// MarkContextBuilt transitions RECEIVED → CONTEXT_BUILT.
// Per contract ⑨ §3, CONTEXT_BUILT means System Rules + Business Context +
// Conversation Context + Conversation State + Catalog Entity Contract +
// Current Message are all assembled and ready to send to Gemini.
func (l *AIRunLifecycle) MarkContextBuilt(ctx context.Context, businessID, runID string) (ports.AIRunRecord, error) {
        now := l.Now()
        patch := ports.AIRunStatusPatch{
                Status:         ports.AIRunStatusContextBuilt,
                ContextBuiltAt: &now,
        }
        return l.transition(ctx, businessID, runID, patch, []string{ports.AIRunStatusReceived})
}

// MarkRunning transitions CONTEXT_BUILT or WAITING_TOOL → RUNNING.
// The WAITING_TOOL → RUNNING transition is the loop return per contract ⑨ §4.
func (l *AIRunLifecycle) MarkRunning(ctx context.Context, businessID, runID string) (ports.AIRunRecord, error) {
        now := l.Now()
        patch := ports.AIRunStatusPatch{
                Status:           ports.AIRunStatusRunning,
                RunningStartedAt: &now,
        }
        return l.transition(ctx, businessID, runID, patch, []string{ports.AIRunStatusContextBuilt, ports.AIRunStatusWaitingTool})
}

// MarkWaitingTool transitions RUNNING → WAITING_TOOL.
// Per contract ⑨ §4, this happens when Gemini invoked a tool and Mujeeb must
// execute the tool and return its result before continuing.
func (l *AIRunLifecycle) MarkWaitingTool(ctx context.Context, businessID, runID string) (ports.AIRunRecord, error) {
        now := l.Now()
        patch := ports.AIRunStatusPatch{
                Status:        ports.AIRunStatusWaitingTool,
                WaitingToolAt: &now,
        }
        return l.transition(ctx, businessID, runID, patch, []string{ports.AIRunStatusRunning})
}

// MarkValidating transitions RUNNING → VALIDATING.
// Per contract ⑨ §3, the moment Gemini's Proposal reaches Mujeeb, validation begins.
func (l *AIRunLifecycle) MarkValidating(ctx context.Context, businessID, runID string) (ports.AIRunRecord, error) {
        now := l.Now()
        patch := ports.AIRunStatusPatch{
                Status:       ports.AIRunStatusValidating,
                ValidatingAt: &now,
        }
        return l.transition(ctx, businessID, runID, patch, []string{ports.AIRunStatusRunning})
}

// MarkAuthorized transitions VALIDATING → AUTHORIZED.
// Per contract ⑥ §17, this means Structural + Reference + Tenant + Ownership
// + Policy + Authorization all passed and an Effective Decision exists.
// Execution may now begin.
func (l *AIRunLifecycle) MarkAuthorized(ctx context.Context, businessID, runID string) (ports.AIRunRecord, error) {
        now := l.Now()
        patch := ports.AIRunStatusPatch{
                Status:       ports.AIRunStatusAuthorized,
                AuthorizedAt: &now,
        }
        return l.transition(ctx, businessID, runID, patch, []string{ports.AIRunStatusValidating})
}

// MarkExecuting transitions AUTHORIZED → EXECUTING.
// Per contract ⑥ §19, Execution only starts after Authorization.
func (l *AIRunLifecycle) MarkExecuting(ctx context.Context, businessID, runID string) (ports.AIRunRecord, error) {
        now := l.Now()
        patch := ports.AIRunStatusPatch{
                Status:      ports.AIRunStatusExecuting,
                ExecutingAt: &now,
        }
        return l.transition(ctx, businessID, runID, patch, []string{ports.AIRunStatusAuthorized})
}

// MarkCompleted transitions EXECUTING (or VALIDATING/AUTHORIZED when no Action
// is required) → COMPLETED.
//
// Per contract ⑨ §31, COMPLETED means the Run itself ended successfully along
// its path. It does NOT always mean "an outbound message was sent" — that is
// determined by the Execution Result, not by the Run status alone.
func (l *AIRunLifecycle) MarkCompleted(ctx context.Context, businessID, runID string) (ports.AIRunRecord, error) {
        now := l.Now()
        patch := ports.AIRunStatusPatch{
                Status:      ports.AIRunStatusCompleted,
                CompletedAt: &now,
        }
        // COMPLETED can be reached from EXECUTING (full path) OR directly from
        // VALIDATING/AUTHORIZED when the Effective Decision has no Action to execute
        // (per contract ⑨ §3 "in the absence of an execution action").
        return l.transition(ctx, businessID, runID, patch, []string{
                ports.AIRunStatusExecuting,
                ports.AIRunStatusValidating,
                ports.AIRunStatusAuthorized,
        })
}

// MarkFailed transitions any non-terminal state → FAILED.
// Per contract ⑨ §27, FAILED is terminal; the only valid forward transition
// from FAILED is via a brand-new Retry Run (a new ai_run_id), not by reopening.
//
// failureStage and failureCategory determine Retryability per contract ⑨ §6-7.
func (l *AIRunLifecycle) MarkFailed(ctx context.Context, businessID, runID, stage, category, reason string) (ports.AIRunRecord, error) {
        if !isValidFailureStage(stage) {
                return ports.AIRunRecord{}, fmt.Errorf("invalid failure_stage %q", stage)
        }
        if !isValidFailureCategory(category) {
                return ports.AIRunRecord{}, fmt.Errorf("invalid failure_category %q", category)
        }
        now := l.Now()
        patch := ports.AIRunStatusPatch{
                Status:          ports.AIRunStatusFailed,
                FailureStage:    &stage,
                FailureCategory: &category,
                FailureReason:   &reason,
                FailedAt:        &now,
        }
        // Per contract ⑨ §26, FAILED can be reached from any non-terminal state.
        return l.transition(ctx, businessID, runID, patch, []string{
                ports.AIRunStatusReceived,
                ports.AIRunStatusContextBuilt,
                ports.AIRunStatusRunning,
                ports.AIRunStatusWaitingTool,
                ports.AIRunStatusValidating,
                ports.AIRunStatusAuthorized,
                ports.AIRunStatusExecuting,
        })
}

// MarkCancelled transitions any non-terminal state → CANCELLED.
// Per contract ⑨ §24-25, Cancellation may happen before or during Execution.
// When cancelled during Execution, the external action may or may not have
// rolled back; that is the Action's responsibility, not the Run's.
func (l *AIRunLifecycle) MarkCancelled(ctx context.Context, businessID, runID string) (ports.AIRunRecord, error) {
        now := l.Now()
        patch := ports.AIRunStatusPatch{
                Status:      ports.AIRunStatusCancelled,
                CancelledAt: &now,
        }
        return l.transition(ctx, businessID, runID, patch, []string{
                ports.AIRunStatusReceived,
                ports.AIRunStatusContextBuilt,
                ports.AIRunStatusRunning,
                ports.AIRunStatusWaitingTool,
                ports.AIRunStatusValidating,
                ports.AIRunStatusAuthorized,
                ports.AIRunStatusExecuting,
        })
}

// transition is the internal helper that enforces contract ⑨ §26: only valid
// forward transitions are allowed. Terminal states (COMPLETED/FAILED/CANCELLED)
// cannot transition to anything else via this helper.
func (l *AIRunLifecycle) transition(ctx context.Context, businessID, runID string, patch ports.AIRunStatusPatch, allowedFrom []string) (ports.AIRunRecord, error) {
        if l.Repo == nil {
                return ports.AIRunRecord{}, errors.New("AI Run repository is not configured")
        }
        run, err := l.Repo.GetRun(ctx, businessID, runID)
        if err != nil {
                return ports.AIRunRecord{}, err
        }
        allowed := false
        for _, s := range allowedFrom {
                if run.Status == s {
                        allowed = true
                        break
                }
        }
        if !allowed {
                return ports.AIRunRecord{}, fmt.Errorf("illegal AI Run transition: %s → %s (allowed from %v) per contract ⑨ §26", run.Status, patch.Status, allowedFrom)
        }
        return l.Repo.UpdateRunStatus(ctx, businessID, runID, patch)
}

// StartRunInput carries the required fields to start a new AI Run.
type StartRunInput struct {
        BusinessID     string
        ConversationID string
        MessageID      string
        SourceEventID  string
        IdempotencyKey string
        AgentRole      string
        NewRunID       func() string
}

// isTerminalRunStatus returns true if the given status is a terminal state
// (no further transitions allowed per contract ⑨ §3). Used by StartRun to
// decide whether to resume an existing Run (non-terminal) or create a new
// one (terminal — the past Run's result is the answer for the past turn,
// not for this new turn).
//
// Per ADR-043.
func isTerminalRunStatus(status string) bool {
        switch status {
        case ports.AIRunStatusCompleted,
                ports.AIRunStatusFailed,
                ports.AIRunStatusCancelled:
                return true
        }
        return false
}

func isValidFailureStage(stage string) bool {
        switch stage {
        case ports.AIRunFailureStageContextBuild,
                ports.AIRunFailureStageGeminiRequest,
                ports.AIRunFailureStageToolCall,
                ports.AIRunFailureStageValidation,
                ports.AIRunFailureStagePolicy,
                ports.AIRunFailureStageAuthorization,
                ports.AIRunFailureStageExecution,
                "":
                return true
        }
        return false
}

func isValidFailureCategory(category string) bool {
        switch category {
        case ports.AIRunFailureCategoryProviderTemporary,
                ports.AIRunFailureCategoryProviderPermanent,
                ports.AIRunFailureCategoryNetwork,
                ports.AIRunFailureCategoryTimeout,
                ports.AIRunFailureCategoryRateLimit,
                ports.AIRunFailureCategoryInvalidAIOutput,
                ports.AIRunFailureCategoryInvalidToolArguments,
                ports.AIRunFailureCategoryTenantViolation,
                ports.AIRunFailureCategoryInvalidReference,
                ports.AIRunFailureCategoryPolicyDenial,
                ports.AIRunFailureCategoryAuthorizationDenial,
                ports.AIRunFailureCategoryUnsupportedAction,
                ports.AIRunFailureCategoryExecutionFailure,
                ports.AIRunFailureCategoryInfrastructure,
                "":
                return true
        }
        return false
}

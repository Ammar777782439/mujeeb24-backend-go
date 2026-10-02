// Package ports contains the application contracts used by Mujeeb AI capabilities.

package ports

// AI Run operational trace contracts are shared infrastructure across AI capabilities.
// They describe Run/Attempt/Tool/Interaction/Batch/Usage state and lifecycle ports,
// not B2B or B2C business semantics.

import (
	"context"
	"time"
)

// ════════════════════════════════════════════════════════════════════════════
// Contract ⑨ §2 — AI Run Lifecycle State
// ════════════════════════════════════════════════════════════════════════════

// AIRunStatus — the ten closed operational lifecycle states per contract ⑨ §2.
//
// These live in ai_runs.status, NOT in ai_decisions.lifecycle. The two are
// distinct concepts: ai_decisions.lifecycle is the BUSINESS decision lifecycle
// (proposed → validated → authorized → executed/expired), while ai_runs.status
// is the OPERATIONAL trace lifecycle.
const (
	AIRunStatusReceived     = "received"
	AIRunStatusContextBuilt = "context_built"
	AIRunStatusRunning      = "running"
	AIRunStatusWaitingTool  = "waiting_tool"
	AIRunStatusValidating   = "validating"
	AIRunStatusAuthorized   = "authorized"
	AIRunStatusExecuting    = "executing"
	AIRunStatusCompleted    = "completed"
	AIRunStatusFailed       = "failed"
	AIRunStatusCancelled    = "cancelled"
)

// AIRunAgentRole — the closed agent roles per contract 11 §2.
//
// Customer Sales AI and Merchant Catalog AI are independent agents that share
// infrastructure (Catalog Contract, Validation, Audit, Gemini) but have distinct
// system prompts, agent roles, tool permissions, and proposal contracts.
const (
	AIRunAgentRoleCustomerSales            = "customer_sales"
	AIRunAgentRoleMerchantCatalogAuthoring = "merchant_catalog_authoring"
)

// AIRunFailureStage per contract ⑨ §30 — pinpoints where the run failed.
const (
	AIRunFailureStageContextBuild  = "context_build"
	AIRunFailureStageGeminiRequest = "gemini_request"
	AIRunFailureStageToolCall      = "tool_call"
	AIRunFailureStageValidation    = "validation"
	AIRunFailureStagePolicy        = "policy"
	AIRunFailureStageAuthorization = "authorization"
	AIRunFailureStageExecution     = "execution"
)

// AIRunFailureCategory per contract ⑨ §6-7 and ⑧ §10 — Retryable vs Non-Retryable.
//
// Per contract ⑨ §6-7:
//
//	Retryable (transient):  provider_temporary, network, timeout, rate_limit, infrastructure
//	Non-Retryable:           provider_permanent, invalid_ai_output, invalid_tool_arguments,
//	                         tenant_violation, invalid_reference, policy_denial,
//	                         authorization_denial, unsupported_action, execution_failure
//
// The Retry Policy service (services/ai_runtime_retry.go) uses this category
// to decide whether to retry the attempt or fail the run.
const (
	AIRunFailureCategoryProviderTemporary    = "provider_temporary"
	AIRunFailureCategoryProviderPermanent    = "provider_permanent"
	AIRunFailureCategoryNetwork              = "network"
	AIRunFailureCategoryTimeout              = "timeout"
	AIRunFailureCategoryRateLimit            = "rate_limit"
	AIRunFailureCategoryInvalidAIOutput      = "invalid_ai_output"
	AIRunFailureCategoryInvalidToolArguments = "invalid_tool_arguments"
	AIRunFailureCategoryTenantViolation      = "tenant_violation"
	AIRunFailureCategoryInvalidReference     = "invalid_reference"
	AIRunFailureCategoryPolicyDenial         = "policy_denial"
	AIRunFailureCategoryAuthorizationDenial  = "authorization_denial"
	AIRunFailureCategoryUnsupportedAction    = "unsupported_action"
	AIRunFailureCategoryExecutionFailure     = "execution_failure"
	AIRunFailureCategoryInfrastructure       = "infrastructure"
)

// IsRetryable returns true when the failure category represents a transient
// error that may succeed on a fresh attempt, per contract ⑨ §6.
func (c AIRunFailureCategory) IsRetryable() bool {
	switch c {
	case AIRunFailureCategoryProviderTemporary,
		AIRunFailureCategoryNetwork,
		AIRunFailureCategoryTimeout,
		AIRunFailureCategoryRateLimit,
		AIRunFailureCategoryInfrastructure:
		return true
	}
	return false
}

// AIRunFailureCategory is a string type for type-safety in service code.
type AIRunFailureCategory string

// ════════════════════════════════════════════════════════════════════════════
// Contract ⑨ §2 — AI Run Record (operational row in ai_runs table)
// ════════════════════════════════════════════════════════════════════════════

// AIRunRecord mirrors the ai_runs row. It is the operational trace header;
// the business decision (ai_decisions) is linked via ai_run_id on that side.
//
// Per contract ⑧ §5, an AI Run is associated with exactly one business,
// conversation, message, and (optionally) source event. It is NOT a Domain
// Entity — it is an operational record.
type AIRunRecord struct {
	ID                    string
	BusinessID            string
	ConversationID        string
	MessageID             string
	SourceEventID         string
	IdempotencyKey        string
	AgentRole             string
	Status                string
	FailureStage          string
	FailureCategory       string
	FailureReason         string
	GeminiInteractionID   string
	PreviousInteractionID string

	StartedAt        time.Time
	ContextBuiltAt   *time.Time
	RunningStartedAt *time.Time
	WaitingToolAt    *time.Time
	ValidatingAt     *time.Time
	AuthorizedAt     *time.Time
	ExecutingAt      *time.Time
	CompletedAt      *time.Time
	FailedAt         *time.Time
	CancelledAt      *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// AIRunAttemptRecord mirrors ai_run_attempts row. Per contract ⑨ §27, an
// Attempt is one operational execution attempt within a logical AI Run.
type AIRunAttemptRecord struct {
	ID                 string
	AIRunID            string
	AttemptNumber      int
	Provider           string
	Model              string
	Status             string
	FailureStage       string
	FailureCategory    string
	FailureReason      string
	RequestPayloadHash string
	StartedAt          time.Time
	FinishedAt         *time.Time
	CreatedAt          time.Time
}

// AIToolCallRecord mirrors ai_tool_calls row. Per contract ⑧ §7, every
// function/tool invocation by Gemini is traced here.
type AIToolCallRecord struct {
	ID            string
	AIRunID       string
	AttemptID     string
	ToolName      string
	ToolCallID    string
	Status        string
	RequestParams []byte
	ResultPayload []byte
	FailureReason string
	StartedAt     time.Time
	FinishedAt    *time.Time
	LatencyMs     int
	CreatedAt     time.Time
}

// AIGeminiInteractionRecord mirrors ai_gemini_interactions row.
type AIGeminiInteractionRecord struct {
	ID                    string
	AIRunID               string
	AttemptID             string
	GeminiInteractionID   string
	PreviousInteractionID string
	Model                 string
	SystemInstructionHash string
	ToolsHash             string
	GenerationConfigHash  string
	StartedAt             time.Time
	FinishedAt            *time.Time
	CreatedAt             time.Time
}

// AICatalogBatchRecord mirrors ai_catalog_batches row. Per contract ② §22 and
// ⑨ §22, every batch in a Catalog Evaluation has a tracked state.
type AICatalogBatchRecord struct {
	ID             string
	AIRunID        string
	BatchNumber    int
	Status         string
	ItemsCount     int
	SchemasCount   int
	InputTokens    int
	CandidateCount int
	FailureReason  string
	StartedAt      *time.Time
	CompletedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// AIUsageTelemetryRecord mirrors ai_usage_telemetry row. Per contract ⑧ §8.
type AIUsageTelemetryRecord struct {
	ID                      string
	AIRunID                 string
	AttemptID               string
	Provider                string
	Model                   string
	InputTokens             int
	CachedTokens            int
	OutputTokens            int
	ToolCallsCount          int
	CatalogProjectionTokens int
	EstimatedCostMicros     int64
	Currency                string
	MeasuredAt              time.Time
	CreatedAt               time.Time
}

// AIStageLatencyRecord mirrors ai_stage_latencies row. Per contract ⑧ §9.
type AIStageLatencyRecord struct {
	ID         string
	AIRunID    string
	Stage      string
	LatencyMs  int
	MeasuredAt time.Time
	CreatedAt  time.Time
}

// ════════════════════════════════════════════════════════════════════════════
// Contract ⑨ §2 — AI Run Repository interfaces
// ════════════════════════════════════════════════════════════════════════════

// AIRunRepository persists operational AI Run trace records. Per contract ⑧,
// Trace records are tenant-scoped via business_id and never contain secrets.
//
// Per contract ⑨ §16, an AI Run MUST NOT be created twice for the same source
// event. The idempotency_key + business_id uniqueness enforces this at the DB
// level (see uq_ai_runs_idempotency index).
type AIRunRepository interface {
	CreateRun(ctx context.Context, run AIRunRecord) (AIRunRecord, error)
	GetRun(ctx context.Context, businessID, runID string) (AIRunRecord, error)
	GetByIdempotencyKey(ctx context.Context, businessID, idempotencyKey string) (AIRunRecord, error)
	UpdateRunStatus(ctx context.Context, businessID, runID string, patch AIRunStatusPatch) (AIRunRecord, error)
	ListRunsByConversation(ctx context.Context, businessID, conversationID string, limit int) ([]AIRunRecord, error)

	CreateAttempt(ctx context.Context, attempt AIRunAttemptRecord) (AIRunAttemptRecord, error)
	UpdateAttempt(ctx context.Context, attemptID string, patch AIRunAttemptPatch) (AIRunAttemptRecord, error)
	ListAttempts(ctx context.Context, runID string) ([]AIRunAttemptRecord, error)

	CreateToolCall(ctx context.Context, call AIToolCallRecord) (AIToolCallRecord, error)
	UpdateToolCall(ctx context.Context, callID string, patch AIToolCallPatch) (AIToolCallRecord, error)
	ListToolCalls(ctx context.Context, runID string) ([]AIToolCallRecord, error)

	CreateGeminiInteraction(ctx context.Context, interaction AIGeminiInteractionRecord) (AIGeminiInteractionRecord, error)
	ListGeminiInteractions(ctx context.Context, runID string) ([]AIGeminiInteractionRecord, error)

	CreateCatalogBatch(ctx context.Context, batch AICatalogBatchRecord) (AICatalogBatchRecord, error)
	UpdateCatalogBatch(ctx context.Context, batchID string, patch AICatalogBatchPatch) (AICatalogBatchRecord, error)
	ListCatalogBatches(ctx context.Context, runID string) ([]AICatalogBatchRecord, error)

	RecordUsage(ctx context.Context, usage AIUsageTelemetryRecord) (AIUsageTelemetryRecord, error)
	RecordStageLatency(ctx context.Context, latency AIStageLatencyRecord) (AIStageLatencyRecord, error)
}

// AIRunStatusPatch is the partial update for an AI Run.
type AIRunStatusPatch struct {
	Status                string
	FailureStage          *string
	FailureCategory       *string
	FailureReason         *string
	GeminiInteractionID   *string
	PreviousInteractionID *string

	ContextBuiltAt   *time.Time
	RunningStartedAt *time.Time
	WaitingToolAt    *time.Time
	ValidatingAt     *time.Time
	AuthorizedAt     *time.Time
	ExecutingAt      *time.Time
	CompletedAt      *time.Time
	FailedAt         *time.Time
	CancelledAt      *time.Time
}

// AIRunAttemptPatch is the partial update for an Attempt.
type AIRunAttemptPatch struct {
	Status          string
	FailureStage    *string
	FailureCategory *string
	FailureReason   *string
	FinishedAt      *time.Time
}

// AIToolCallPatch is the partial update for a Tool Call.
type AIToolCallPatch struct {
	Status        string
	ResultPayload []byte
	FailureReason *string
	FinishedAt    *time.Time
	LatencyMs     *int
}

// AICatalogBatchPatch is the partial update for a Catalog Batch.
type AICatalogBatchPatch struct {
	Status         string
	CandidateCount *int
	InputTokens    *int
	FailureReason  *string
	StartedAt      *time.Time
	CompletedAt    *time.Time
}

// AIRunLifecyclePort is the minimal lifecycle interface needed by the Gemini
// transition the AI Run lifecycle during the Tool Loop. Per fix #2:
// customer-sales adapter (in the Gemini package) cannot import services.AIRunLifecycle
// (would create a cycle). This abstraction lets the caller wire the
// existing lifecycle implementation without creating an import cycle.
//
// Per the spec: "استخدم الموجود: services.AIRunLifecycle"
// AIRunLifecycle already implements MarkWaitingTool + MarkRunning.
type AIRunLifecyclePort interface {
	MarkWaitingTool(ctx context.Context, businessID, runID string) (AIRunRecord, error)
	MarkRunning(ctx context.Context, businessID, runID string) (AIRunRecord, error)
}

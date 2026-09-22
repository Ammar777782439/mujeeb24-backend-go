// Package ports — AI Gemini Contract types aligned with contracts ①②③④⑤⑥⑧⑨.
//
// This file contains the contract-aligned types that supersedes the older mixing
// in ai_runtime.go. The old AIDecisionProposal field set mixed Mujeeb-side
// retrieval tracking (CatalogRetrievalState, CatalogStreams, CatalogIncomplete,
// SafetyBudgetExhausted, Discovered*Evidence) with Gemini's pure output.
//
// Per contract ④ §5, Gemini's output contract is ONLY:
//   status + action + response_text + selected[]
//
// Per contract ② §5, Gemini's batch-evaluation output is ONLY:
//   candidates[] (item_id, variant_id, offer_id, reason)
//
// Per contract ⑧ §5, the operational trace (Run, Attempt, Tool Call, Interaction,
// Usage, Latency) is in separate operational tables — never inside AI Proposal.
//
// Per contract ⑨ §27, AI Run vs Attempt are distinct concepts.
//
// Migration strategy:
//   - Old fields in AIDecisionProposal remain for now but are deprecated.
//   - New code should write/read only the contract-aligned fields defined here.
//   - A future commit will delete the deprecated fields once services migrate.

package ports

import (
	"context"
	"time"
)

// ════════════════════════════════════════════════════════════════════════════
// Contract ④ §4 — Gemini → Mujeeb Output Contract (Final Proposal)
// ════════════════════════════════════════════════════════════════════════════

// AIProposalStatus — the four closed status values per contract ④ §4.
//
// These are the ONLY allowed status values Gemini may return. Any other value
// fails Structural Validation per contract ⑥ §4.
type AIProposalStatus string

const (
	AIProposalStatusResolved      AIProposalStatus = "resolved"
	AIProposalStatusAmbiguous     AIProposalStatus = "ambiguous"
	AIProposalStatusNotFound      AIProposalStatus = "not_found"
	AIProposalStatusNeedsMoreData AIProposalStatus = "needs_more_data"
)

// AIProposalAction — the five closed action values per contract ④ §4.
//
// These are the ONLY allowed action values Gemini may return. Any other value
// fails Structural Validation per contract ⑥ §5.
//
// Note: contract ④ uses "human_request" (not "request_human"), and
// "clarification" (not "ask_clarification"), "lead_draft" (not "create_lead"),
// "order_draft" (not "create_transaction_draft"). Migration 000056 aligns the
// ai_decisions table accordingly.
type AIProposalAction string

const (
	AIProposalActionAnswer        AIProposalAction = "answer"
	AIProposalActionClarification AIProposalAction = "clarification"
	AIProposalActionHumanRequest  AIProposalAction = "human_request"
	AIProposalActionLeadDraft     AIProposalAction = "lead_draft"
	AIProposalActionOrderDraft    AIProposalAction = "order_draft"
)

// SelectedReference is a per-contract ④ §4 reference to a catalog entity that
// Gemini's proposal depends on. All three IDs are required to be UUIDs that
// were actually present in the evidence Mujeeb sent — never invented by Gemini.
//
// variant_id and offer_id may be nil when the relation does not apply or is
// not required (e.g., a service-type item with no variants).
type SelectedReference struct {
	ItemID    string  `json:"item_id"`
	VariantID *string `json:"variant_id,omitempty"`
	OfferID   *string `json:"offer_id,omitempty"`
}

// CatalogBatchCandidate is per-contract ② §5 — the per-batch evaluation result
// Gemini returns. Mujeeb already knows which items it sent, so Gemini only
// returns the IDs it considered candidates plus a short reason.
//
// variant_ids and offer_ids are plural because one item may produce multiple
// variants/offers that are all candidates.
type CatalogBatchCandidate struct {
	ItemID     string   `json:"item_id"`
	VariantIDs []string `json:"variant_ids,omitempty"`
	OfferIDs   []string `json:"offer_ids,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

// AIGeminiProposal is the contract ④ §4 final output of one Gemini interaction
// for one AI Run. It is the ONLY shape Gemini is allowed to return for the
// customer-facing decision.
//
// This type is distinct from the per-batch CatalogBatchResult (contract ② §5).
//
// Contract ④ §5 explicitly forbids these fields from appearing in Gemini output:
//   - business_id, tenant_id
//   - requires_approval, authorized, executed, sent
//   - payment_confirmed, order_created
//
// Such fields are Mujeeb's responsibility and live in EffectiveDecision.
type AIGeminiProposal struct {
	Status       AIProposalStatus    `json:"status"`
	Action       AIProposalAction    `json:"action"`
	ResponseText string              `json:"response_text"`
	Selected     []SelectedReference `json:"selected,omitempty"`
}

// CatalogBatchResult is the contract ② §5 per-batch evaluation output.
//
// Per contract ② §7, Gemini does NOT decide whether a batch was "skipped" or
// "completed". Mujeeb (the Catalog Batch Controller) owns coverage; Gemini only
// returns the candidates it inferred from the items in this batch.
type CatalogBatchResult struct {
	BatchNumber int                     `json:"batch_number"`
	Candidates  []CatalogBatchCandidate `json:"candidates"`
}

// ════════════════════════════════════════════════════════════════════════════
// Contract ③ §4 — Gemini Interaction Continuity
// ════════════════════════════════════════════════════════════════════════════

// GeminiInteractionContext carries the previous_interaction_id (if any) for
// the current customer turn. Mujeeb stores it on the conversation row; the
// AI Runtime passes it to the Gemini adapter which then sends it to the API.
//
// Per contract ③ §4, last_gemini_interaction_id is NOT the source of truth —
// Mujeeb's canonical conversation state is. This ID is only for Gemini's
// internal continuity; if Gemini is unavailable, Mujeeb does not lose any
// conversation, message, state, or business context.
type GeminiInteractionContext struct {
	// PreviousInteractionID is the gemini_interaction_id returned by the
	// previous successful customer-facing Gemini call for this conversation.
	// Empty for the first turn of a new conversation or after Gemini history
	// expiry (1 day free tier, 55 days paid tier per contract ③ §9).
	PreviousInteractionID string

	// ResultingInteractionID is populated by the AI Runtime after a successful
	// Gemini call. Mujeeb persists this as the new last_gemini_interaction_id
	// on the conversation row, to be used as PreviousInteractionID in the
	// next turn.
	ResultingInteractionID string

	// Store controls whether Gemini server-side history is used. Per contract
	// ③ §9, Mujeeb uses store=true to enable previous_interaction_id chaining.
	// When store=false, PreviousInteractionID MUST be empty and chaining is
	// disabled for that interaction.
	Store bool
}

// ════════════════════════════════════════════════════════════════════════════
// Contract ⑥ §17 — Effective Decision (post-validation, post-authorization)
// ════════════════════════════════════════════════════════════════════════════

// EffectiveDecision is the contract ⑥ §17 outcome after Structural → Reference
// → Tenant → Ownership → Policy → Authorization. It is distinct from AIGeminiProposal.
//
// Per contract ⑥ §22, no new "invalid/rejected/unauthorized" statuses are added
// to the AI Contract; those are internal layer resultss. The EffectiveDecision
// carries the authoritative action and the policy decision Mujeeb reached.
type EffectiveDecision struct {
	// DecisionID is the ai_decisions.id this Effective Decision is attached to.
	DecisionID string

	// EffectiveAction may differ from the AI proposal's action when policy or
	// authorization overrode it. For example, if Gemini proposed "answer" but
	// policy requires approval, EffectiveAction becomes "blocked" until
	// approval arrives, then transitions to the original action.
	//
	// Allowed values: same as AIProposalAction plus "no_action" and "blocked".
	EffectiveAction string

	// PolicyDecision is the PolicyEvaluator's verdict per contract ⑥ §12-13.
	//   allowed           — proceed without human approval
	//   requires_approval — handoff to human queue; do not execute yet
	//   denied            — do not execute at all
	PolicyDecision string

	// Reason captures the policy key or authorization reason that produced
	// this Effective Decision. Useful for audit and human review.
	Reason string

	// AuthorizedAt is when the Authorization step completed successfully.
	// Empty if PolicyDecision == "denied".
	AuthorizedAt *time.Time

	// ExecutedAt is when the Executor completed the authorized action.
	// Per contract ⑥ §14, Execution Result is tracked separately in
	// ai_runs.status (EXECUTING → COMPLETED/FAILED), not here.
	ExecutedAt *time.Time
}

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

// ════════════════════════════════════════════════════════════════════════════
// Contract ④ §8 — ContractRuntime interface (replaces legacy AIRuntime)
// ════════════════════════════════════════════════════════════════════════════

// ContractRuntime is the contract ④ §8 mapping of Mujeeb Contract to Gemini API.
//
// Per contract ④ §8:
//
//	Mujeeb System Contract → system_instruction
//	Mujeeb Input Context → input (contents)
//	Catalog boundary → Function Calling / tool
//	Mujeeb Output Contract → Structured Output (responseSchema)
//
// ContractRuntime replaces the legacy AIRuntime interface. New code MUST
// use ContractRuntime; the legacy AIRuntime is kept only for migration.
//
// Per contract ③ §4, this interface carries GeminiInteractionContext with
// previous_interaction_id chaining.
//
// Per contract ⑤ §7, the Catalog Entity Contract is sent as part of system
// instruction; it is passed through as opaque JSON.
type ContractRuntime interface {
	DecideContract(ctx context.Context, input ContractRuntimeInput) (ContractRuntimeOutput, error)
}

// ContractRuntimeInput is the input to ContractRuntime.DecideContract.
type ContractRuntimeInput struct {
	// DecisionInput carries business_id, conversation_id, message text, channel,
	// and the built AIContext (per contract ③ §2).
	DecisionInput AIDecisionInput

	// GeminiInteraction per contract ③ §4. Empty PreviousInteractionID means
	// this is the first turn (no chaining). ResultingInteractionID is populated
	// by the runtime after a successful Gemini call.
	GeminiInteraction GeminiInteractionContext

	// EntityContractPayload is the JSON-serializable Catalog Entity Contract
	// payload per contract ⑤ §7. Passed as raw bytes to avoid a circular
	// dependency between ports and services (where CatalogEntityContractPayload
	// is defined). The runtime passes it through to Gemini as system_instruction.
	EntityContractPayload []byte
}

// ContractRuntimeOutput is the output of ContractRuntime.DecideContract.
type ContractRuntimeOutput struct {
	// Proposal is the contract ④ §4 structured Gemini output.
	Proposal AIGeminiProposal

	// GeminiInteraction echoes the input and is populated with the
	// ResultingInteractionID returned by Gemini. Caller persists this as the
	// new last_gemini_interaction_id on the conversation row per contract ③ §4.
	GeminiInteraction GeminiInteractionContext

	// Usage per contract ⑧ §8 (token counts + estimated cost).
	Usage ContractUsageTelemetry

	// LatencyMs per contract ⑧ §9 (the Gemini API call latency).
	LatencyMs int64
}

// ContractUsageTelemetry is the per-call usage data per contract ⑧ §8.
type ContractUsageTelemetry struct {
	InputTokens         int
	CachedTokens        int
	OutputTokens        int
	Model               string
	EstimatedCostMicros int64
}

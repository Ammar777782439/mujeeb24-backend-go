package ports

import (
	"context"
	"time"
)

type AIDecisionRepository interface {
	CreateProposed(ctx context.Context, draft AIDecisionDraft) (AIDecisionRecord, error)
	List(ctx context.Context, filter AIDecisionFilter) (AIDecisionPage, error)
	Get(ctx context.Context, businessID, decisionID string) (AIDecisionRecord, error)
	RequestHumanReview(ctx context.Context, patch HumanReviewPatch) (AIDecisionRecord, error)
}

type AIDecisionDraft struct {
	ID                     string
	BusinessID             string
	ConversationID         *string
	SourceMessageReference *string
	IntentBase             string
	DomainContext          *string
	Entities               []byte
	EvidenceReferences     []byte
	RequestedAction        string
	ConfidenceValue        *string
	ConfidenceBand         string
	RequiresHuman          bool
	MissingInformation     []byte
	ReasonCodes            []byte
	PolicyReference        *string
	PolicyVersion          string
	KnowledgeVersion       *string
	ModelReference         *string
	SchemaVersion          int
	Lifecycle              string
	PolicyDecision         *string
	Outcome                *string
	ExecutionReference     *string
	CorrelationID          *string
	CausationID            *string
	ExpiresAt              *time.Time
	DecidedAt              *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
	// AIRunID links this business decision to the operational AI Run trace
	// (ai_runs.id) per contract ⑧ §5. Per contract ⑨ §1, an AI Run is the
	// operational trace; an ai_decisions row is the business decision. They
	// are distinct concepts; this column bridges them for traceability.
	AIRunID *string
	// EffectiveAction per contract ⑥ §17 — the final action after Policy +
	// Authorization. May differ from RequestedAction when policy overrides.
	EffectiveAction         *string
	EffectiveDecisionReason *string
	AuthorizedAt            *time.Time
	ExecutedAt              *time.Time
}

type AIDecisionFilter struct {
	BusinessID     string
	Lifecycle      string
	ConversationID string
	RequiresHuman  *bool
	Limit          int
	Cursor         string
}

type AIDecisionPage struct {
	Items      []AIDecisionRecord
	NextCursor string
	HasMore    bool
}

type AIDecisionRecord struct {
	ID                     string
	BusinessID             string
	ConversationID         *string
	SourceMessageReference *string
	IntentBase             string
	DomainContext          *string
	Entities               []byte
	EvidenceReferences     []byte
	RequestedAction        string
	ConfidenceValue        *string
	ConfidenceBand         string
	RequiresHuman          bool
	MissingInformation     []byte
	ReasonCodes            []byte
	PolicyReference        *string
	PolicyVersion          string
	KnowledgeVersion       *string
	ModelReference         *string
	SchemaVersion          int
	Lifecycle              string
	PolicyDecision         *string
	Outcome                *string
	ExecutionReference     *string
	CorrelationID          *string
	CausationID            *string
	ExpiresAt              *time.Time
	HumanReviewReason      *string
	HumanReviewRequestedAt *time.Time
	HumanReviewRequestedBy *string
	DecidedAt              *time.Time
	ResourceVersion        int64
	CreatedAt              time.Time
	UpdatedAt              time.Time
	// Per migration 000056 (contract ⑧ §5 + ⑥ §17):
	// AIRunID links this business decision to the operational AI Run trace
	// (ai_runs.id). Per contract ⑨ §1, an AI Run is the operational trace;
	// an ai_decisions row is the business decision. They are distinct
	// concepts; this column bridges them for traceability.
	AIRunID *string
	// EffectiveAction per contract ⑥ §17 — the final action after Policy +
	// Authorization. May differ from RequestedAction when policy overrides.
	// Per migration 000056 ai_decisions_effective_action_chk, allowed values:
	//   answer, clarification, human_request, lead_draft, order_draft,
	//   no_action, blocked.
	EffectiveAction *string
	// EffectiveDecisionReason per contract ⑥ §17 — the policy key or
	// authorization reason that produced this Effective Decision.
	EffectiveDecisionReason *string
	// AuthorizedAt per contract ⑥ §17 — when the Authorization step completed
	// successfully. Empty when PolicyDecision == "denied".
	AuthorizedAt *time.Time
	// ExecutedAt per contract ⑥ §19 — when the Executor completed the
	// authorized action. Per contract ⑨ §31, the Run-level execution result
	// is tracked separately in ai_runs.status (EXECUTING → COMPLETED/FAILED).
	ExecutedAt *time.Time
}

type HumanReviewPatch struct {
	BusinessID      string
	DecisionID      string
	Reason          string
	RequestedBy     string
	RequestedAt     time.Time
	ExpectedVersion int64
}

type AuditEventRepository interface {
	List(ctx context.Context, filter AuditEventFilter) (AuditEventPage, error)
	Get(ctx context.Context, businessID, eventID string) (AuditEventRecord, error)
	Append(ctx context.Context, draft AuditEventDraft) (AuditEventRecord, error)
}

type AuditEventFilter struct {
	BusinessID   string
	ActorType    string
	Action       string
	ResourceType string
	From         *time.Time
	Until        *time.Time
	Limit        int
	Cursor       string
}

type AuditEventPage struct {
	Items      []AuditEventRecord
	NextCursor string
	HasMore    bool
}

type AuditEventRecord struct {
	ID                string
	BusinessID        string
	ActorType         string
	ActorReference    *string
	Action            string
	ResourceType      string
	ResourceID        *string
	Metadata          []byte
	DecisionReference *string
	Result            *string
	ReasonCode        *string
	BeforeReference   *string
	AfterReference    *string
	CorrelationID     *string
	CausationID       *string
	OccurredAt        time.Time
	CreatedAt         time.Time
	SchemaVersion     int
	RedactionVersion  int
}

type AuditEventDraft struct {
	ID                string
	BusinessID        string
	ActorType         string
	ActorReference    *string
	Action            string
	ResourceType      string
	ResourceID        *string
	Metadata          []byte
	DecisionReference *string
	Result            *string
	ReasonCode        *string
	BeforeReference   *string
	AfterReference    *string
	CorrelationID     *string
	CausationID       *string
	OccurredAt        time.Time
	CreatedAt         time.Time
	SchemaVersion     int
	RedactionVersion  int
}

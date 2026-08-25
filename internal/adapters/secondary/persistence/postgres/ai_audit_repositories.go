package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type AIDecisionRepository struct{ adapter *Adapter }

func NewAIDecisionRepository(adapter *Adapter) *AIDecisionRepository {
	return &AIDecisionRepository{adapter: adapter}
}

const aiDecisionSelect = `SELECT id::text, business_id::text, conversation_id::text, source_message_reference, intent_base, domain_context, entities, evidence_references, requested_action, confidence_value::text, confidence_band, requires_human, missing_information, reason_codes, policy_reference, policy_version, knowledge_version, model_reference, schema_version, lifecycle, policy_decision, outcome, execution_reference, correlation_id::text, causation_id::text, expires_at, human_review_reason, human_review_requested_at, human_review_requested_by, decided_at, resource_version, created_at, updated_at FROM ai_decisions`

func (r *AIDecisionRepository) CreateProposed(ctx context.Context, draft ports.AIDecisionDraft) (ports.AIDecisionRecord, error) {
	executor, err := r.executor(ctx)
	if err != nil {
		return ports.AIDecisionRecord{}, err
	}
	if draft.ID == "" || draft.BusinessID == "" || draft.IntentBase == "" || draft.RequestedAction == "" || draft.ConfidenceBand == "" || draft.PolicyVersion == "" || draft.SchemaVersion <= 0 || draft.Lifecycle != "proposed" || draft.CreatedAt.IsZero() || draft.UpdatedAt.IsZero() {
		return ports.AIDecisionRecord{}, invalidRepositoryInput("ai_decision.create_proposed", "id, business, intent, action, confidence band, policy version, schema version, proposed lifecycle, and timestamps are required")
	}
	entities := draft.Entities
	if len(entities) == 0 {
		entities = []byte(`{}`)
	}
	evidence := draft.EvidenceReferences
	if len(evidence) == 0 {
		evidence = []byte(`[]`)
	}
	missing := draft.MissingInformation
	if len(missing) == 0 {
		missing = []byte(`[]`)
	}
	reasons := draft.ReasonCodes
	if len(reasons) == 0 {
		reasons = []byte(`[]`)
	}
	const query = `INSERT INTO ai_decisions (id, business_id, conversation_id, source_message_reference, intent_base, domain_context, entities, evidence_references, requested_action, confidence_value, confidence_band, requires_human, missing_information, reason_codes, policy_reference, policy_version, knowledge_version, model_reference, schema_version, lifecycle, policy_decision, outcome, execution_reference, correlation_id, causation_id, expires_at, decided_at, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7::jsonb, $8::jsonb, $9, $10::numeric, $11, $12, $13::jsonb, $14::jsonb, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24::uuid, $25::uuid, $26, $27, $28, $29) RETURNING id::text, business_id::text, conversation_id::text, source_message_reference, intent_base, domain_context, entities, evidence_references, requested_action, confidence_value::text, confidence_band, requires_human, missing_information, reason_codes, policy_reference, policy_version, knowledge_version, model_reference, schema_version, lifecycle, policy_decision, outcome, execution_reference, correlation_id::text, causation_id::text, expires_at, human_review_reason, human_review_requested_at, human_review_requested_by, decided_at, resource_version, created_at, updated_at`
	return scanAIDecision(executor.QueryRow(ctx, query, draft.ID, draft.BusinessID, draft.ConversationID, draft.SourceMessageReference, draft.IntentBase, draft.DomainContext, entities, evidence, draft.RequestedAction, draft.ConfidenceValue, draft.ConfidenceBand, draft.RequiresHuman, missing, reasons, draft.PolicyReference, draft.PolicyVersion, draft.KnowledgeVersion, draft.ModelReference, draft.SchemaVersion, draft.Lifecycle, draft.PolicyDecision, draft.Outcome, draft.ExecutionReference, draft.CorrelationID, draft.CausationID, draft.ExpiresAt, draft.DecidedAt, draft.CreatedAt, draft.UpdatedAt), "ai_decision.create_proposed")
}

func (r *AIDecisionRepository) List(ctx context.Context, filter ports.AIDecisionFilter) (ports.AIDecisionPage, error) {
	executor, err := r.executor(ctx)
	if err != nil {
		return ports.AIDecisionPage{}, err
	}
	limit, cursor, err := salesPageArgs(filter.Limit, filter.Cursor, "ai_decision.list")
	if err != nil {
		return ports.AIDecisionPage{}, err
	}
	var cursorAt any
	var cursorID any
	if cursor != nil {
		cursorAt, cursorID = cursor.At, cursor.ID
	}
	var conversationID any
	if filter.ConversationID != "" {
		conversationID = filter.ConversationID
	}
	rows, err := executor.Query(ctx, aiDecisionSelect+` WHERE business_id = $1::uuid AND ($2 = '' OR lifecycle = $2) AND ($3::uuid IS NULL OR conversation_id = $3::uuid) AND ($4::bool IS NULL OR requires_human = $4::bool) AND ($5::timestamptz IS NULL OR (created_at, id) < ($5::timestamptz, $6::uuid)) ORDER BY created_at DESC, id DESC LIMIT $7`, filter.BusinessID, filter.Lifecycle, conversationID, filter.RequiresHuman, cursorAt, cursorID, limit+1)
	if err != nil {
		return ports.AIDecisionPage{}, salesRepositoryError("ai_decision.list", err)
	}
	defer rows.Close()
	items := make([]ports.AIDecisionRecord, 0, limit)
	for rows.Next() {
		item, scanErr := scanAIDecision(rows, "ai_decision.list")
		if scanErr != nil {
			return ports.AIDecisionPage{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.AIDecisionPage{}, salesRepositoryError("ai_decision.list", err)
	}
	page := ports.AIDecisionPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeSalesCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

func (r *AIDecisionRepository) Get(ctx context.Context, businessID, decisionID string) (ports.AIDecisionRecord, error) {
	executor, err := r.executor(ctx)
	if err != nil {
		return ports.AIDecisionRecord{}, err
	}
	if businessID == "" || decisionID == "" {
		return ports.AIDecisionRecord{}, invalidRepositoryInput("ai_decision.get", "business and decision ids are required")
	}
	return scanAIDecision(executor.QueryRow(ctx, aiDecisionSelect+` WHERE business_id = $1::uuid AND id = $2::uuid`, businessID, decisionID), "ai_decision.get")
}

func (r *AIDecisionRepository) RequestHumanReview(ctx context.Context, patch ports.HumanReviewPatch) (ports.AIDecisionRecord, error) {
	executor, err := r.executor(ctx)
	if err != nil {
		return ports.AIDecisionRecord{}, err
	}
	if patch.BusinessID == "" || patch.DecisionID == "" || patch.RequestedBy == "" || patch.Reason == "" || patch.ExpectedVersion <= 0 || patch.RequestedAt.IsZero() {
		return ports.AIDecisionRecord{}, invalidRepositoryInput("ai_decision.request_human", "business, decision, reason, requester, timestamp, and positive expected version are required")
	}
	record, err := scanAIDecision(executor.QueryRow(ctx, `UPDATE ai_decisions SET requires_human = TRUE, human_review_reason = $3, human_review_requested_at = $4, human_review_requested_by = $5, resource_version = resource_version + 1, updated_at = $4 WHERE business_id = $1::uuid AND id = $2::uuid AND resource_version = $6 AND lifecycle NOT IN ('expired', 'rejected') RETURNING id::text, business_id::text, conversation_id::text, source_message_reference, intent_base, domain_context, entities, evidence_references, requested_action, confidence_value::text, confidence_band, requires_human, missing_information, reason_codes, policy_reference, policy_version, knowledge_version, model_reference, schema_version, lifecycle, policy_decision, outcome, execution_reference, correlation_id::text, causation_id::text, expires_at, human_review_reason, human_review_requested_at, human_review_requested_by, decided_at, resource_version, created_at, updated_at`, patch.BusinessID, patch.DecisionID, patch.Reason, patch.RequestedAt, patch.RequestedBy, patch.ExpectedVersion), "ai_decision.request_human")
	if err == nil {
		return record, nil
	}
	return record, classifyAIDecisionMutationMiss(ctx, executor, patch, err)
}

func (r *AIDecisionRepository) executor(ctx context.Context) (SQLExecutor, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	return r.adapter.Executor(ctx)
}

type AuditEventRepository struct{ adapter *Adapter }

func NewAuditEventRepository(adapter *Adapter) *AuditEventRepository {
	return &AuditEventRepository{adapter: adapter}
}

const auditEventSelect = `SELECT id::text, business_id::text, actor_type, actor_reference, action, resource_type, resource_id, metadata, decision_reference, result, reason_code, before_reference, after_reference, correlation_id::text, causation_id::text, occurred_at, created_at, schema_version, redaction_version FROM audit_events`

func (r *AuditEventRepository) List(ctx context.Context, filter ports.AuditEventFilter) (ports.AuditEventPage, error) {
	executor, err := r.executor(ctx)
	if err != nil {
		return ports.AuditEventPage{}, err
	}
	limit, cursor, err := salesPageArgs(filter.Limit, filter.Cursor, "audit_event.list")
	if err != nil {
		return ports.AuditEventPage{}, err
	}
	var cursorAt any
	var cursorID any
	if cursor != nil {
		cursorAt, cursorID = cursor.At, cursor.ID
	}
	rows, err := executor.Query(ctx, auditEventSelect+` WHERE business_id = $1::uuid AND ($2 = '' OR actor_type = $2) AND ($3 = '' OR action = $3) AND ($4 = '' OR resource_type = $4) AND ($5::timestamptz IS NULL OR occurred_at >= $5::timestamptz) AND ($6::timestamptz IS NULL OR occurred_at <= $6::timestamptz) AND ($7::timestamptz IS NULL OR (occurred_at, id) < ($7::timestamptz, $8::uuid)) ORDER BY occurred_at DESC, id DESC LIMIT $9`, filter.BusinessID, filter.ActorType, filter.Action, filter.ResourceType, filter.From, filter.Until, cursorAt, cursorID, limit+1)
	if err != nil {
		return ports.AuditEventPage{}, salesRepositoryError("audit_event.list", err)
	}
	defer rows.Close()
	items := make([]ports.AuditEventRecord, 0, limit)
	for rows.Next() {
		item, scanErr := scanAuditEvent(rows, "audit_event.list")
		if scanErr != nil {
			return ports.AuditEventPage{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.AuditEventPage{}, salesRepositoryError("audit_event.list", err)
	}
	page := ports.AuditEventPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = encodeSalesCursor(last.OccurredAt, last.ID)
	}
	return page, nil
}

func (r *AuditEventRepository) Get(ctx context.Context, businessID, eventID string) (ports.AuditEventRecord, error) {
	executor, err := r.executor(ctx)
	if err != nil {
		return ports.AuditEventRecord{}, err
	}
	if businessID == "" || eventID == "" {
		return ports.AuditEventRecord{}, invalidRepositoryInput("audit_event.get", "business and event ids are required")
	}
	return scanAuditEvent(executor.QueryRow(ctx, auditEventSelect+` WHERE business_id = $1::uuid AND id = $2::uuid`, businessID, eventID), "audit_event.get")
}

func (r *AuditEventRepository) Append(ctx context.Context, draft ports.AuditEventDraft) (ports.AuditEventRecord, error) {
	executor, err := r.executor(ctx)
	if err != nil {
		return ports.AuditEventRecord{}, err
	}
	if draft.ID == "" || draft.BusinessID == "" || draft.ActorType == "" || draft.Action == "" || draft.ResourceType == "" || draft.OccurredAt.IsZero() || draft.CreatedAt.IsZero() || draft.SchemaVersion <= 0 || draft.RedactionVersion <= 0 {
		return ports.AuditEventRecord{}, invalidRepositoryInput("audit_event.append", "id, business, actor, action, resource, timestamps, and positive versions are required")
	}
	metadata := draft.Metadata
	if len(metadata) == 0 {
		metadata = []byte(`{}`)
	}
	return scanAuditEvent(executor.QueryRow(ctx, `INSERT INTO audit_events (id, business_id, actor_type, actor_reference, action, resource_type, resource_id, metadata, decision_reference, result, reason_code, before_reference, after_reference, correlation_id, causation_id, occurred_at, created_at, schema_version, redaction_version) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, $11, $12, $13, $14::uuid, $15::uuid, $16, $17, $18, $19) RETURNING id::text, business_id::text, actor_type, actor_reference, action, resource_type, resource_id, metadata, decision_reference, result, reason_code, before_reference, after_reference, correlation_id::text, causation_id::text, occurred_at, created_at, schema_version, redaction_version`, draft.ID, draft.BusinessID, draft.ActorType, draft.ActorReference, draft.Action, draft.ResourceType, draft.ResourceID, metadata, draft.DecisionReference, draft.Result, draft.ReasonCode, draft.BeforeReference, draft.AfterReference, draft.CorrelationID, draft.CausationID, draft.OccurredAt, draft.CreatedAt, draft.SchemaVersion, draft.RedactionVersion), "audit_event.append")
}

func (r *AuditEventRepository) executor(ctx context.Context) (SQLExecutor, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	return r.adapter.Executor(ctx)
}

func scanAIDecision(row interface{ Scan(...any) error }, operation string) (ports.AIDecisionRecord, error) {
	var item ports.AIDecisionRecord
	err := row.Scan(&item.ID, &item.BusinessID, &item.ConversationID, &item.SourceMessageReference, &item.IntentBase, &item.DomainContext, &item.Entities, &item.EvidenceReferences, &item.RequestedAction, &item.ConfidenceValue, &item.ConfidenceBand, &item.RequiresHuman, &item.MissingInformation, &item.ReasonCodes, &item.PolicyReference, &item.PolicyVersion, &item.KnowledgeVersion, &item.ModelReference, &item.SchemaVersion, &item.Lifecycle, &item.PolicyDecision, &item.Outcome, &item.ExecutionReference, &item.CorrelationID, &item.CausationID, &item.ExpiresAt, &item.HumanReviewReason, &item.HumanReviewRequestedAt, &item.HumanReviewRequestedBy, &item.DecidedAt, &item.ResourceVersion, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, classifyRepositoryGetError(operation, err)
	}
	return item, nil
}

func scanAuditEvent(row interface{ Scan(...any) error }, operation string) (ports.AuditEventRecord, error) {
	var item ports.AuditEventRecord
	err := row.Scan(&item.ID, &item.BusinessID, &item.ActorType, &item.ActorReference, &item.Action, &item.ResourceType, &item.ResourceID, &item.Metadata, &item.DecisionReference, &item.Result, &item.ReasonCode, &item.BeforeReference, &item.AfterReference, &item.CorrelationID, &item.CausationID, &item.OccurredAt, &item.CreatedAt, &item.SchemaVersion, &item.RedactionVersion)
	if err != nil {
		return item, classifyRepositoryGetError(operation, err)
	}
	return item, nil
}

func classifyAIDecisionMutationMiss(ctx context.Context, executor SQLExecutor, patch ports.HumanReviewPatch, original error) error {
	if !errors.Is(original, pgx.ErrNoRows) {
		return salesRepositoryError("ai_decision.request_human", original)
	}
	var currentVersion int64
	var lifecycle string
	if err := executor.QueryRow(ctx, `SELECT resource_version, lifecycle FROM ai_decisions WHERE business_id = $1::uuid AND id = $2::uuid`, patch.BusinessID, patch.DecisionID).Scan(&currentVersion, &lifecycle); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &RepositoryError{Operation: "ai_decision.request_human", Kind: RepositoryNotFound, Err: err}
		}
		return salesRepositoryError("ai_decision.request_human", err)
	}
	if currentVersion != patch.ExpectedVersion {
		return &RepositoryError{Operation: "ai_decision.request_human", Kind: RepositoryStale, Err: fmt.Errorf("expected version %d, current version %d", patch.ExpectedVersion, currentVersion)}
	}
	return &RepositoryError{Operation: "ai_decision.request_human", Kind: RepositoryConflict, Err: fmt.Errorf("decision lifecycle %s does not accept human review", lifecycle)}
}

var _ ports.AIDecisionRepository = (*AIDecisionRepository)(nil)
var _ ports.AuditEventRepository = (*AuditEventRepository)(nil)

package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type LeadRepository struct{ adapter *Adapter }

func NewLeadRepository(adapter *Adapter) *LeadRepository { return &LeadRepository{adapter: adapter} }

type salesCursor struct {
	At time.Time
	ID string
}

func (r *LeadRepository) List(ctx context.Context, businessID, status, customerID, scoreBand string, limit int, cursor string) (ports.LeadPage, error) {
	executor, err := r.salesExecutor(ctx, "lead.list")
	if err != nil {
		return ports.LeadPage{}, err
	}
	limit, decoded, err := salesPageArgs(limit, cursor, "lead.list")
	if err != nil {
		return ports.LeadPage{}, err
	}
	var at any
	var id any
	if decoded != nil {
		at, id = decoded.At, decoded.ID
	}
	rows, err := executor.Query(ctx, `SELECT id::text, business_id::text, customer_id::text, status, qualification_state, current_score_value::text, current_score_band, score_rule_version, score_model_reference, qualification_context, created_by, qualified_by, qualification_reason, qualification_evidence, lost_reason, source_conversation_reference_id::text, source_channel, intent_reference, assigned_ownership_reference, next_action_at, resource_version, created_at, updated_at FROM leads WHERE business_id = $1::uuid AND ($2 = '' OR status = $2) AND ($3 = '' OR customer_id = $3::uuid) AND ($4 = '' OR current_score_band = $4) AND ($5::timestamptz IS NULL OR (updated_at, id) < ($5::timestamptz, $6::uuid)) ORDER BY updated_at DESC, id DESC LIMIT $7`, businessID, status, customerID, scoreBand, at, id, limit+1)
	if err != nil {
		return ports.LeadPage{}, salesRepositoryError("lead.list", err)
	}
	defer rows.Close()
	items := make([]ports.LeadRecord, 0, limit)
	for rows.Next() {
		item, scanErr := scanLead(rows)
		if scanErr != nil {
			return ports.LeadPage{}, salesRepositoryError("lead.list", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.LeadPage{}, salesRepositoryError("lead.list", err)
	}
	page := ports.LeadPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		page.NextCursor = encodeSalesCursor(page.Items[len(page.Items)-1].UpdatedAt, page.Items[len(page.Items)-1].ID)
	}
	return page, nil
}

func (r *LeadRepository) Get(ctx context.Context, businessID, leadID string) (ports.LeadRecord, error) {
	executor, err := r.salesExecutor(ctx, "lead.get")
	if err != nil {
		return ports.LeadRecord{}, err
	}
	if businessID == "" || leadID == "" {
		return ports.LeadRecord{}, invalidRepositoryInput("lead.get", "business and lead ids are required")
	}
	return scanLead(executor.QueryRow(ctx, `SELECT id::text, business_id::text, customer_id::text, status, qualification_state, current_score_value::text, current_score_band, score_rule_version, score_model_reference, qualification_context, created_by, qualified_by, qualification_reason, qualification_evidence, lost_reason, source_conversation_reference_id::text, source_channel, intent_reference, assigned_ownership_reference, next_action_at, resource_version, created_at, updated_at FROM leads WHERE business_id = $1::uuid AND id = $2::uuid`, businessID, leadID))
}

func (r *LeadRepository) Create(ctx context.Context, draft ports.LeadDraft) (ports.LeadRecord, error) {
	executor, err := r.salesExecutor(ctx, "lead.create")
	if err != nil {
		return ports.LeadRecord{}, err
	}
	if draft.ID == "" || draft.BusinessID == "" || draft.CustomerID == "" || draft.CreatedBy == "" || draft.CreatedAt.IsZero() || draft.UpdatedAt.IsZero() {
		return ports.LeadRecord{}, invalidRepositoryInput("lead.create", "required lead fields are missing")
	}
	contextJSON := draft.QualificationContext
	if len(contextJSON) == 0 {
		contextJSON = []byte(`{}`)
	}
	return scanLead(executor.QueryRow(ctx, `INSERT INTO leads (id, business_id, customer_id, status, qualification_state, qualification_context, created_by, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'new', 'new', $4::jsonb, $5, $6, $7) RETURNING id::text, business_id::text, customer_id::text, status, qualification_state, current_score_value::text, current_score_band, score_rule_version, score_model_reference, qualification_context, created_by, qualified_by, qualification_reason, qualification_evidence, lost_reason, source_conversation_reference_id::text, source_channel, intent_reference, assigned_ownership_reference, next_action_at, resource_version, created_at, updated_at`, draft.ID, draft.BusinessID, draft.CustomerID, contextJSON, draft.CreatedBy, draft.CreatedAt, draft.UpdatedAt))
}

func (r *LeadRepository) Update(ctx context.Context, patch ports.LeadPatch) (ports.LeadRecord, error) {
	executor, err := r.salesExecutor(ctx, "lead.update")
	if err != nil {
		return ports.LeadRecord{}, err
	}
	if patch.ID == "" || patch.BusinessID == "" || patch.ExpectedVersion <= 0 {
		return ports.LeadRecord{}, invalidRepositoryInput("lead.update", "id, business, and positive expected version are required")
	}
	contextJSON := patch.QualificationContext
	if len(contextJSON) == 0 {
		contextJSON = nil
	}
	var record ports.LeadRecord
	err = executor.QueryRow(ctx, `UPDATE leads SET qualification_context = COALESCE($3::jsonb, qualification_context), resource_version = resource_version + 1, updated_at = $4 WHERE business_id = $1::uuid AND id = $2::uuid AND resource_version = $5 RETURNING id::text, business_id::text, customer_id::text, status, qualification_state, current_score_value::text, current_score_band, score_rule_version, score_model_reference, qualification_context, created_by, qualified_by, qualification_reason, qualification_evidence, lost_reason, source_conversation_reference_id::text, source_channel, intent_reference, assigned_ownership_reference, next_action_at, resource_version, created_at, updated_at`, patch.BusinessID, patch.ID, contextJSON, patch.UpdatedAt, patch.ExpectedVersion).Scan(&record.ID, &record.BusinessID, &record.CustomerID, &record.Status, &record.QualificationState, &record.CurrentScoreValue, &record.CurrentScoreBand, &record.ScoreRuleVersion, &record.ScoreModelReference, &record.QualificationContext, &record.CreatedBy, &record.QualifiedBy, &record.QualificationReason, &record.QualificationEvidence, &record.LostReason, &record.SourceConversationReferenceID, &record.SourceChannel, &record.IntentReference, &record.AssignedOwnershipReference, &record.NextActionAt, &record.ResourceVersion, &record.CreatedAt, &record.UpdatedAt)
	if err == nil {
		return record, nil
	}
	return record, classifyLeadUpdateMiss(ctx, executor, "lead.update", patch.BusinessID, patch.ID, patch.ExpectedVersion, err)
}

func (r *LeadRepository) Qualify(ctx context.Context, patch ports.LeadQualificationPatch) (ports.LeadRecord, error) {
	executor, err := r.salesExecutor(ctx, "lead.qualify")
	if err != nil {
		return ports.LeadRecord{}, err
	}
	if patch.ID == "" || patch.BusinessID == "" || patch.QualifiedBy == "" || patch.ExpectedVersion <= 0 {
		return ports.LeadRecord{}, invalidRepositoryInput("lead.qualify", "id, business, actor, and positive expected version are required")
	}
	var record ports.LeadRecord
	err = executor.QueryRow(ctx, `UPDATE leads SET status = 'qualified', qualification_state = 'qualified', qualified_by = $3, qualification_reason = NULLIF($4, ''), qualification_evidence = COALESCE($5::jsonb, '[]'::jsonb), resource_version = resource_version + 1, updated_at = $6 WHERE business_id = $1::uuid AND id = $2::uuid AND resource_version = $7 AND status IN ('new', 'interested') RETURNING id::text, business_id::text, customer_id::text, status, qualification_state, current_score_value::text, current_score_band, score_rule_version, score_model_reference, qualification_context, created_by, qualified_by, qualification_reason, qualification_evidence, lost_reason, source_conversation_reference_id::text, source_channel, intent_reference, assigned_ownership_reference, next_action_at, resource_version, created_at, updated_at`, patch.BusinessID, patch.ID, patch.QualifiedBy, patch.QualificationReason, patch.QualificationEvidence, patch.UpdatedAt, patch.ExpectedVersion).Scan(&record.ID, &record.BusinessID, &record.CustomerID, &record.Status, &record.QualificationState, &record.CurrentScoreValue, &record.CurrentScoreBand, &record.ScoreRuleVersion, &record.ScoreModelReference, &record.QualificationContext, &record.CreatedBy, &record.QualifiedBy, &record.QualificationReason, &record.QualificationEvidence, &record.LostReason, &record.SourceConversationReferenceID, &record.SourceChannel, &record.IntentReference, &record.AssignedOwnershipReference, &record.NextActionAt, &record.ResourceVersion, &record.CreatedAt, &record.UpdatedAt)
	if err == nil {
		return record, nil
	}
	return record, classifyLeadTransitionMiss(ctx, executor, "lead.qualify", patch.BusinessID, patch.ID, patch.ExpectedVersion, err)
}

func (r *LeadRepository) MarkLost(ctx context.Context, patch ports.LeadLostPatch) (ports.LeadRecord, error) {
	executor, err := r.salesExecutor(ctx, "lead.mark_lost")
	if err != nil {
		return ports.LeadRecord{}, err
	}
	if patch.ID == "" || patch.BusinessID == "" || strings.TrimSpace(patch.LostReason) == "" || patch.ExpectedVersion <= 0 {
		return ports.LeadRecord{}, invalidRepositoryInput("lead.mark_lost", "id, business, reason, and positive expected version are required")
	}
	var record ports.LeadRecord
	err = executor.QueryRow(ctx, `UPDATE leads SET status = 'lost', qualification_state = 'lost', lost_reason = $3, resource_version = resource_version + 1, updated_at = $4 WHERE business_id = $1::uuid AND id = $2::uuid AND resource_version = $5 AND status NOT IN ('lost', 'won') RETURNING id::text, business_id::text, customer_id::text, status, qualification_state, current_score_value::text, current_score_band, score_rule_version, score_model_reference, qualification_context, created_by, qualified_by, qualification_reason, qualification_evidence, lost_reason, source_conversation_reference_id::text, source_channel, intent_reference, assigned_ownership_reference, next_action_at, resource_version, created_at, updated_at`, patch.BusinessID, patch.ID, patch.LostReason, patch.UpdatedAt, patch.ExpectedVersion).Scan(&record.ID, &record.BusinessID, &record.CustomerID, &record.Status, &record.QualificationState, &record.CurrentScoreValue, &record.CurrentScoreBand, &record.ScoreRuleVersion, &record.ScoreModelReference, &record.QualificationContext, &record.CreatedBy, &record.QualifiedBy, &record.QualificationReason, &record.QualificationEvidence, &record.LostReason, &record.SourceConversationReferenceID, &record.SourceChannel, &record.IntentReference, &record.AssignedOwnershipReference, &record.NextActionAt, &record.ResourceVersion, &record.CreatedAt, &record.UpdatedAt)
	if err == nil {
		return record, nil
	}
	return record, classifyLeadTransitionMiss(ctx, executor, "lead.mark_lost", patch.BusinessID, patch.ID, patch.ExpectedVersion, err)
}

func (r *LeadRepository) ListAttributions(ctx context.Context, businessID, leadID string, limit int, cursor string) (ports.LeadAttributionPage, error) {
	executor, err := r.salesExecutor(ctx, "lead.attribution.list")
	if err != nil {
		return ports.LeadAttributionPage{}, err
	}
	if err := ensureSalesParent(ctx, executor, "lead.attribution.list", `SELECT EXISTS (SELECT 1 FROM leads WHERE business_id = $1::uuid AND id = $2::uuid)`, businessID, leadID); err != nil {
		return ports.LeadAttributionPage{}, err
	}
	limit, decoded, err := salesPageArgs(limit, cursor, "lead.attribution.list")
	if err != nil {
		return ports.LeadAttributionPage{}, err
	}
	var at any
	var id any
	if decoded != nil {
		at, id = decoded.At, decoded.ID
	}
	rows, err := executor.Query(ctx, `SELECT id::text, business_id::text, lead_id::text, source_conversation_id::text, source_channel, source_interaction_reference, catalog_item_id::text, offer_id::text, campaign_reference, captured_at FROM lead_attributions WHERE business_id = $1::uuid AND lead_id = $2::uuid AND ($3::timestamptz IS NULL OR (captured_at, id) < ($3::timestamptz, $4::uuid)) ORDER BY captured_at DESC, id DESC LIMIT $5`, businessID, leadID, at, id, limit+1)
	if err != nil {
		return ports.LeadAttributionPage{}, salesRepositoryError("lead.attribution.list", err)
	}
	defer rows.Close()
	items := make([]ports.LeadAttributionRecord, 0, limit)
	for rows.Next() {
		var item ports.LeadAttributionRecord
		if err := rows.Scan(&item.ID, &item.BusinessID, &item.LeadID, &item.SourceConversationID, &item.SourceChannel, &item.SourceInteractionReference, &item.CatalogItemID, &item.OfferID, &item.CampaignReference, &item.CapturedAt); err != nil {
			return ports.LeadAttributionPage{}, salesRepositoryError("lead.attribution.list", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.LeadAttributionPage{}, salesRepositoryError("lead.attribution.list", err)
	}
	page := ports.LeadAttributionPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		page.NextCursor = encodeSalesCursor(page.Items[len(page.Items)-1].CapturedAt, page.Items[len(page.Items)-1].ID)
	}
	return page, nil
}

func (r *LeadRepository) ListScores(ctx context.Context, businessID, leadID string, limit int, cursor string) (ports.LeadScorePage, error) {
	executor, err := r.salesExecutor(ctx, "lead.score.list")
	if err != nil {
		return ports.LeadScorePage{}, err
	}
	if err := ensureSalesParent(ctx, executor, "lead.score.list", `SELECT EXISTS (SELECT 1 FROM leads WHERE business_id = $1::uuid AND id = $2::uuid)`, businessID, leadID); err != nil {
		return ports.LeadScorePage{}, err
	}
	limit, decoded, err := salesPageArgs(limit, cursor, "lead.score.list")
	if err != nil {
		return ports.LeadScorePage{}, err
	}
	var at any
	var id any
	if decoded != nil {
		at, id = decoded.At, decoded.ID
	}
	rows, err := executor.Query(ctx, `SELECT id::text, business_id::text, lead_id::text, value::text, band, factors, rule_version, model_reference, calculated_at, created_at FROM lead_scores WHERE business_id = $1::uuid AND lead_id = $2::uuid AND ($3::timestamptz IS NULL OR (calculated_at, id) < ($3::timestamptz, $4::uuid)) ORDER BY calculated_at DESC, id DESC LIMIT $5`, businessID, leadID, at, id, limit+1)
	if err != nil {
		return ports.LeadScorePage{}, salesRepositoryError("lead.score.list", err)
	}
	defer rows.Close()
	items := make([]ports.LeadScoreRecord, 0, limit)
	for rows.Next() {
		var item ports.LeadScoreRecord
		if err := rows.Scan(&item.ID, &item.BusinessID, &item.LeadID, &item.Value, &item.Band, &item.Factors, &item.RuleVersion, &item.ModelReference, &item.CalculatedAt, &item.CreatedAt); err != nil {
			return ports.LeadScorePage{}, salesRepositoryError("lead.score.list", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.LeadScorePage{}, salesRepositoryError("lead.score.list", err)
	}
	page := ports.LeadScorePage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		page.NextCursor = encodeSalesCursor(page.Items[len(page.Items)-1].CalculatedAt, page.Items[len(page.Items)-1].ID)
	}
	return page, nil
}

func scanLead(row interface{ Scan(...any) error }) (ports.LeadRecord, error) {
	var record ports.LeadRecord
	err := row.Scan(&record.ID, &record.BusinessID, &record.CustomerID, &record.Status, &record.QualificationState, &record.CurrentScoreValue, &record.CurrentScoreBand, &record.ScoreRuleVersion, &record.ScoreModelReference, &record.QualificationContext, &record.CreatedBy, &record.QualifiedBy, &record.QualificationReason, &record.QualificationEvidence, &record.LostReason, &record.SourceConversationReferenceID, &record.SourceChannel, &record.IntentReference, &record.AssignedOwnershipReference, &record.NextActionAt, &record.ResourceVersion, &record.CreatedAt, &record.UpdatedAt)
	if err == nil {
		return record, nil
	}
	return record, classifyRepositoryWriteError("lead", err)
}

func (r *LeadRepository) salesExecutor(ctx context.Context, operation string) (SQLExecutor, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	return r.adapter.Executor(ctx)
}

func ensureSalesParent(ctx context.Context, executor SQLExecutor, operation, query, businessID, id string) error {
	if businessID == "" || id == "" {
		return invalidRepositoryInput(operation, "business and parent ids are required")
	}
	var exists bool
	if err := executor.QueryRow(ctx, query, businessID, id).Scan(&exists); err != nil {
		return salesRepositoryError(operation, err)
	}
	if !exists {
		return &RepositoryError{Operation: operation, Kind: RepositoryNotFound, Err: pgx.ErrNoRows}
	}
	return nil
}

func salesPageArgs(limit int, cursor, operation string) (int, *salesCursor, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 10000 {
		limit = 100
	}
	if cursor == "" {
		return limit, nil, nil
	}
	decoded, err := decodeSalesCursor(cursor)
	if err != nil {
		return 0, nil, &RepositoryError{Operation: operation, Kind: RepositoryInvalid, Err: err}
	}
	return limit, decoded, nil
}

func encodeSalesCursor(at time.Time, id string) string {
	payload := at.UTC().Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func decodeSalesCursor(cursor string) (*salesCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor encoding: %w", err)
	}
	parts := strings.Split(string(payload), "|")
	if len(parts) != 2 || parts[1] == "" {
		return nil, errors.New("invalid cursor shape")
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid cursor timestamp: %w", err)
	}
	return &salesCursor{At: at, ID: parts[1]}, nil
}

func classifyLeadUpdateMiss(ctx context.Context, executor SQLExecutor, operation, businessID, leadID string, expected int64, original error) error {
	if !errors.Is(original, pgx.ErrNoRows) {
		return classifyRepositoryWriteError(operation, original)
	}
	var currentVersion int64
	if err := executor.QueryRow(ctx, `SELECT resource_version FROM leads WHERE business_id = $1::uuid AND id = $2::uuid`, businessID, leadID).Scan(&currentVersion); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &RepositoryError{Operation: operation, Kind: RepositoryNotFound, Err: err}
		}
		return classifyRepositoryWriteError(operation, err)
	}
	if currentVersion != expected {
		return &RepositoryError{Operation: operation, Kind: RepositoryStale, Err: fmt.Errorf("expected version %d, current version %d", expected, currentVersion)}
	}
	return &RepositoryError{Operation: operation, Kind: RepositoryConflict, Err: errors.New("lead update was rejected")}
}

func classifyLeadTransitionMiss(ctx context.Context, executor SQLExecutor, operation, businessID, leadID string, expected int64, original error) error {
	if !errors.Is(original, pgx.ErrNoRows) {
		return classifyRepositoryWriteError(operation, original)
	}
	var currentVersion int64
	var status string
	if err := executor.QueryRow(ctx, `SELECT resource_version, status FROM leads WHERE business_id = $1::uuid AND id = $2::uuid`, businessID, leadID).Scan(&currentVersion, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &RepositoryError{Operation: operation, Kind: RepositoryNotFound, Err: err}
		}
		return classifyRepositoryWriteError(operation, err)
	}
	if currentVersion != expected {
		return &RepositoryError{Operation: operation, Kind: RepositoryStale, Err: fmt.Errorf("expected version %d, current version %d", expected, currentVersion)}
	}
	return &RepositoryError{Operation: operation, Kind: RepositoryConflict, Err: fmt.Errorf("lead status %s cannot perform transition", status)}
}

func salesRepositoryError(operation string, err error) error {
	return classifyRepositoryWriteError(operation, err)
}

var _ ports.LeadRepository = (*LeadRepository)(nil)

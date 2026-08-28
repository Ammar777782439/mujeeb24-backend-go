package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AutomationRuleRepository struct{ adapter *Adapter }

func NewAutomationRuleRepository(adapter *Adapter) *AutomationRuleRepository {
	return &AutomationRuleRepository{adapter: adapter}
}

type automationRuleCursor struct {
	Position int
	ID       string
}

func (r *AutomationRuleRepository) List(ctx context.Context, businessID, status string, limit int, cursor string) (ports.AutomationRulePage, error) {
	if r == nil || r.adapter == nil {
		return ports.AutomationRulePage{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" {
		return ports.AutomationRulePage{}, invalidRepositoryInput("automation_rule.list", "business id is required")
	}
	if status != "" && status != "active" && status != "disabled" {
		return ports.AutomationRulePage{}, invalidRepositoryInput("automation_rule.list", "status must be active or disabled")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		return ports.AutomationRulePage{}, invalidRepositoryInput("automation_rule.list", "limit must not exceed 100")
	}
	decoded, err := decodeAutomationRuleCursor(cursor)
	if err != nil {
		return ports.AutomationRulePage{}, invalidRepositoryInput("automation_rule.list", err.Error())
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AutomationRulePage{}, err
	}
	var position, id any
	if decoded != nil {
		position, id = decoded.Position, decoded.ID
	}
	const query = `SELECT id::text,business_id::text,name,status,trigger_kind,conditions,action_kind,action_payload,position,resource_version,created_at,updated_at
	FROM automation_rules
	WHERE business_id=$1::uuid
	  AND ($2='' OR status=$2)
	  AND ($3::integer IS NULL OR (position,id)>($3::integer,$4::uuid))
	ORDER BY position ASC,id ASC
	LIMIT $5`
	rows, err := executor.Query(ctx, query, businessID, status, position, id, limit+1)
	if err != nil {
		return ports.AutomationRulePage{}, &RepositoryError{Operation: "automation_rule.list", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.AutomationRuleRecord, 0, limit)
	for rows.Next() {
		record, scanErr := scanAutomationRule(rows)
		if scanErr != nil {
			return ports.AutomationRulePage{}, scanErr
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return ports.AutomationRulePage{}, &RepositoryError{Operation: "automation_rule.list", Kind: RepositoryInvalid, Err: err}
	}
	page := ports.AutomationRulePage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		page.NextCursor = encodeAutomationRuleCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

func (r *AutomationRuleRepository) ListActiveInbound(ctx context.Context, businessID string) ([]ports.AutomationRuleRecord, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" {
		return nil, invalidRepositoryInput("automation_rule.list_active_inbound", "business id is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.Query(ctx, `SELECT id::text,business_id::text,name,status,trigger_kind,conditions,action_kind,action_payload,position,resource_version,created_at,updated_at FROM automation_rules WHERE business_id=$1::uuid AND status='active' AND trigger_kind='inbound_message' ORDER BY position ASC,id ASC`, businessID)
	if err != nil {
		return nil, &RepositoryError{Operation: "automation_rule.list_active_inbound", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.AutomationRuleRecord, 0)
	for rows.Next() {
		record, scanErr := scanAutomationRule(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, record)
	}
	if err := rows.Err(); err != nil {
		return nil, &RepositoryError{Operation: "automation_rule.list_active_inbound", Kind: RepositoryInvalid, Err: err}
	}
	return items, nil
}

func (r *AutomationRuleRepository) GetByID(ctx context.Context, businessID, automationRuleID string) (ports.AutomationRuleRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.AutomationRuleRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(automationRuleID) == "" {
		return ports.AutomationRuleRecord{}, invalidRepositoryInput("automation_rule.get", "business and rule ids are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AutomationRuleRecord{}, err
	}
	record, err := scanAutomationRule(executor.QueryRow(ctx, `SELECT id::text,business_id::text,name,status,trigger_kind,conditions,action_kind,action_payload,position,resource_version,created_at,updated_at FROM automation_rules WHERE business_id=$1::uuid AND id=$2::uuid`, businessID, automationRuleID))
	if err != nil {
		return ports.AutomationRuleRecord{}, classifyRepositoryGetError("automation_rule.get", err)
	}
	return record, nil
}

func (r *AutomationRuleRepository) Create(ctx context.Context, create ports.AutomationRuleCreate) (ports.AutomationRuleRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.AutomationRuleRecord{}, ErrPoolClosed
	}
	if err := validateAutomationRuleCreate(create); err != nil {
		return ports.AutomationRuleRecord{}, err
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AutomationRuleRecord{}, err
	}
	record, err := scanAutomationRule(executor.QueryRow(ctx, `INSERT INTO automation_rules (id,business_id,name,status,trigger_kind,conditions,action_kind,action_payload,position,created_at,updated_at) VALUES ($1::uuid,$2::uuid,$3,$4,$5,$6::jsonb,$7,$8::jsonb,$9,$10,$11) RETURNING id::text,business_id::text,name,status,trigger_kind,conditions,action_kind,action_payload,position,resource_version,created_at,updated_at`, create.ID, create.BusinessID, create.Name, create.Status, create.TriggerKind, create.Conditions, create.ActionKind, create.ActionPayload, create.Position, create.CreatedAt.UTC(), create.UpdatedAt.UTC()))
	if err != nil {
		return ports.AutomationRuleRecord{}, classifyRepositoryWriteError("automation_rule.create", err)
	}
	return record, nil
}

func (r *AutomationRuleRepository) Update(ctx context.Context, update ports.AutomationRuleUpdate) (ports.AutomationRuleRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.AutomationRuleRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(update.BusinessID) == "" || strings.TrimSpace(update.AutomationRuleID) == "" || update.ExpectedVersion <= 0 || update.UpdatedAt.IsZero() {
		return ports.AutomationRuleRecord{}, invalidRepositoryInput("automation_rule.update", "business, rule, expected resource version, and update time are required")
	}
	if update.Name == nil && update.Status == nil && update.Conditions == nil && update.ActionKind == nil && update.ActionPayload == nil && update.Position == nil {
		return ports.AutomationRuleRecord{}, invalidRepositoryInput("automation_rule.update", "at least one field is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.AutomationRuleRecord{}, err
	}
	record, err := scanAutomationRule(executor.QueryRow(ctx, `UPDATE automation_rules SET name=COALESCE($4,name),status=COALESCE($5,status),conditions=COALESCE($6::jsonb,conditions),action_kind=COALESCE($7,action_kind),action_payload=COALESCE($8::jsonb,action_payload),position=COALESCE($9,position),resource_version=resource_version+1,updated_at=$10 WHERE business_id=$1::uuid AND id=$2::uuid AND resource_version=$3 RETURNING id::text,business_id::text,name,status,trigger_kind,conditions,action_kind,action_payload,position,resource_version,created_at,updated_at`, update.BusinessID, update.AutomationRuleID, update.ExpectedVersion, update.Name, update.Status, update.Conditions, update.ActionKind, update.ActionPayload, update.Position, update.UpdatedAt.UTC()))
	if err == nil {
		return record, nil
	}
	return ports.AutomationRuleRecord{}, classifyAutomationRuleUpdateMiss(ctx, executor, update, err)
}

func validateAutomationRuleCreate(create ports.AutomationRuleCreate) error {
	if uuid.Validate(create.ID) != nil || strings.TrimSpace(create.BusinessID) == "" || strings.TrimSpace(create.Name) == "" || (create.Status != "active" && create.Status != "disabled") || create.TriggerKind != "inbound_message" || len(create.Conditions) == 0 || (create.ActionKind != "add_label" && create.ActionKind != "set_priority" && create.ActionKind != "assign_human") || len(create.ActionPayload) == 0 || create.Position <= 0 || create.CreatedAt.IsZero() || create.UpdatedAt.IsZero() {
		return invalidRepositoryInput("automation_rule.create", "valid id, business, name, status, inbound trigger, JSON condition/action, position, and timestamps are required")
	}
	return nil
}

func classifyAutomationRuleUpdateMiss(ctx context.Context, executor SQLExecutor, update ports.AutomationRuleUpdate, original error) error {
	if !errors.Is(original, pgx.ErrNoRows) {
		return classifyRepositoryWriteError("automation_rule.update", original)
	}
	var version int64
	err := executor.QueryRow(ctx, `SELECT resource_version FROM automation_rules WHERE business_id=$1::uuid AND id=$2::uuid`, update.BusinessID, update.AutomationRuleID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return &RepositoryError{Operation: "automation_rule.update", Kind: RepositoryNotFound, Err: err}
	}
	if err != nil {
		return classifyRepositoryWriteError("automation_rule.update", err)
	}
	if version != update.ExpectedVersion {
		return &RepositoryError{Operation: "automation_rule.update", Kind: RepositoryStale, Err: errors.New("automation rule resource version is stale")}
	}
	return &RepositoryError{Operation: "automation_rule.update", Kind: RepositoryConflict, Err: errors.New("automation rule update was rejected")}
}

type AutomationExecutionRepository struct{ adapter *Adapter }

func NewAutomationExecutionRepository(adapter *Adapter) *AutomationExecutionRepository {
	return &AutomationExecutionRepository{adapter: adapter}
}

func (r *AutomationExecutionRepository) RecordIfAbsent(ctx context.Context, draft ports.AutomationExecutionDraft) (bool, ports.AutomationExecutionDraft, error) {
	if r == nil || r.adapter == nil {
		return false, ports.AutomationExecutionDraft{}, ErrPoolClosed
	}
	if uuid.Validate(draft.ID) != nil || strings.TrimSpace(draft.BusinessID) == "" || strings.TrimSpace(draft.RuleID) == "" || strings.TrimSpace(draft.InboundEventID) == "" || draft.Result != "processing" || strings.TrimSpace(draft.ReasonCode) == "" || draft.CreatedAt.IsZero() || draft.UpdatedAt.IsZero() {
		return false, ports.AutomationExecutionDraft{}, invalidRepositoryInput("automation_execution.record", "valid ids, processing result, reason code, and timestamps are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return false, ports.AutomationExecutionDraft{}, err
	}
	var result ports.AutomationExecutionDraft
	err = executor.QueryRow(ctx, `INSERT INTO automation_executions (id,business_id,rule_id,inbound_event_id,result,reason_code,created_at,updated_at) VALUES ($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,$8) ON CONFLICT (business_id,rule_id,inbound_event_id) DO NOTHING RETURNING id::text,business_id::text,rule_id::text,inbound_event_id::text,result,reason_code,created_at,updated_at`, draft.ID, draft.BusinessID, draft.RuleID, draft.InboundEventID, draft.Result, draft.ReasonCode, draft.CreatedAt.UTC(), draft.UpdatedAt.UTC()).Scan(&result.ID, &result.BusinessID, &result.RuleID, &result.InboundEventID, &result.Result, &result.ReasonCode, &result.CreatedAt, &result.UpdatedAt)
	if err == nil {
		return true, result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, ports.AutomationExecutionDraft{}, classifyRepositoryWriteError("automation_execution.record", err)
	}
	err = executor.QueryRow(ctx, `SELECT id::text,business_id::text,rule_id::text,inbound_event_id::text,result,reason_code,created_at,updated_at FROM automation_executions WHERE business_id=$1::uuid AND rule_id=$2::uuid AND inbound_event_id=$3::uuid`, draft.BusinessID, draft.RuleID, draft.InboundEventID).Scan(&result.ID, &result.BusinessID, &result.RuleID, &result.InboundEventID, &result.Result, &result.ReasonCode, &result.CreatedAt, &result.UpdatedAt)
	if err != nil {
		return false, ports.AutomationExecutionDraft{}, classifyRepositoryGetError("automation_execution.record", err)
	}
	return false, result, nil
}

func (r *AutomationExecutionRepository) Complete(ctx context.Context, patch ports.AutomationExecutionPatch) error {
	if r == nil || r.adapter == nil {
		return ErrPoolClosed
	}
	if strings.TrimSpace(patch.ID) == "" || (patch.Result != "executed" && patch.Result != "skipped" && patch.Result != "failed") || strings.TrimSpace(patch.ReasonCode) == "" || patch.UpdatedAt.IsZero() {
		return invalidRepositoryInput("automation_execution.complete", "id, final result, reason code, and update time are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return err
	}
	tag, err := executor.Exec(ctx, `UPDATE automation_executions SET result=$2,reason_code=$3,updated_at=$4 WHERE id=$1::uuid AND result='processing'`, patch.ID, patch.Result, patch.ReasonCode, patch.UpdatedAt.UTC())
	if err != nil {
		return classifyRepositoryWriteError("automation_execution.complete", err)
	}
	if tag.RowsAffected() != 1 {
		return &RepositoryError{Operation: "automation_execution.complete", Kind: RepositoryConflict, Err: errors.New("automation execution is not processing")}
	}
	return nil
}

type automationRuleScanner interface{ Scan(dest ...any) error }

func scanAutomationRule(scanner automationRuleScanner) (ports.AutomationRuleRecord, error) {
	var record ports.AutomationRuleRecord
	if err := scanner.Scan(&record.ID, &record.BusinessID, &record.Name, &record.Status, &record.TriggerKind, &record.Conditions, &record.ActionKind, &record.ActionPayload, &record.Position, &record.ResourceVersion, &record.CreatedAt, &record.UpdatedAt); err != nil {
		return ports.AutomationRuleRecord{}, err
	}
	return record, nil
}

func encodeAutomationRuleCursor(record ports.AutomationRuleRecord) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.TrimSpace(strings.Join([]string{strconv.Itoa(record.Position), record.ID}, "|"))))
}

func decodeAutomationRuleCursor(value string) (*automationRuleCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid automation rule cursor")
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 || uuid.Validate(parts[1]) != nil {
		return nil, errors.New("invalid automation rule cursor")
	}
	position, err := strconv.Atoi(parts[0])
	if err != nil || position <= 0 {
		return nil, errors.New("invalid automation rule cursor")
	}
	return &automationRuleCursor{Position: position, ID: parts[1]}, nil
}

var _ ports.AutomationRuleRepository = (*AutomationRuleRepository)(nil)
var _ ports.AutomationExecutionRepository = (*AutomationExecutionRepository)(nil)

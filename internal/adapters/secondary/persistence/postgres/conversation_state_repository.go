package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

type ConversationStateRepository struct{ adapter *Adapter }

func NewConversationStateRepository(adapter *Adapter) *ConversationStateRepository {
	return &ConversationStateRepository{adapter: adapter}
}

func (r *ConversationStateRepository) Get(ctx context.Context, businessID, conversationID string) (ports.ConversationStateRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ConversationStateRecord{}, ErrPoolClosed
	}
	businessID = strings.TrimSpace(businessID)
	conversationID = strings.TrimSpace(conversationID)
	if businessID == "" || conversationID == "" {
		return ports.ConversationStateRecord{}, invalidRepositoryInput("conversation_state.get", "business and conversation ids are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ConversationStateRecord{}, err
	}
	var rec ports.ConversationStateRecord
	var focusJSON, previousJSON, comparisonJSON, preferencesJSON, constraintsJSON, pendingJSON []byte
	var summary, checkpoint *string // nullable
	err = executor.QueryRow(ctx, `SELECT business_id::text, conversation_id::text, focus, previous, comparison, preferences, constraints, pending, summary, summary_turn_count, summary_last_message_id::text, version, created_at, updated_at FROM conversation_state WHERE business_id = $1::uuid AND conversation_id = $2::uuid`, businessID, conversationID).Scan(
		&rec.BusinessID, &rec.ConversationID, &focusJSON, &previousJSON, &comparisonJSON, &preferencesJSON, &constraintsJSON, &pendingJSON, &summary, &rec.SummaryTurnCount, &checkpoint, &rec.Version, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ports.ConversationStateRecord{}, &RepositoryError{Operation: "conversation_state.get", Kind: RepositoryNotFound, Err: err}
		}
		return ports.ConversationStateRecord{}, &RepositoryError{Operation: "conversation_state.get", Kind: RepositoryInvalid, Err: err}
	}
	if summary != nil {
		rec.Summary = *summary
	}
	if checkpoint != nil {
		rec.SummaryLastMessageID = *checkpoint
	}
	if len(focusJSON) > 0 && string(focusJSON) != "null" {
		var focus ports.ConversationFocus
		if err := json.Unmarshal(focusJSON, &focus); err == nil && focus.ID != "" {
			rec.Focus = &focus
		}
	}
	if err := json.Unmarshal(previousJSON, &rec.Previous); err != nil {
		rec.Previous = []ports.ConversationFocus{}
	}
	if len(comparisonJSON) > 0 && string(comparisonJSON) != "null" {
		var comp ports.ConversationComparison
		if err := json.Unmarshal(comparisonJSON, &comp); err == nil && len(comp.IDs) > 0 {
			rec.Comparison = &comp
		}
	}
	_ = json.Unmarshal(preferencesJSON, &rec.Preferences)
	_ = json.Unmarshal(constraintsJSON, &rec.Constraints)
	_ = json.Unmarshal(pendingJSON, &rec.Pending)
	if rec.Previous == nil {
		rec.Previous = []ports.ConversationFocus{}
	}
	if rec.Preferences == nil {
		rec.Preferences = []ports.StatePreference{}
	}
	if rec.Constraints == nil {
		rec.Constraints = []ports.StateConstraint{}
	}
	if rec.Pending == nil {
		rec.Pending = []ports.StatePending{}
	}
	return rec, nil
}

func (r *ConversationStateRepository) UpsertValidated(ctx context.Context, record ports.ConversationStateRecord) (ports.ConversationStateRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ConversationStateRecord{}, ErrPoolClosed
	}
	record.BusinessID = strings.TrimSpace(record.BusinessID)
	record.ConversationID = strings.TrimSpace(record.ConversationID)
	if record.BusinessID == "" || record.ConversationID == "" {
		return ports.ConversationStateRecord{}, invalidRepositoryInput("conversation_state.upsert", "business and conversation ids are required")
	}
	if record.Version <= 0 {
		record.Version = 1
	}
	now := time.Now().UTC()
	if !record.UpdatedAt.IsZero() {
		now = record.UpdatedAt.UTC()
	}
	var focusJSON []byte
	if record.Focus != nil {
		b, _ := json.Marshal(record.Focus)
		focusJSON = b
	}
	previousJSON, _ := json.Marshal(record.Previous)
	if record.Previous == nil {
		previousJSON = []byte(`[]`)
	}
	var comparisonJSON []byte
	if record.Comparison != nil {
		b, _ := json.Marshal(record.Comparison)
		comparisonJSON = b
	}
	preferencesJSON, _ := json.Marshal(record.Preferences)
	if record.Preferences == nil {
		preferencesJSON = []byte(`[]`)
	}
	constraintsJSON, _ := json.Marshal(record.Constraints)
	if record.Constraints == nil {
		constraintsJSON = []byte(`[]`)
	}
	pendingJSON, _ := json.Marshal(record.Pending)
	if record.Pending == nil {
		pendingJSON = []byte(`[]`)
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ConversationStateRecord{}, err
	}
	var result ports.ConversationStateRecord
	var outFocus, outPrevious, outComparison, outPreferences, outConstraints, outPending []byte
	var outSummary, outCheckpoint *string
	err = executor.QueryRow(ctx, `
                INSERT INTO conversation_state (business_id, conversation_id, focus, previous, comparison, preferences, constraints, pending, summary, summary_turn_count, summary_last_message_id, version, created_at, updated_at)
                VALUES ($1::uuid, $2::uuid, $3::jsonb, $4::jsonb, $5::jsonb, $6::jsonb, $7::jsonb, $8::jsonb, $9, $10, $11::uuid, $12, $13, $13)
                ON CONFLICT (business_id, conversation_id) DO UPDATE SET
                        focus = EXCLUDED.focus,
                        previous = EXCLUDED.previous,
                        comparison = EXCLUDED.comparison,
                        preferences = EXCLUDED.preferences,
                        constraints = EXCLUDED.constraints,
                        pending = EXCLUDED.pending,
                        summary = conversation_state.summary,
                        summary_turn_count = conversation_state.summary_turn_count,
                        summary_last_message_id = conversation_state.summary_last_message_id,
                        version = conversation_state.version + 1,
                        updated_at = EXCLUDED.updated_at
                RETURNING business_id::text, conversation_id::text, focus, previous, comparison, preferences, constraints, pending, summary, summary_turn_count, summary_last_message_id::text, version, created_at, updated_at
        `, record.BusinessID, record.ConversationID, nullableJSON(focusJSON), previousJSON, nullableJSON(comparisonJSON), preferencesJSON, constraintsJSON, pendingJSON,
		nullableText(record.Summary), record.SummaryTurnCount, nullableText(record.SummaryLastMessageID), record.Version, now).Scan(
		&result.BusinessID, &result.ConversationID, &outFocus, &outPrevious, &outComparison, &outPreferences, &outConstraints, &outPending, &outSummary, &result.SummaryTurnCount, &outCheckpoint, &result.Version, &result.CreatedAt, &result.UpdatedAt,
	)
	if err != nil {
		return ports.ConversationStateRecord{}, &RepositoryError{Operation: "conversation_state.upsert", Kind: RepositoryInvalid, Err: err}
	}
	if outSummary != nil {
		result.Summary = *outSummary
	}
	if outCheckpoint != nil {
		result.SummaryLastMessageID = *outCheckpoint
	}
	if len(outFocus) > 0 && string(outFocus) != "null" {
		var focus ports.ConversationFocus
		if err := json.Unmarshal(outFocus, &focus); err == nil {
			result.Focus = &focus
		}
	}
	_ = json.Unmarshal(outPrevious, &result.Previous)
	if len(outComparison) > 0 && string(outComparison) != "null" {
		var comp ports.ConversationComparison
		_ = json.Unmarshal(outComparison, &comp)
		result.Comparison = &comp
	}
	_ = json.Unmarshal(outPreferences, &result.Preferences)
	_ = json.Unmarshal(outConstraints, &result.Constraints)
	_ = json.Unmarshal(outPending, &result.Pending)
	return result, nil
}

// EnsureSummaryState creates memory state for ordinary conversations that have
// no selected catalog focus. It never changes an existing state row.
func (r *ConversationStateRepository) EnsureSummaryState(ctx context.Context, businessID, conversationID string) (ports.ConversationStateRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ConversationStateRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(conversationID) == "" {
		return ports.ConversationStateRecord{}, invalidRepositoryInput("conversation_state.ensure", "scope required")
	}
	exec, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ConversationStateRecord{}, err
	}
	_, err = exec.Exec(ctx, "INSERT INTO conversation_state (business_id,conversation_id,created_at,updated_at) VALUES ($1::uuid,$2::uuid,NOW(),NOW()) ON CONFLICT (business_id,conversation_id) DO NOTHING", businessID, conversationID)
	if err != nil {
		return ports.ConversationStateRecord{}, &RepositoryError{Operation: "conversation_state.ensure", Kind: RepositoryInvalid, Err: err}
	}
	return r.Get(ctx, businessID, conversationID)
}

// SaveSummaryIfVersion updates only summarization fields and requires the
// state version seen before inference. No other writer can be overwritten.
func (r *ConversationStateRepository) SaveSummaryIfVersion(ctx context.Context, businessID, conversationID string, expectedVersion int64, summary, lastMessageID string, coveredInbound int) (bool, error) {
	if r == nil || r.adapter == nil {
		return false, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(conversationID) == "" || strings.TrimSpace(lastMessageID) == "" || strings.TrimSpace(summary) == "" || coveredInbound < 0 || expectedVersion < 1 {
		return false, invalidRepositoryInput("conversation_state.save_summary", "scope, version, checkpoint and summary required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return false, err
	}
	tag, err := executor.Exec(ctx, "UPDATE conversation_state SET summary=$1, summary_turn_count=$2, summary_last_message_id=$3::uuid, version=version+1, updated_at=NOW() WHERE business_id=$4::uuid AND conversation_id=$5::uuid AND version=$6", summary, coveredInbound, lastMessageID, businessID, conversationID, expectedVersion)
	if err != nil {
		return false, &RepositoryError{Operation: "conversation_state.save_summary", Kind: RepositoryInvalid, Err: err}
	}
	return tag.RowsAffected() == 1, nil
}

var _ ports.ConversationSummaryCheckpointRepository = (*ConversationStateRepository)(nil)

func nullableJSON(b []byte) any {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	return string(b)
}

// nullableText is defined in provider_inbound_store.go and reused here.

var _ ports.ConversationStateRepository = (*ConversationStateRepository)(nil)

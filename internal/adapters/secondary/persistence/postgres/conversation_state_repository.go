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
	err = executor.QueryRow(ctx, `SELECT business_id::text, conversation_id::text, focus, previous, comparison, preferences, constraints, pending, version, created_at, updated_at FROM conversation_state WHERE business_id = $1::uuid AND conversation_id = $2::uuid`, businessID, conversationID).Scan(
		&rec.BusinessID, &rec.ConversationID, &focusJSON, &previousJSON, &comparisonJSON, &preferencesJSON, &constraintsJSON, &pendingJSON, &rec.Version, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ports.ConversationStateRecord{}, &RepositoryError{Operation: "conversation_state.get", Kind: RepositoryNotFound, Err: err}
		}
		return ports.ConversationStateRecord{}, &RepositoryError{Operation: "conversation_state.get", Kind: RepositoryInvalid, Err: err}
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
	err = executor.QueryRow(ctx, `
		INSERT INTO conversation_state (business_id, conversation_id, focus, previous, comparison, preferences, constraints, pending, version, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::jsonb, $4::jsonb, $5::jsonb, $6::jsonb, $7::jsonb, $8::jsonb, $9, $10, $10)
		ON CONFLICT (business_id, conversation_id) DO UPDATE SET
			focus = EXCLUDED.focus,
			previous = EXCLUDED.previous,
			comparison = EXCLUDED.comparison,
			preferences = EXCLUDED.preferences,
			constraints = EXCLUDED.constraints,
			pending = EXCLUDED.pending,
			version = conversation_state.version + 1,
			updated_at = EXCLUDED.updated_at
		RETURNING business_id::text, conversation_id::text, focus, previous, comparison, preferences, constraints, pending, version, created_at, updated_at
	`, record.BusinessID, record.ConversationID, nullableJSON(focusJSON), previousJSON, nullableJSON(comparisonJSON), preferencesJSON, constraintsJSON, pendingJSON, record.Version, now).Scan(
		&result.BusinessID, &result.ConversationID, &outFocus, &outPrevious, &outComparison, &outPreferences, &outConstraints, &outPending, &result.Version, &result.CreatedAt, &result.UpdatedAt,
	)
	if err != nil {
		return ports.ConversationStateRecord{}, &RepositoryError{Operation: "conversation_state.upsert", Kind: RepositoryInvalid, Err: err}
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

func nullableJSON(b []byte) any {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	return string(b)
}

var _ ports.ConversationStateRepository = (*ConversationStateRepository)(nil)

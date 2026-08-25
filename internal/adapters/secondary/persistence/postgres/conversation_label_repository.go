package postgres

import (
	"context"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type ConversationLabelRepository struct{ adapter *Adapter }

func NewConversationLabelRepository(adapter *Adapter) *ConversationLabelRepository {
	return &ConversationLabelRepository{adapter: adapter}
}

func (r *ConversationLabelRepository) List(ctx context.Context, businessID, conversationID string) ([]string, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(conversationID) == "" {
		return nil, invalidRepositoryInput("conversation_label.list", "business and conversation ids are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.Query(ctx, `SELECT label FROM conversation_labels WHERE business_id=$1::uuid AND conversation_id=$2::uuid ORDER BY label`, businessID, conversationID)
	if err != nil {
		return nil, &RepositoryError{Operation: "conversation_label.list", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, &RepositoryError{Operation: "conversation_label.list", Kind: RepositoryInvalid, Err: err}
		}
		result = append(result, label)
	}
	if err := rows.Err(); err != nil {
		return nil, &RepositoryError{Operation: "conversation_label.list", Kind: RepositoryInvalid, Err: err}
	}
	return result, nil
}

func (r *ConversationLabelRepository) Apply(ctx context.Context, businessID, conversationID string, add, remove []string) error {
	if r == nil || r.adapter == nil {
		return ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(conversationID) == "" {
		return invalidRepositoryInput("conversation_label.apply", "business and conversation ids are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return err
	}
	var exists bool
	if err := executor.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM conversations WHERE business_id=$1::uuid AND id=$2::uuid)`, businessID, conversationID).Scan(&exists); err != nil {
		return &RepositoryError{Operation: "conversation_label.apply", Kind: RepositoryInvalid, Err: err}
	}
	if !exists {
		return &RepositoryError{Operation: "conversation_label.apply", Kind: RepositoryNotFound}
	}
	for _, label := range normalizeLabels(remove) {
		if _, err := executor.Exec(ctx, `DELETE FROM conversation_labels WHERE business_id=$1::uuid AND conversation_id=$2::uuid AND label=$3`, businessID, conversationID, label); err != nil {
			return &RepositoryError{Operation: "conversation_label.apply", Kind: RepositoryInvalid, Err: err}
		}
	}
	for _, label := range normalizeLabels(add) {
		if _, err := executor.Exec(ctx, `INSERT INTO conversation_labels (business_id,conversation_id,label,created_at) VALUES ($1::uuid,$2::uuid,$3,now()) ON CONFLICT DO NOTHING`, businessID, conversationID, label); err != nil {
			return &RepositoryError{Operation: "conversation_label.apply", Kind: RepositoryInvalid, Err: err}
		}
	}
	return nil
}

func normalizeLabels(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		label := strings.ToLower(strings.TrimSpace(value))
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		result = append(result, label)
	}
	return result
}

var _ ports.ConversationLabelRepository = (*ConversationLabelRepository)(nil)

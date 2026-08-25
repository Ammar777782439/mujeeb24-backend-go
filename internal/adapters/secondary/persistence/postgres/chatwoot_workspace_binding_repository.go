package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ChatwootWorkspaceBindingRepository struct{ adapter *Adapter }

func NewChatwootWorkspaceBindingRepository(adapter *Adapter) *ChatwootWorkspaceBindingRepository {
	return &ChatwootWorkspaceBindingRepository{adapter: adapter}
}

func (r *ChatwootWorkspaceBindingRepository) EnsureBinding(ctx context.Context, businessID, routeKey, accountID, inboxID, channel string) (string, error) {
	if r == nil || r.adapter == nil {
		return "", ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(routeKey) == "" || strings.TrimSpace(accountID) == "" || strings.TrimSpace(inboxID) == "" || strings.TrimSpace(channel) == "" {
		return "", invalidRepositoryInput("chatwoot_binding.ensure", "business, route, account, inbox, and channel are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	const query = `
		INSERT INTO chatwoot_workspace_bindings (id, business_id, route_key, account_id, inbox_id, channel, active, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, TRUE, now(), now())
		ON CONFLICT (route_key) DO UPDATE SET updated_at = chatwoot_workspace_bindings.updated_at
		RETURNING id::text, business_id::text, account_id, inbox_id, channel`
	var resultID, resultBusiness, resultAccount, resultInbox, resultChannel string
	if err := executor.QueryRow(ctx, query, id, businessID, routeKey, accountID, inboxID, channel).Scan(&resultID, &resultBusiness, &resultAccount, &resultInbox, &resultChannel); err != nil {
		return "", &RepositoryError{Operation: "chatwoot_binding.ensure", Kind: RepositoryInvalid, Err: err}
	}
	if resultBusiness != businessID || resultAccount != accountID || resultInbox != inboxID || resultChannel != channel {
		return "", &RepositoryError{Operation: "chatwoot_binding.ensure", Kind: RepositoryConflict, Err: errors.New("chatwoot binding belongs to a different business or workspace")}
	}
	return resultID, nil
}

var _ ports.ChatwootWorkspaceBindingWriter = (*ChatwootWorkspaceBindingRepository)(nil)
var _ = pgx.ErrNoRows

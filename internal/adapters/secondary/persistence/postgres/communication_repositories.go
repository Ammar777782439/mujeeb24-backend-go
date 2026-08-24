package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type ConversationReferenceRepository struct{ adapter *Adapter }

func NewConversationReferenceRepository(adapter *Adapter) *ConversationReferenceRepository {
	return &ConversationReferenceRepository{adapter: adapter}
}

func (r *ConversationReferenceRepository) GetCurrentByConversation(ctx context.Context, businessID, conversationID, system string) (ports.ConversationReferenceRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ConversationReferenceRecord{}, ErrPoolClosed
	}
	if businessID == "" || conversationID == "" || system == "" {
		return ports.ConversationReferenceRecord{}, invalidRepositoryInput("conversation_reference.get_current", "business id, conversation id, and system are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ConversationReferenceRecord{}, err
	}
	const query = `SELECT id::text, business_id::text, conversation_id::text, system, provider_ref, resource_type, resource_id, connection_id::text, conversation_kind, is_current, mapping_status FROM conversation_references WHERE business_id = $1::uuid AND conversation_id = $2::uuid AND system = $3 AND is_current ORDER BY updated_at DESC LIMIT 1`
	var record ports.ConversationReferenceRecord
	if err := executor.QueryRow(ctx, query, businessID, conversationID, system).Scan(&record.ID, &record.BusinessID, &record.ConversationID, &record.System, &record.ProviderRef, &record.ResourceType, &record.ResourceID, &record.ConnectionID, &record.ConversationKind, &record.IsCurrent, &record.MappingStatus); err != nil {
		return record, classifyRepositoryGetError("conversation_reference.get_current", err)
	}
	return record, nil
}

var _ ports.ConversationReferenceRepository = (*ConversationReferenceRepository)(nil)

type OutboundMessageRepository struct{ adapter *Adapter }

func NewOutboundMessageRepository(adapter *Adapter) *OutboundMessageRepository {
	return &OutboundMessageRepository{adapter: adapter}
}

func (r *OutboundMessageRepository) CreatePending(ctx context.Context, draft ports.OutboundMessageDraft) (ports.OutboundMessageRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.OutboundMessageRecord{}, ErrPoolClosed
	}
	if draft.BusinessID == "" || draft.ID == "" || draft.ConversationID == "" || draft.ConversationReferenceID == "" || draft.ConnectionID == "" || draft.ProviderRef == "" || draft.Channel == "" || draft.Origin == "" || draft.Transport == "" || draft.ContentReference == "" || draft.ProviderIdempotencyKey == "" {
		return ports.OutboundMessageRecord{}, invalidRepositoryInput("outbound_message.create_pending", "required outbound message fields are missing")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.OutboundMessageRecord{}, err
	}
	const query = `INSERT INTO outbound_messages (id, business_id, conversation_id, conversation_reference_id, connection_id, provider_ref, channel, origin, direction, transport, content_reference, provider_idempotency_key, status, correlation_id, causation_id, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6, $7, $8, 'outbound', $9, $10, $11, 'pending', $12::uuid, $13::uuid, now(), now()) RETURNING id::text, business_id::text, conversation_id::text, conversation_reference_id::text, connection_id::text, provider_ref, channel, origin, direction, transport, content_reference, provider_idempotency_key, status, provider_message_id, chatwoot_message_id, failure_code, attempt_count, correlation_id::text, causation_id::text`
	return scanOutboundMessage(executor.QueryRow(ctx, query, draft.ID, draft.BusinessID, draft.ConversationID, draft.ConversationReferenceID, draft.ConnectionID, draft.ProviderRef, draft.Channel, draft.Origin, draft.Transport, draft.ContentReference, draft.ProviderIdempotencyKey, draft.CorrelationID, draft.CausationID))
}

func (r *OutboundMessageRepository) GetByID(ctx context.Context, businessID, messageID string) (ports.OutboundMessageRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.OutboundMessageRecord{}, ErrPoolClosed
	}
	if businessID == "" || messageID == "" {
		return ports.OutboundMessageRecord{}, invalidRepositoryInput("outbound_message.get_by_id", "business and message ids are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.OutboundMessageRecord{}, err
	}
	const query = `SELECT id::text, business_id::text, conversation_id::text, conversation_reference_id::text, connection_id::text, provider_ref, channel, origin, direction, transport, content_reference, provider_idempotency_key, status, provider_message_id, chatwoot_message_id, failure_code, attempt_count, correlation_id::text, causation_id::text FROM outbound_messages WHERE business_id = $1::uuid AND id = $2::uuid`
	return scanOutboundMessage(executor.QueryRow(ctx, query, businessID, messageID))
}

func scanOutboundMessage(row pgx.Row) (ports.OutboundMessageRecord, error) {
	var record ports.OutboundMessageRecord
	err := row.Scan(&record.ID, &record.BusinessID, &record.ConversationID, &record.ConversationReferenceID, &record.ConnectionID, &record.ProviderRef, &record.Channel, &record.Origin, &record.Direction, &record.Transport, &record.ContentReference, &record.ProviderIdempotencyKey, &record.Status, &record.ProviderMessageID, &record.ChatwootMessageID, &record.FailureCode, &record.AttemptCount, &record.CorrelationID, &record.CausationID)
	if err == nil {
		return record, nil
	}
	return record, classifyRepositoryWriteError("outbound_message", err)
}

func classifyRepositoryWriteError(operation string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return &RepositoryError{Operation: operation, Kind: RepositoryNotFound, Err: err}
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return &RepositoryError{Operation: operation, Kind: RepositoryConflict, Err: err}
	}
	return &RepositoryError{Operation: operation, Kind: RepositoryInvalid, Err: fmt.Errorf("%s: %w", operation, err)}
}

var _ ports.OutboundMessageRepository = (*OutboundMessageRepository)(nil)

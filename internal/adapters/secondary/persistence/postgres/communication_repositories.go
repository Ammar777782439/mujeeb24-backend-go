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

func (r *ConversationReferenceRepository) GetByID(ctx context.Context, businessID, referenceID string) (ports.ConversationReferenceRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ConversationReferenceRecord{}, ErrPoolClosed
	}
	if businessID == "" || referenceID == "" {
		return ports.ConversationReferenceRecord{}, invalidRepositoryInput("conversation_reference.get_by_id", "business id and reference id are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ConversationReferenceRecord{}, err
	}
	const query = `SELECT id::text, business_id::text, conversation_id::text, system, provider_ref, resource_type, resource_id, connection_id::text, conversation_kind, is_current, mapping_status, chatwoot_account_id, chatwoot_inbox_id, chatwoot_conversation_id FROM conversation_references WHERE business_id = $1::uuid AND id = $2::uuid`
	var record ports.ConversationReferenceRecord
	if err := executor.QueryRow(ctx, query, businessID, referenceID).Scan(&record.ID, &record.BusinessID, &record.ConversationID, &record.System, &record.ProviderRef, &record.ResourceType, &record.ResourceID, &record.ConnectionID, &record.ConversationKind, &record.IsCurrent, &record.MappingStatus, &record.ChatwootAccountID, &record.ChatwootInboxID, &record.ChatwootConversationID); err != nil {
		return record, classifyRepositoryGetError("conversation_reference.get_by_id", err)
	}
	return record, nil
}

func (r *ConversationReferenceRepository) BindProviderToChatwoot(ctx context.Context, draft ports.ProviderChatwootBindingDraft) (ports.ConversationReferenceRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ConversationReferenceRecord{}, ErrPoolClosed
	}
	if draft.BusinessID == "" || draft.ReferenceID == "" || draft.AccountID == "" || draft.InboxID == "" || draft.ChatwootConversationID == "" {
		return ports.ConversationReferenceRecord{}, invalidRepositoryInput("conversation_reference.bind_chatwoot", "business, reference, Chatwoot account, inbox, and conversation ids are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ConversationReferenceRecord{}, err
	}
	const query = `UPDATE conversation_references SET chatwoot_account_id = $3, chatwoot_inbox_id = $4, chatwoot_conversation_id = $5, updated_at = now() WHERE business_id = $1::uuid AND id = $2::uuid AND system = 'provider' AND is_current AND mapping_status = 'active' RETURNING id::text, business_id::text, conversation_id::text, system, provider_ref, resource_type, resource_id, connection_id::text, conversation_kind, is_current, mapping_status, chatwoot_account_id, chatwoot_inbox_id, chatwoot_conversation_id`
	var record ports.ConversationReferenceRecord
	if err := executor.QueryRow(ctx, query, draft.BusinessID, draft.ReferenceID, draft.AccountID, draft.InboxID, draft.ChatwootConversationID).Scan(&record.ID, &record.BusinessID, &record.ConversationID, &record.System, &record.ProviderRef, &record.ResourceType, &record.ResourceID, &record.ConnectionID, &record.ConversationKind, &record.IsCurrent, &record.MappingStatus, &record.ChatwootAccountID, &record.ChatwootInboxID, &record.ChatwootConversationID); err != nil {
		return record, classifyRepositoryGetError("conversation_reference.bind_chatwoot", err)
	}
	return record, nil
}

func (r *ConversationReferenceRepository) GetCurrentProviderByChatwoot(ctx context.Context, businessID, accountID, inboxID, conversationID string) (ports.ConversationReferenceRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ConversationReferenceRecord{}, ErrPoolClosed
	}
	if businessID == "" || accountID == "" || inboxID == "" || conversationID == "" {
		return ports.ConversationReferenceRecord{}, invalidRepositoryInput("conversation_reference.get_provider_by_chatwoot", "business, Chatwoot account, inbox, and conversation ids are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ConversationReferenceRecord{}, err
	}
	const query = `SELECT id::text, business_id::text, conversation_id::text, system, provider_ref, resource_type, resource_id, connection_id::text, conversation_kind, is_current, mapping_status, chatwoot_account_id, chatwoot_inbox_id, chatwoot_conversation_id FROM conversation_references WHERE business_id = $1::uuid AND system = 'provider' AND is_current AND chatwoot_account_id = $2 AND chatwoot_inbox_id = $3 AND chatwoot_conversation_id = $4 ORDER BY updated_at DESC LIMIT 1`
	var record ports.ConversationReferenceRecord
	if err := executor.QueryRow(ctx, query, businessID, accountID, inboxID, conversationID).Scan(&record.ID, &record.BusinessID, &record.ConversationID, &record.System, &record.ProviderRef, &record.ResourceType, &record.ResourceID, &record.ConnectionID, &record.ConversationKind, &record.IsCurrent, &record.MappingStatus, &record.ChatwootAccountID, &record.ChatwootInboxID, &record.ChatwootConversationID); err != nil {
		return record, classifyRepositoryGetError("conversation_reference.get_provider_by_chatwoot", err)
	}
	return record, nil
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
	const query = `SELECT id::text, business_id::text, conversation_id::text, system, provider_ref, resource_type, resource_id, connection_id::text, conversation_kind, is_current, mapping_status, chatwoot_account_id, chatwoot_inbox_id, chatwoot_conversation_id FROM conversation_references WHERE business_id = $1::uuid AND conversation_id = $2::uuid AND system = $3 AND is_current ORDER BY updated_at DESC LIMIT 1`
	var record ports.ConversationReferenceRecord
	if err := executor.QueryRow(ctx, query, businessID, conversationID, system).Scan(&record.ID, &record.BusinessID, &record.ConversationID, &record.System, &record.ProviderRef, &record.ResourceType, &record.ResourceID, &record.ConnectionID, &record.ConversationKind, &record.IsCurrent, &record.MappingStatus, &record.ChatwootAccountID, &record.ChatwootInboxID, &record.ChatwootConversationID); err != nil {
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

func (r *OutboundMessageRepository) MarkProviderAccepted(ctx context.Context, businessID, messageID, providerMessageID string) (ports.OutboundMessageRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.OutboundMessageRecord{}, ErrPoolClosed
	}
	if businessID == "" || messageID == "" || providerMessageID == "" {
		return ports.OutboundMessageRecord{}, invalidRepositoryInput("outbound_message.mark_accepted", "business, message, and provider message are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.OutboundMessageRecord{}, err
	}
	const query = `
		UPDATE outbound_messages
		SET provider_message_id = COALESCE(provider_message_id, $3),
			status = CASE WHEN status IN ('delivered', 'read') THEN status ELSE 'accepted' END,
			updated_at = now()
		WHERE business_id = $1::uuid AND id = $2::uuid
		  AND (provider_message_id IS NULL OR provider_message_id = $3)
		RETURNING id::text, business_id::text, conversation_id::text, conversation_reference_id::text, connection_id::text, provider_ref, channel, origin, direction, transport, content_reference, provider_idempotency_key, status, provider_message_id, chatwoot_message_id, failure_code, attempt_count, correlation_id::text, causation_id::text`
	return scanOutboundMessage(executor.QueryRow(ctx, query, businessID, messageID, providerMessageID))
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
var _ ports.ProviderAcceptanceRecorder = (*OutboundMessageRepository)(nil)

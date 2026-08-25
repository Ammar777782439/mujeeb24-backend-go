package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ChatwootInboundStore struct{ adapter *Adapter }

func NewChatwootInboundStore(adapter *Adapter) *ChatwootInboundStore {
	return &ChatwootInboundStore{adapter: adapter}
}

func (s *ChatwootInboundStore) Materialize(ctx context.Context, draft ports.ChatwootInboundDraft) (ports.ChatwootInboundResult, error) {
	if s == nil || s.adapter == nil {
		return ports.ChatwootInboundResult{}, ErrPoolClosed
	}
	if err := validateChatwootInboundDraft(draft); err != nil {
		return ports.ChatwootInboundResult{}, err
	}
	if draft.ReceivedAt.IsZero() {
		draft.ReceivedAt = time.Now().UTC()
	}
	if draft.OccurredAt.IsZero() {
		draft.OccurredAt = draft.ReceivedAt
	}
	if draft.EventID == "" {
		return ports.ChatwootInboundResult{}, invalidRepositoryInput("chatwoot_inbound.materialize", "event id is required")
	}

	var result ports.ChatwootInboundResult
	err := s.adapter.Within(ctx, func(txCtx context.Context) error {
		executor, err := s.adapter.Executor(txCtx)
		if err != nil {
			return err
		}
		bindingID, businessID, err := findChatwootBinding(txCtx, executor, draft)
		if err != nil {
			return err
		}
		if err := lockChatwootKey(txCtx, executor, "contact:"+bindingID+":"+draft.ExternalUserID); err != nil {
			return err
		}

		inboundEventID, created, err := recordChatwootInboundEvent(txCtx, executor, businessID, draft)
		if err != nil {
			return err
		}
		result.BusinessID = businessID
		result.InboundEventID = inboundEventID
		if !created {
			result.Duplicate = true
			result.CustomerID, result.ConversationID, result.ConversationReferenceID, result.CommunicationMessageID = findMaterializedChatwootReferences(txCtx, executor, businessID, draft)
			return nil
		}

		customerID, err := findOrCreateChatwootCustomer(txCtx, executor, bindingID, businessID, draft)
		if err != nil {
			return err
		}
		conversationID, referenceID, err := findOrCreateChatwootConversation(txCtx, executor, businessID, customerID, draft)
		if err != nil {
			return err
		}
		messageID, err := recordChatwootCommunicationMessage(txCtx, executor, businessID, conversationID, referenceID, inboundEventID, draft)
		if err != nil {
			return err
		}
		processedAt := time.Now().UTC()
		if _, err := executor.Exec(txCtx, `UPDATE inbound_event_ledger SET processing_state = 'processed', processing_result_code = 'chatwoot_materialized', processed_at = $2, updated_at = $2 WHERE id = $1::uuid AND processing_state = 'received'`, inboundEventID, processedAt); err != nil {
			return classifyRepositoryWriteError("chatwoot_inbound.mark_processed", err)
		}
		result.BusinessID = businessID
		result.CustomerID = customerID
		result.ConversationID = conversationID
		result.ConversationReferenceID = referenceID
		result.CommunicationMessageID = messageID
		return nil
	})
	if err != nil {
		return ports.ChatwootInboundResult{}, err
	}
	return result, nil
}

func validateChatwootInboundDraft(draft ports.ChatwootInboundDraft) error {
	if strings.TrimSpace(draft.RouteKey) == "" || strings.TrimSpace(draft.AccountID) == "" || strings.TrimSpace(draft.InboxID) == "" || strings.TrimSpace(draft.ConversationID) == "" || strings.TrimSpace(draft.ExternalUserID) == "" || strings.TrimSpace(draft.EventType) == "" || strings.TrimSpace(draft.RawPayloadReference) == "" || strings.TrimSpace(draft.PayloadHash) == "" {
		return invalidRepositoryInput("chatwoot_inbound.materialize", "route, account, inbox, conversation, external user, event, payload reference, and payload hash are required")
	}
	return nil
}

func findChatwootBinding(ctx context.Context, executor SQLExecutor, draft ports.ChatwootInboundDraft) (string, string, error) {
	var bindingID, businessID string
	err := executor.QueryRow(ctx, `SELECT id::text, business_id::text FROM chatwoot_workspace_bindings WHERE route_key = $1 AND account_id = $2 AND inbox_id = $3 AND active`, draft.RouteKey, draft.AccountID, draft.InboxID).Scan(&bindingID, &businessID)
	if err != nil {
		return "", "", classifyRepositoryGetError("chatwoot_binding.get", err)
	}
	return bindingID, businessID, nil
}

func lockChatwootKey(ctx context.Context, executor SQLExecutor, key string) error {
	if _, err := executor.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
		return classifyRepositoryWriteError("chatwoot_inbound.lock", err)
	}
	return nil
}

func recordChatwootInboundEvent(ctx context.Context, executor SQLExecutor, businessID string, draft ports.ChatwootInboundDraft) (string, bool, error) {
	var eventID string
	ledgerID := uuid.NewString()
	connectionReference := draft.AccountID + ":" + draft.InboxID
	err := executor.QueryRow(ctx, `INSERT INTO inbound_event_ledger (id, provider_ref, provider_connection_ref, provider_event_id, dedupe_strategy, business_id, connection_id, event_type, interaction_kind, provider_message_id, provider_conversation_id, external_user_id, content_reference, received_at, raw_payload_reference, payload_hash, signature_verified, processing_state, created_at, updated_at) VALUES ($1::uuid, 'chatwoot', $2, $3, 'provider_event_id', $4::uuid, NULL, $5, 'dm', NULLIF($6, ''), $7, $8, $9, $10, $9, $11, true, 'received', $10, $10) ON CONFLICT (provider_ref, provider_connection_ref, provider_event_id) DO NOTHING RETURNING id::text`, ledgerID, connectionReference, draft.EventID, businessID, draft.EventType, draft.ProviderMessageID, draft.ConversationID, draft.ExternalUserID, draft.RawPayloadReference, draft.ReceivedAt, draft.PayloadHash).Scan(&eventID)
	if err == nil {
		return eventID, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, classifyRepositoryWriteError("chatwoot_inbound.record_event", err)
	}
	if err := executor.QueryRow(ctx, `SELECT id::text FROM inbound_event_ledger WHERE provider_ref = 'chatwoot' AND provider_connection_ref = $1 AND provider_event_id = $2`, connectionReference, draft.EventID).Scan(&eventID); err != nil {
		return "", false, classifyRepositoryGetError("chatwoot_inbound.get_duplicate_event", err)
	}
	return eventID, false, nil
}

func findOrCreateChatwootCustomer(ctx context.Context, executor SQLExecutor, bindingID, businessID string, draft ports.ChatwootInboundDraft) (string, error) {
	var customerID string
	err := executor.QueryRow(ctx, `SELECT customer_id::text FROM chatwoot_contact_links WHERE binding_id = $1::uuid AND external_user_id = $2`, bindingID, draft.ExternalUserID).Scan(&customerID)
	if err == nil {
		return customerID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", classifyRepositoryGetError("chatwoot_contact_link.get", err)
	}
	customerID = uuid.NewString()
	profile := fmt.Sprintf(`{"source":"chatwoot","external_user_id":%q}`, draft.ExternalUserID)
	if _, err := executor.Exec(ctx, `INSERT INTO customers (id, business_id, profile, contact_points, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::jsonb, '[]'::jsonb, 'active', $4, $4)`, customerID, businessID, profile, draft.ReceivedAt); err != nil {
		return "", classifyRepositoryWriteError("chatwoot_customer.create", err)
	}
	if _, err := executor.Exec(ctx, `INSERT INTO chatwoot_contact_links (id, binding_id, business_id, customer_id, external_user_id, chatwoot_contact_id, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $5, $6, $6)`, uuid.NewString(), bindingID, businessID, customerID, draft.ExternalUserID, draft.ReceivedAt); err != nil {
		return "", classifyRepositoryWriteError("chatwoot_contact_link.create", err)
	}
	return customerID, nil
}

func findOrCreateChatwootConversation(ctx context.Context, executor SQLExecutor, businessID, customerID string, draft ports.ChatwootInboundDraft) (string, string, error) {
	if err := lockChatwootKey(ctx, executor, "conversation:"+businessID+":"+draft.ConversationID); err != nil {
		return "", "", err
	}
	var conversationID, referenceID string
	err := executor.QueryRow(ctx, `SELECT id::text, conversation_id::text FROM conversation_references WHERE business_id = $1::uuid AND system = 'chatwoot' AND provider_ref = 'chatwoot' AND resource_type = 'conversation' AND resource_id = $2 AND is_current`, businessID, draft.ConversationID).Scan(&referenceID, &conversationID)
	if err == nil {
		return conversationID, referenceID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", classifyRepositoryGetError("chatwoot_conversation_reference.get", err)
	}
	conversationID = uuid.NewString()
	if _, err := executor.Exec(ctx, `INSERT INTO conversations (id, business_id, customer_id, state, ownership, priority, last_activity_at, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'open', 'none', 'normal', $4, $4, $4)`, conversationID, businessID, customerID, draft.OccurredAt); err != nil {
		return "", "", classifyRepositoryWriteError("chatwoot_conversation.create", err)
	}
	referenceID = uuid.NewString()
	if _, err := executor.Exec(ctx, `INSERT INTO conversation_references (id, business_id, conversation_id, system, provider_ref, resource_type, resource_id, connection_id, conversation_kind, is_current, mapping_status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'chatwoot', 'chatwoot', 'conversation', $4, NULL, 'dm', true, 'active', $5, $5)`, referenceID, businessID, conversationID, draft.ConversationID, draft.ReceivedAt); err != nil {
		return "", "", classifyRepositoryWriteError("chatwoot_conversation_reference.create", err)
	}
	return conversationID, referenceID, nil
}

func recordChatwootCommunicationMessage(ctx context.Context, executor SQLExecutor, businessID, conversationID, referenceID, inboundEventID string, draft ports.ChatwootInboundDraft) (string, error) {
	if strings.TrimSpace(draft.ProviderMessageID) == "" {
		return "", nil
	}
	messageID := uuid.NewString()
	direction := draft.Direction
	if direction != "inbound" && direction != "outbound" {
		direction = "inbound"
	}
	origin := draft.Origin
	if origin == "" {
		if direction == "outbound" {
			origin = "human"
		} else {
			origin = "customer"
		}
	}
	contentType := "unknown"
	var textContent any
	if strings.TrimSpace(draft.Content) != "" {
		contentType = "text"
		textContent = draft.Content
	}
	contentReference := draft.RawPayloadReference
	if _, err := executor.Exec(ctx, `INSERT INTO communication_messages (id, business_id, conversation_reference_id, inbound_event_id, direction, origin, transport, provider_message_id, chatwoot_message_id, content_type, text_content, content_reference, occurred_at, created_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, 'chatwoot', $7, $7, $8, $9, $10, $11, $12)`, messageID, businessID, referenceID, inboundEventID, direction, origin, draft.ProviderMessageID, contentType, textContent, contentReference, draft.OccurredAt, draft.ReceivedAt); err != nil {
		return "", classifyRepositoryWriteError("chatwoot_communication_message.create", err)
	}
	_ = conversationID
	return messageID, nil
}

func findMaterializedChatwootReferences(ctx context.Context, executor SQLExecutor, businessID string, draft ports.ChatwootInboundDraft) (string, string, string, string) {
	var customerID, conversationID, referenceID, messageID string
	_ = executor.QueryRow(ctx, `SELECT c.customer_id::text, c.id::text, r.id::text, COALESCE(m.id::text, '') FROM conversations c JOIN conversation_references r ON r.business_id = c.business_id AND r.conversation_id = c.id LEFT JOIN communication_messages m ON m.business_id = c.business_id AND m.conversation_reference_id = r.id AND m.chatwoot_message_id = NULLIF($3, '') WHERE c.business_id = $1::uuid AND r.system = 'chatwoot' AND r.provider_ref = 'chatwoot' AND r.resource_type = 'conversation' AND r.resource_id = $2 AND r.is_current LIMIT 1`, businessID, draft.ConversationID, draft.ProviderMessageID).Scan(&customerID, &conversationID, &referenceID, &messageID)
	return customerID, conversationID, referenceID, messageID
}

var _ ports.ChatwootInboundStore = (*ChatwootInboundStore)(nil)

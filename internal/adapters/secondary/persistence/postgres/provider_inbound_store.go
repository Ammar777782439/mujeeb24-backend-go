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

type ProviderInboundStore struct{ adapter *Adapter }

func NewProviderInboundStore(adapter *Adapter) *ProviderInboundStore {
	return &ProviderInboundStore{adapter: adapter}
}

func (s *ProviderInboundStore) Materialize(ctx context.Context, draft ports.ProviderInboundDraft) (ports.ProviderInboundResult, error) {
	if s == nil || s.adapter == nil {
		return ports.ProviderInboundResult{}, ErrPoolClosed
	}
	if draft.ReceivedAt.IsZero() {
		draft.ReceivedAt = time.Now().UTC()
	}
	if draft.ExternalCreatedAt == nil {
		timestamp := draft.ReceivedAt
		draft.ExternalCreatedAt = &timestamp
	}
	if err := validateProviderInboundDraft(draft); err != nil {
		return ports.ProviderInboundResult{}, err
	}

	var result ports.ProviderInboundResult
	err := s.adapter.Within(ctx, func(txCtx context.Context) error {
		executor, err := s.adapter.Executor(txCtx)
		if err != nil {
			return err
		}
		state, businessID, connectionID, providerRef, providerAccountRef, providerEventID, providerMessageID, providerConversationID, externalUserID, err := getProviderInboundLedger(txCtx, executor, draft.InboundEventID)
		if err != nil {
			return err
		}
		if businessID != draft.BusinessID || connectionID != draft.ConnectionID || providerRef != draft.ProviderRef || providerAccountRef != draft.ProviderAccountRef || providerEventID != draft.ProviderEventID || providerMessageID != draft.ProviderMessageID || providerConversationID != draft.ProviderConversationID || externalUserID != draft.ExternalUserID {
			return &RepositoryError{Operation: "provider_inbound.materialize", Kind: RepositoryConflict, Err: errors.New("inbound event tenant or connection does not match draft")}
		}
		if state == "processed" {
			result = findProviderMaterialization(txCtx, executor, businessID, draft)
			result.Duplicate = true
			result.InboundEventID = draft.InboundEventID
			return nil
		}
		if state != "received" {
			return &RepositoryError{Operation: "provider_inbound.materialize", Kind: RepositoryConflict, Err: fmt.Errorf("inbound event state %s is not materializable", state)}
		}
		result.BusinessID = businessID
		result.InboundEventID = draft.InboundEventID
		if draft.EventType != "interaction_received" || draft.ProviderConversationID == "" || draft.ExternalUserID == "" || draft.ProviderMessageID == "" {
			return markProviderInboundProcessed(txCtx, executor, draft.InboundEventID, "socialapi_ignored_event", &result)
		}
		if err := lockProviderInboundKey(txCtx, executor, "identity:"+draft.ConnectionID+":"+draft.ExternalUserID); err != nil {
			return err
		}
		if err := lockProviderInboundKey(txCtx, executor, "conversation:"+draft.ConnectionID+":"+draft.ProviderConversationID); err != nil {
			return err
		}
		customerID, err := findOrCreateProviderCustomer(txCtx, executor, draft)
		if err != nil {
			return err
		}
		conversationID, referenceID, err := findOrCreateProviderConversation(txCtx, executor, customerID, draft)
		if err != nil {
			return err
		}
		messageID, err := recordProviderCommunicationMessage(txCtx, executor, referenceID, draft)
		if err != nil {
			return err
		}
		result.CustomerID = customerID
		result.ConversationID = conversationID
		result.ConversationReferenceID = referenceID
		result.CommunicationMessageID = messageID
		return markProviderInboundProcessed(txCtx, executor, draft.InboundEventID, "socialapi_materialized", &result)
	})
	if err != nil {
		return ports.ProviderInboundResult{}, err
	}
	return result, nil
}

func validateProviderInboundDraft(draft ports.ProviderInboundDraft) error {
	if strings.TrimSpace(draft.InboundEventID) == "" || strings.TrimSpace(draft.BusinessID) == "" || strings.TrimSpace(draft.ConnectionID) == "" || strings.TrimSpace(draft.ProviderRef) == "" || strings.TrimSpace(draft.ProviderEventID) == "" || strings.TrimSpace(draft.Channel) == "" || strings.TrimSpace(draft.ProviderAccountRef) == "" || strings.TrimSpace(draft.EventType) == "" || strings.TrimSpace(draft.RawPayloadReference) == "" || strings.TrimSpace(draft.PayloadHash) == "" || draft.ReceivedAt.IsZero() {
		return invalidRepositoryInput("provider_inbound.materialize", "event, business, connection, provider, channel, account, payload reference/hash, and timestamp are required")
	}
	return nil
}

func getProviderInboundLedger(ctx context.Context, executor SQLExecutor, eventID string) (string, string, string, string, string, string, string, string, string, error) {
	var state, businessID, connectionID, providerRef, providerAccountRef, providerEventID string
	var providerMessageID, providerConversationID, externalUserID *string
	if err := executor.QueryRow(ctx, `SELECT processing_state, business_id::text, connection_id::text, provider_ref, provider_connection_ref, provider_event_id, provider_message_id, provider_conversation_id, external_user_id FROM inbound_event_ledger WHERE id = $1::uuid`, eventID).Scan(&state, &businessID, &connectionID, &providerRef, &providerAccountRef, &providerEventID, &providerMessageID, &providerConversationID, &externalUserID); err != nil {
		return "", "", "", "", "", "", "", "", "", classifyRepositoryGetError("provider_inbound.ledger.get", err)
	}
	providerMessage := ""
	if providerMessageID != nil {
		providerMessage = *providerMessageID
	}
	providerConversation := ""
	if providerConversationID != nil {
		providerConversation = *providerConversationID
	}
	externalUser := ""
	if externalUserID != nil {
		externalUser = *externalUserID
	}
	if providerRef != "socialapi" || businessID == "" || connectionID == "" {
		return "", "", "", "", "", "", "", "", "", &RepositoryError{Operation: "provider_inbound.ledger.get", Kind: RepositoryConflict, Err: errors.New("ledger event is not a resolved SocialAPI event")}
	}
	return state, businessID, connectionID, providerRef, providerAccountRef, providerEventID, providerMessage, providerConversation, externalUser, nil
}

func lockProviderInboundKey(ctx context.Context, executor SQLExecutor, key string) error {
	if _, err := executor.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
		return classifyRepositoryWriteError("provider_inbound.lock", err)
	}
	return nil
}

func findOrCreateProviderCustomer(ctx context.Context, executor SQLExecutor, draft ports.ProviderInboundDraft) (string, error) {
	var customerID *string
	err := executor.QueryRow(ctx, `SELECT customer_id::text FROM external_identities WHERE business_id = $1::uuid AND connection_id = $2::uuid AND external_user_id = $3 AND link_status = 'linked'`, draft.BusinessID, draft.ConnectionID, draft.ExternalUserID).Scan(&customerID)
	if err == nil && customerID != nil && strings.TrimSpace(*customerID) != "" {
		return *customerID, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", classifyRepositoryGetError("provider_inbound.identity.get", err)
	}
	if err == nil && customerID == nil {
		return "", &RepositoryError{Operation: "provider_inbound.identity.get", Kind: RepositoryConflict, Err: errors.New("linked external identity has no customer")}
	}
	customerIDValue := uuid.NewString()
	profile := fmt.Sprintf(`{"source":"socialapi","provider_ref":%q,"external_user_id":%q}`, draft.ProviderRef, draft.ExternalUserID)
	if _, err := executor.Exec(ctx, `INSERT INTO customers (id, business_id, profile, contact_points, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::jsonb, '[]'::jsonb, 'active', $4, $4)`, customerIDValue, draft.BusinessID, profile, draft.ReceivedAt); err != nil {
		return "", classifyRepositoryWriteError("provider_inbound.customer.create", err)
	}
	if _, err := executor.Exec(ctx, `INSERT INTO external_identities (id, business_id, connection_id, customer_id, provider_ref, channel, external_account_ref, external_user_id, profile_snapshot_reference, link_status, first_seen_at, last_seen_at, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7, $8, $9, 'linked', $10, $10, $10, $10)`, uuid.NewString(), draft.BusinessID, draft.ConnectionID, customerIDValue, draft.ProviderRef, draft.Channel, draft.ProviderAccountRef, draft.ExternalUserID, draft.RawPayloadReference, draft.ReceivedAt); err != nil {
		return "", classifyRepositoryWriteError("provider_inbound.identity.create", err)
	}
	return customerIDValue, nil
}

func findOrCreateProviderConversation(ctx context.Context, executor SQLExecutor, customerID string, draft ports.ProviderInboundDraft) (string, string, error) {
	var conversationID, referenceID, existingCustomerID string
	err := executor.QueryRow(ctx, `SELECT r.id::text, r.conversation_id::text, c.customer_id::text FROM conversation_references r JOIN conversations c ON c.business_id = r.business_id AND c.id = r.conversation_id WHERE r.business_id = $1::uuid AND r.system = 'provider' AND r.provider_ref = $2 AND r.resource_type = 'conversation' AND r.resource_id = $3 AND r.connection_id = $4::uuid AND r.is_current AND r.mapping_status = 'active'`, draft.BusinessID, draft.ProviderRef, draft.ProviderConversationID, draft.ConnectionID).Scan(&referenceID, &conversationID, &existingCustomerID)
	if err == nil {
		if existingCustomerID != customerID {
			return "", "", &RepositoryError{Operation: "provider_inbound.conversation.get", Kind: RepositoryConflict, Err: errors.New("provider conversation is linked to another customer")}
		}
		if _, err := executor.Exec(ctx, `UPDATE conversations SET last_activity_at = $2, updated_at = $2 WHERE business_id = $1::uuid AND id = $3::uuid`, draft.BusinessID, draft.ReceivedAt, conversationID); err != nil {
			return "", "", classifyRepositoryWriteError("provider_inbound.conversation.touch", err)
		}
		return conversationID, referenceID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", classifyRepositoryGetError("provider_inbound.conversation.get", err)
	}
	conversationID = uuid.NewString()
	if _, err := executor.Exec(ctx, `INSERT INTO conversations (id, business_id, customer_id, state, ownership, priority, last_activity_at, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'open', 'none', 'normal', $4, $4, $4)`, conversationID, draft.BusinessID, customerID, draft.ReceivedAt); err != nil {
		return "", "", classifyRepositoryWriteError("provider_inbound.conversation.create", err)
	}
	referenceID = uuid.NewString()
	kind := draft.InteractionKind
	if kind == "" {
		kind = "other"
	}
	if _, err := executor.Exec(ctx, `INSERT INTO conversation_references (id, business_id, conversation_id, system, provider_ref, resource_type, resource_id, connection_id, conversation_kind, is_current, mapping_status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'provider', $4, 'conversation', $5, $6::uuid, $7, true, 'active', $8, $8)`, referenceID, draft.BusinessID, conversationID, draft.ProviderRef, draft.ProviderConversationID, draft.ConnectionID, kind, draft.ReceivedAt); err != nil {
		return "", "", classifyRepositoryWriteError("provider_inbound.reference.create", err)
	}
	return conversationID, referenceID, nil
}

func recordProviderCommunicationMessage(ctx context.Context, executor SQLExecutor, referenceID string, draft ports.ProviderInboundDraft) (string, error) {
	var messageID string
	err := executor.QueryRow(ctx, `INSERT INTO communication_messages (id, business_id, conversation_reference_id, inbound_event_id, direction, origin, transport, provider_message_id, content_type, text_content, content_reference, occurred_at, created_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, 'inbound', 'customer', 'provider', $5, $6, $7, $8, $9, $10) ON CONFLICT DO NOTHING RETURNING id::text`, uuid.NewString(), draft.BusinessID, referenceID, draft.InboundEventID, draft.ProviderMessageID, providerContentType(draft.Text), nullableText(draft.Text), draft.RawPayloadReference, eventOccurredAt(draft), draft.ReceivedAt).Scan(&messageID)
	if err == nil {
		return messageID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", classifyRepositoryWriteError("provider_inbound.message.create", err)
	}
	if err := executor.QueryRow(ctx, `SELECT id::text FROM communication_messages WHERE business_id = $1::uuid AND inbound_event_id = $2::uuid ORDER BY created_at DESC LIMIT 1`, draft.BusinessID, draft.InboundEventID).Scan(&messageID); err != nil {
		return "", classifyRepositoryGetError("provider_inbound.message.get_duplicate", err)
	}
	return messageID, nil
}

func providerContentType(text string) string {
	if strings.TrimSpace(text) == "" {
		return "unknown"
	}
	return "text"
}

func nullableText(text string) any {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return text
}

func eventOccurredAt(draft ports.ProviderInboundDraft) time.Time {
	if draft.ExternalCreatedAt != nil && !draft.ExternalCreatedAt.IsZero() {
		return draft.ExternalCreatedAt.UTC()
	}
	return draft.ReceivedAt
}

func markProviderInboundProcessed(ctx context.Context, executor SQLExecutor, eventID, resultCode string, result *ports.ProviderInboundResult) error {
	commandTag, err := executor.Exec(ctx, `UPDATE inbound_event_ledger SET processing_state = 'processed', processing_result_code = $2, processed_at = $3, updated_at = $3 WHERE id = $1::uuid AND processing_state = 'received'`, eventID, resultCode, time.Now().UTC())
	if err != nil {
		return classifyRepositoryWriteError("provider_inbound.ledger.mark_processed", err)
	}
	if commandTag.RowsAffected() == 0 {
		result.Duplicate = true
	}
	if resultCode == "socialapi_ignored_event" {
		result.Ignored = true
	}
	return nil
}

func findProviderMaterialization(ctx context.Context, executor SQLExecutor, businessID string, draft ports.ProviderInboundDraft) ports.ProviderInboundResult {
	result := ports.ProviderInboundResult{BusinessID: businessID, InboundEventID: draft.InboundEventID}
	_ = executor.QueryRow(ctx, `SELECT c.customer_id::text, c.id::text, r.id::text, COALESCE(m.id::text, '') FROM conversations c JOIN conversation_references r ON r.business_id = c.business_id AND r.conversation_id = c.id LEFT JOIN communication_messages m ON m.business_id = c.business_id AND m.conversation_reference_id = r.id AND m.inbound_event_id = $4::uuid WHERE c.business_id = $1::uuid AND r.system = 'provider' AND r.provider_ref = $2 AND r.resource_type = 'conversation' AND r.resource_id = $3 AND r.is_current LIMIT 1`, businessID, draft.ProviderRef, draft.ProviderConversationID, draft.InboundEventID).Scan(&result.CustomerID, &result.ConversationID, &result.ConversationReferenceID, &result.CommunicationMessageID)
	return result
}

var _ ports.ProviderInboundStore = (*ProviderInboundStore)(nil)

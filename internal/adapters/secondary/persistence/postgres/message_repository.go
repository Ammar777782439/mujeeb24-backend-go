package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

type MessageRepository struct{ adapter *Adapter }

func NewMessageRepository(adapter *Adapter) *MessageRepository {
	return &MessageRepository{adapter: adapter}
}

type messageCursor struct {
	OccurredAt time.Time
	CreatedAt  time.Time
	ID         string
}

func (r *MessageRepository) Record(ctx context.Context, draft ports.CommunicationMessageDraft) (ports.CommunicationMessageRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.CommunicationMessageRecord{}, ErrPoolClosed
	}
	if draft.ID == "" || draft.BusinessID == "" || draft.ConversationReferenceID == "" || draft.Direction == "" || draft.Origin == "" || draft.Transport == "" || draft.ContentType == "" || draft.ContentReference == "" || draft.OccurredAt.IsZero() || draft.CreatedAt.IsZero() {
		return ports.CommunicationMessageRecord{}, invalidRepositoryInput("communication_message.record", "required message fields are missing")
	}
	if draft.Visibility == "" {
		draft.Visibility = "public"
	}
	if draft.Visibility != "public" && draft.Visibility != "private" {
		return ports.CommunicationMessageRecord{}, invalidRepositoryInput("communication_message.record", "message visibility is invalid")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.CommunicationMessageRecord{}, err
	}
	const query = `INSERT INTO communication_messages (id, business_id, conversation_reference_id, inbound_event_id, outbound_message_id, direction, origin, transport, provider_message_id, content_type, text_content, content_reference, visibility, occurred_at, created_at, author_principal_id) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16::uuid) RETURNING id::text, business_id::text, (SELECT r.conversation_id::text FROM conversation_references AS r WHERE r.business_id = communication_messages.business_id AND r.id = communication_messages.conversation_reference_id), conversation_reference_id::text, inbound_event_id::text, outbound_message_id::text, direction, origin, transport, provider_message_id, content_type, text_content, content_reference, visibility, occurred_at, created_at, COALESCE((SELECT o.status FROM outbound_messages AS o WHERE o.business_id = communication_messages.business_id AND o.id = communication_messages.outbound_message_id), CASE WHEN direction = 'inbound' THEN 'received' ELSE 'recorded' END), author_principal_id::text, (SELECT p.display_name FROM principals p WHERE p.id=communication_messages.author_principal_id)`
	return scanCommunicationMessage(executor.QueryRow(ctx, query, draft.ID, draft.BusinessID, draft.ConversationReferenceID, optionalStringValue(draft.InboundEventID), optionalStringValue(draft.OutboundMessageID), draft.Direction, draft.Origin, draft.Transport, optionalStringValue(draft.ProviderMessageID), draft.ContentType, optionalStringValue(draft.TextContent), draft.ContentReference, draft.Visibility, draft.OccurredAt, draft.CreatedAt, optionalStringValue(draft.AuthorPrincipalID)))
}

func (r *MessageRepository) ListByConversation(ctx context.Context, businessID, conversationID string, limit int, cursor string) (ports.MessagePage, error) {
	if r == nil || r.adapter == nil {
		return ports.MessagePage{}, ErrPoolClosed
	}
	if businessID == "" || conversationID == "" {
		return ports.MessagePage{}, invalidRepositoryInput("message.list_by_conversation", "business and conversation ids are required")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 10000 {
		return ports.MessagePage{}, invalidRepositoryInput("message.list_by_conversation", "limit must not exceed 10000")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.MessagePage{}, err
	}
	var conversationExists bool
	if err := executor.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM conversations WHERE business_id = $1::uuid AND id = $2::uuid)`, businessID, conversationID).Scan(&conversationExists); err != nil {
		return ports.MessagePage{}, classifyRepositoryGetError("message.list_by_conversation", err)
	}
	if !conversationExists {
		return ports.MessagePage{}, &RepositoryError{Operation: "message.list_by_conversation", Kind: RepositoryNotFound, Err: pgx.ErrNoRows}
	}
	decoded, err := decodeMessageCursor(cursor)
	if err != nil {
		return ports.MessagePage{}, invalidRepositoryInput("message.list_by_conversation", err.Error())
	}
	const query = `SELECT m.id::text, m.business_id::text, r.conversation_id::text, m.conversation_reference_id::text, m.inbound_event_id::text, m.outbound_message_id::text, m.direction, m.origin, m.transport, m.provider_message_id, m.content_type, m.text_content, m.content_reference, m.visibility, m.occurred_at, m.created_at, CASE WHEN EXISTS (SELECT 1 FROM outbox_entries e WHERE e.business_id=m.business_id AND e.outbound_message_id=m.outbound_message_id AND e.status='dead_letter') THEN 'failed' ELSE COALESCE(o.status, CASE WHEN m.direction = 'inbound' THEN 'received' ELSE 'recorded' END) END, m.author_principal_id::text,(SELECT p.display_name FROM principals p WHERE p.id=m.author_principal_id) FROM communication_messages AS m JOIN conversation_references AS r ON r.business_id = m.business_id AND r.id = m.conversation_reference_id LEFT JOIN outbound_messages AS o ON o.business_id = m.business_id AND o.id = m.outbound_message_id WHERE m.business_id = $1::uuid AND r.conversation_id = $2::uuid AND ($3::timestamptz IS NULL OR (m.occurred_at, m.created_at, m.id) < ($3::timestamptz, $4::timestamptz, $5::uuid)) ORDER BY m.occurred_at DESC, m.created_at DESC, m.id DESC LIMIT $6`
	queryLimit := limit + 1
	var occurredAt any
	var createdAt any
	var id any
	if decoded != nil {
		occurredAt, createdAt, id = decoded.OccurredAt, decoded.CreatedAt, decoded.ID
	}
	rows, err := executor.Query(ctx, query, businessID, conversationID, occurredAt, createdAt, id, queryLimit)
	if err != nil {
		return ports.MessagePage{}, &RepositoryError{Operation: "message.list_by_conversation", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.CommunicationMessageRecord, 0, limit)
	for rows.Next() {
		var item ports.CommunicationMessageRecord
		if err := rows.Scan(&item.ID, &item.BusinessID, &item.ConversationID, &item.ConversationReferenceID, &item.InboundEventID, &item.OutboundMessageID, &item.Direction, &item.Origin, &item.Transport, &item.ProviderMessageID, &item.ContentType, &item.TextContent, &item.ContentReference, &item.Visibility, &item.OccurredAt, &item.CreatedAt, &item.Status, &item.AuthorPrincipalID, &item.AuthorDisplayName); err != nil {
			return ports.MessagePage{}, &RepositoryError{Operation: "message.list_by_conversation", Kind: RepositoryInvalid, Err: err}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.MessagePage{}, &RepositoryError{Operation: "message.list_by_conversation", Kind: RepositoryInvalid, Err: err}
	}
	page := ports.MessagePage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		page.NextCursor = encodeMessageCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

func scanCommunicationMessage(row pgx.Row) (ports.CommunicationMessageRecord, error) {
	var record ports.CommunicationMessageRecord
	err := row.Scan(&record.ID, &record.BusinessID, &record.ConversationID, &record.ConversationReferenceID, &record.InboundEventID, &record.OutboundMessageID, &record.Direction, &record.Origin, &record.Transport, &record.ProviderMessageID, &record.ContentType, &record.TextContent, &record.ContentReference, &record.Visibility, &record.OccurredAt, &record.CreatedAt, &record.Status, &record.AuthorPrincipalID, &record.AuthorDisplayName)
	if err == nil {
		return record, nil
	}
	return record, classifyRepositoryWriteError("communication_message.record", err)
}

func optionalStringValue(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func encodeMessageCursor(item ports.CommunicationMessageRecord) string {
	raw := strings.Join([]string{item.OccurredAt.UTC().Format(time.RFC3339Nano), item.CreatedAt.UTC().Format(time.RFC3339Nano), item.ID}, "|")
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}
func decodeMessageCursor(value string) (*messageCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid cursor encoding")
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return nil, errors.New("invalid cursor")
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, errors.New("invalid cursor occurred_at")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[1])
	if err != nil {
		return nil, errors.New("invalid cursor created_at")
	}
	if parts[2] == "" {
		return nil, errors.New("invalid cursor id")
	}
	return &messageCursor{OccurredAt: occurredAt, CreatedAt: createdAt, ID: parts[2]}, nil
}

var _ ports.MessageRepository = (*MessageRepository)(nil)

func (r *MessageRepository) GetPrivateNoteTarget(ctx context.Context, businessID, conversationID, messageID string) (ports.CommunicationMessageRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.CommunicationMessageRecord{}, ErrPoolClosed
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.CommunicationMessageRecord{}, err
	}
	var target ports.CommunicationMessageRecord
	err = executor.QueryRow(ctx, `SELECT m.id::text, m.text_content FROM communication_messages m JOIN conversation_references r ON r.business_id=m.business_id AND r.id=m.conversation_reference_id WHERE m.business_id=$1::uuid AND r.conversation_id=$2::uuid AND m.id=$3::uuid AND m.direction='inbound' AND m.visibility='public'`, businessID, conversationID, messageID).Scan(&target.ID, &target.TextContent)
	if err != nil {
		return ports.CommunicationMessageRecord{}, classifyRepositoryGetError("message.private_note_target", err)
	}
	return target, nil
}

// RecordPrivateNoteOnce uses a transaction-scoped lock so parallel HTTP retries
// observe the committed original note, including its original timestamps.
// The ordinary public/provider message insertion path remains unchanged.
func (r *MessageRepository) RecordPrivateNoteOnce(ctx context.Context, conversationID string, draft ports.CommunicationMessageDraft) (ports.CommunicationMessageRecord, bool, error) {
	if r == nil || r.adapter == nil {
		return ports.CommunicationMessageRecord{}, false, ErrPoolClosed
	}
	if _, ok := transactionFromContext(ctx); !ok {
		return ports.CommunicationMessageRecord{}, false, ErrTransactionExecutorMissing
	}
	if conversationID == "" || draft.ID == "" || draft.BusinessID == "" || draft.Visibility != "private" || draft.Direction != "outbound" || draft.Origin != "human" || draft.Transport != "mujeeb" || draft.ContentType != "text" || draft.TextContent == nil || strings.TrimSpace(*draft.TextContent) == "" {
		return ports.CommunicationMessageRecord{}, false, invalidRepositoryInput("private_note.record_once", "a scoped private note is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.CommunicationMessageRecord{}, false, err
	}
	if _, err := executor.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "private-note/"+draft.BusinessID+"/"+draft.ID); err != nil {
		return ports.CommunicationMessageRecord{}, false, err
	}
	const query = `SELECT m.id::text,m.business_id::text,r.conversation_id::text,m.conversation_reference_id::text,m.inbound_event_id::text,m.outbound_message_id::text,m.direction,m.origin,m.transport,m.provider_message_id,m.content_type,m.text_content,m.content_reference,m.visibility,m.occurred_at,m.created_at,'recorded',m.author_principal_id::text,(SELECT p.display_name FROM principals p WHERE p.id=m.author_principal_id) FROM communication_messages m JOIN conversation_references r ON r.business_id=m.business_id AND r.id=m.conversation_reference_id WHERE m.business_id=$1::uuid AND r.conversation_id=$2::uuid AND m.id=$3::uuid`
	existing, err := scanCommunicationMessage(executor.QueryRow(ctx, query, draft.BusinessID, conversationID, draft.ID))
	if err == nil {
		if existing.Visibility != "private" || existing.Direction != "outbound" || existing.Origin != "human" || existing.Transport != "mujeeb" || existing.ContentType != "text" || existing.ContentReference != draft.ContentReference || existing.TextContent == nil || *existing.TextContent != *draft.TextContent {
			return ports.CommunicationMessageRecord{}, false, &RepositoryError{Operation: "private_note.record_once", Kind: RepositoryConflict, Err: errors.New("idempotency key payload changed")}
		}
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ports.CommunicationMessageRecord{}, false, err
	}
	record, err := r.Record(ctx, draft)
	return record, err == nil, err
}

package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

type conversationCursor struct {
	LastActivityAt time.Time
	ID             string
}

func (r *ConversationRepository) List(ctx context.Context, businessID, state, ownership, channel string, customerID *string, limit int, cursor string) (ports.ConversationPage, error) {
	if r == nil || r.adapter == nil {
		return ports.ConversationPage{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" {
		return ports.ConversationPage{}, invalidRepositoryInput("conversation.list", "business id is required")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		return ports.ConversationPage{}, invalidRepositoryInput("conversation.list", "limit must not exceed 100")
	}
	decoded, err := decodeConversationCursor(cursor)
	if err != nil {
		return ports.ConversationPage{}, invalidRepositoryInput("conversation.list", err.Error())
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ConversationPage{}, err
	}
	var customer any
	if customerID != nil {
		customer = *customerID
	}
	var cursorAt any
	var cursorID any
	if decoded != nil {
		cursorAt, cursorID = decoded.LastActivityAt, decoded.ID
	}
	const query = `SELECT c.id::text,c.business_id::text,c.customer_id::text,cu.profile->>'display_name',c.state,c.ownership,c.ai_mode_override,c.priority,c.assignment_reference,c.resource_version,c.last_activity_at FROM conversations AS c LEFT JOIN customers AS cu ON cu.business_id = c.business_id AND cu.id = c.customer_id WHERE c.business_id=$1::uuid AND ($2='' OR c.state=$2) AND ($3='' OR c.ownership=$3) AND ($4::uuid IS NULL OR c.customer_id=$4::uuid) AND ($5='' OR EXISTS (SELECT 1 FROM conversation_references r JOIN channel_connections cc ON cc.business_id=r.business_id AND cc.id=r.connection_id WHERE r.business_id=c.business_id AND r.conversation_id=c.id AND r.is_current AND cc.channel=$5)) AND ($6::timestamptz IS NULL OR (c.last_activity_at,c.id)<($6::timestamptz,$7::uuid)) ORDER BY c.last_activity_at DESC,c.id DESC LIMIT $8`
	rows, err := executor.Query(ctx, query, businessID, strings.TrimSpace(state), strings.TrimSpace(ownership), customer, strings.TrimSpace(channel), cursorAt, cursorID, limit+1)
	if err != nil {
		return ports.ConversationPage{}, &RepositoryError{Operation: "conversation.list", Kind: RepositoryInvalid, Err: err}
	}
	defer rows.Close()
	items := make([]ports.ConversationRecord, 0, limit)
	for rows.Next() {
		var item ports.ConversationRecord
		if err := rows.Scan(&item.ID, &item.BusinessID, &item.CustomerID, &item.CustomerDisplayName, &item.State, &item.Ownership, &item.AIModeOverride, &item.Priority, &item.AssignmentReference, &item.ResourceVersion, &item.LastActivityAt); err != nil {
			return ports.ConversationPage{}, &RepositoryError{Operation: "conversation.list", Kind: RepositoryInvalid, Err: err}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.ConversationPage{}, &RepositoryError{Operation: "conversation.list", Kind: RepositoryInvalid, Err: err}
	}
	page := ports.ConversationPage{Items: items}
	if len(items) > limit {
		page.HasMore = true
		page.Items = items[:limit]
		page.NextCursor = encodeConversationCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

func (r *ConversationRepository) Update(ctx context.Context, update ports.ConversationUpdate) (ports.ConversationRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ConversationRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(update.BusinessID) == "" || strings.TrimSpace(update.ConversationID) == "" || update.ExpectedVersion <= 0 {
		return ports.ConversationRecord{}, invalidRepositoryInput("conversation.update", "business, conversation, and expected resource version are required")
	}
	if update.State == nil && update.Ownership == nil && update.AIModeOverride == nil && update.Priority == nil && update.AssignmentReference == nil {
		return ports.ConversationRecord{}, invalidRepositoryInput("conversation.update", "at least one field is required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ConversationRecord{}, err
	}
	const query = `UPDATE conversations SET state=COALESCE($4,state),ownership=COALESCE($5,ownership),ai_mode_override=COALESCE($6,ai_mode_override),priority=COALESCE($7,priority),assignment_reference=COALESCE($8,assignment_reference),resource_version=resource_version+1,updated_at=now() WHERE business_id=$1::uuid AND id=$2::uuid AND resource_version=$3 RETURNING id::text,business_id::text,customer_id::text,state,ownership,ai_mode_override,priority,assignment_reference,resource_version,last_activity_at`
	var record ports.ConversationRecord
	err = executor.QueryRow(ctx, query, update.BusinessID, update.ConversationID, update.ExpectedVersion, nilIfBlank(update.State), nilIfBlank(update.Ownership), nilIfBlank(update.AIModeOverride), nilIfBlank(update.Priority), nilIfBlank(update.AssignmentReference)).Scan(&record.ID, &record.BusinessID, &record.CustomerID, &record.State, &record.Ownership, &record.AIModeOverride, &record.Priority, &record.AssignmentReference, &record.ResourceVersion, &record.LastActivityAt)
	if err == nil {
		return record, nil
	}
	return ports.ConversationRecord{}, classifyCoreStaleOrNotFound(ctx, executor, "conversation.update", "conversations", update.BusinessID, update.ConversationID, err)
}

func (r *ConversationRepository) AdvanceVersion(ctx context.Context, businessID, conversationID string, expectedVersion int64) (ports.ConversationRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ConversationRecord{}, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(conversationID) == "" || expectedVersion <= 0 {
		return ports.ConversationRecord{}, invalidRepositoryInput("conversation.advance_version", "business, conversation, and expected resource version are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ConversationRecord{}, err
	}
	const query = `UPDATE conversations SET resource_version=resource_version+1,updated_at=now() WHERE business_id=$1::uuid AND id=$2::uuid AND resource_version=$3 RETURNING id::text,business_id::text,customer_id::text,state,ownership,ai_mode_override,priority,assignment_reference,resource_version,last_activity_at`
	var record ports.ConversationRecord
	err = executor.QueryRow(ctx, query, businessID, conversationID, expectedVersion).Scan(&record.ID, &record.BusinessID, &record.CustomerID, &record.State, &record.Ownership, &record.AIModeOverride, &record.Priority, &record.AssignmentReference, &record.ResourceVersion, &record.LastActivityAt)
	if err == nil {
		return record, nil
	}
	return ports.ConversationRecord{}, classifyCoreStaleOrNotFound(ctx, executor, "conversation.advance_version", "conversations", businessID, conversationID, err)
}

func encodeConversationCursor(record ports.ConversationRecord) string {
	return base64.RawURLEncoding.EncodeToString([]byte(record.LastActivityAt.UTC().Format(time.RFC3339Nano) + "|" + record.ID))
}
func decodeConversationCursor(value string) (*conversationCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, errors.New("invalid conversation cursor")
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 {
		return nil, errors.New("invalid conversation cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, errors.New("invalid conversation cursor")
	}
	if err := uuid.Validate(parts[1]); err != nil {
		return nil, errors.New("invalid conversation cursor")
	}
	return &conversationCursor{LastActivityAt: at, ID: parts[1]}, nil
}

var _ ports.ConversationRuntimeRepository = (*ConversationRepository)(nil)

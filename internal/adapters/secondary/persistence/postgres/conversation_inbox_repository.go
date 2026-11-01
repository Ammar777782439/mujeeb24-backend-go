package postgres

import (
	"context"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

const activeConversationAssignment = `(c.ownership='human' AND NULLIF(btrim(c.assignment_reference),'') IS NOT NULL)`
const inboxVisibility = `($3::boolean OR NOT ` + activeConversationAssignment + ` OR c.assignment_reference=$2)`
const inboxScope = `($4='' OR $4='all' OR ($4='mine' AND ` + activeConversationAssignment + ` AND c.assignment_reference=$2) OR ($4='unassigned' AND NOT ` + activeConversationAssignment + `))`

const inboxColumns = `c.id::text,c.business_id::text,c.customer_id::text,cu.profile->>'display_name',c.state,c.ownership,c.ai_mode_override,c.priority,c.assignment_reference,c.resource_version,c.last_activity_at,c.last_gemini_interaction_id,
 (SELECT p.display_name FROM business_memberships bm JOIN principals p ON p.id=bm.principal_id WHERE bm.business_id=c.business_id AND bm.principal_id::text=c.assignment_reference),
 (SELECT cc.channel FROM conversation_references cr JOIN channel_connections cc ON cc.business_id=cr.business_id AND cc.id=cr.connection_id WHERE cr.business_id=c.business_id AND cr.conversation_id=c.id ORDER BY cr.is_current DESC,cr.created_at DESC,cr.id DESC LIMIT 1),
 last_message.text_content,last_message.direction,last_message.origin,last_message.author_name,last_message.content_type,
 (SELECT count(*)::int FROM communication_messages unread JOIN conversation_references ur ON ur.business_id=unread.business_id AND ur.id=unread.conversation_reference_id WHERE unread.business_id=c.business_id AND ur.conversation_id=c.id AND unread.direction='inbound' AND unread.visibility='public' AND unread.created_at>COALESCE((SELECT read_at FROM conversation_read_cursors crc WHERE crc.business_id=c.business_id AND crc.conversation_id=c.id AND crc.principal_id=NULLIF($2,'')::uuid),'epoch'::timestamptz))`

const inboxFrom = ` FROM conversations c LEFT JOIN customers cu ON cu.business_id=c.business_id AND cu.id=c.customer_id
 LEFT JOIN LATERAL (SELECT left(m.text_content,240) AS text_content,m.direction,m.origin,p.display_name AS author_name,m.content_type FROM communication_messages m JOIN conversation_references mr ON mr.business_id=m.business_id AND mr.id=m.conversation_reference_id LEFT JOIN principals p ON p.id=m.author_principal_id WHERE m.business_id=c.business_id AND mr.conversation_id=c.id AND m.visibility='public' ORDER BY m.occurred_at DESC,m.created_at DESC,m.id DESC LIMIT 1) last_message ON true `

func scanInboxConversation(row pgx.Row) (ports.ConversationRecord, error) {
	var r ports.ConversationRecord
	err := row.Scan(&r.ID, &r.BusinessID, &r.CustomerID, &r.CustomerDisplayName, &r.State, &r.Ownership, &r.AIModeOverride, &r.Priority, &r.AssignmentReference, &r.ResourceVersion, &r.LastActivityAt, &r.LastGeminiInteractionID, &r.AssigneeDisplayName, &r.Channel, &r.LastMessageText, &r.LastMessageDirection, &r.LastMessageOrigin, &r.LastMessageAuthorName, &r.LastMessageContentType, &r.UnreadCount)
	if err != nil {
		return r, classifyRepositoryGetError("conversation.inbox", err)
	}
	return r, nil
}

func (r *ConversationRepository) GetInboxConversation(ctx context.Context, businessID, conversationID string, access ports.ConversationAccess, forUpdate bool) (ports.ConversationRecord, error) {
	if r == nil || r.adapter == nil {
		return ports.ConversationRecord{}, ErrPoolClosed
	}
	if forUpdate {
		if _, ok := transactionFromContext(ctx); !ok {
			return ports.ConversationRecord{}, ErrTransactionExecutorMissing
		}
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ConversationRecord{}, err
	}
	query := `SELECT ` + inboxColumns + inboxFrom + ` WHERE c.business_id=$1::uuid AND ` + inboxVisibility + ` AND c.id=$4::uuid`
	if forUpdate {
		query += ` FOR UPDATE OF c`
	}
	return scanInboxConversation(executor.QueryRow(ctx, query, businessID, access.PrincipalID, access.Supervisor, conversationID))
}

func (r *ConversationRepository) ListInbox(ctx context.Context, f ports.ConversationInboxFilter) (ports.ConversationPage, error) {
	if r == nil || r.adapter == nil {
		return ports.ConversationPage{}, ErrPoolClosed
	}
	if f.Scope != "" && f.Scope != "all" && f.Scope != "mine" && f.Scope != "unassigned" {
		return ports.ConversationPage{}, invalidRepositoryInput("conversation.inbox", "invalid scope")
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 100 {
		f.Limit = 100
	}
	cursor, err := decodeConversationCursor(f.Cursor)
	if err != nil {
		return ports.ConversationPage{}, invalidRepositoryInput("conversation.inbox", err.Error())
	}
	var at, id, customer any
	if cursor != nil {
		at, id = cursor.LastActivityAt, cursor.ID
	}
	if f.CustomerID != nil {
		customer = *f.CustomerID
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ConversationPage{}, err
	}
	query := `SELECT ` + inboxColumns + inboxFrom + ` WHERE c.business_id=$1::uuid AND ` + inboxVisibility + ` AND ` + inboxScope + `
 AND ($5='' OR c.state=$5 OR ($5='human_handling' AND c.state='open' AND c.ownership='human') OR ($5='ai_handling' AND c.state='open' AND c.ownership='ai'))
 AND ($6='' OR c.ownership=$6) AND ($7='' OR EXISTS(SELECT 1 FROM conversation_references r JOIN channel_connections cc ON cc.business_id=r.business_id AND cc.id=r.connection_id WHERE r.business_id=c.business_id AND r.conversation_id=c.id AND cc.channel=$7))
 AND ($8='' OR cu.profile->>'display_name' ILIKE '%'||$8||'%' OR c.id::text ILIKE '%'||$8||'%') AND ($9::uuid IS NULL OR c.customer_id=$9::uuid)
 AND ($10::timestamptz IS NULL OR (c.last_activity_at,c.id)<($10::timestamptz,$11::uuid)) ORDER BY c.last_activity_at DESC,c.id DESC LIMIT $12`
	rows, err := executor.Query(ctx, query, f.BusinessID, f.Access.PrincipalID, f.Access.Supervisor, f.Scope, f.State, f.Ownership, f.Channel, strings.TrimSpace(f.Search), customer, at, id, f.Limit+1)
	if err != nil {
		return ports.ConversationPage{}, err
	}
	defer rows.Close()
	items := make([]ports.ConversationRecord, 0, f.Limit)
	for rows.Next() {
		item, err := scanInboxConversation(rows)
		if err != nil {
			return ports.ConversationPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ports.ConversationPage{}, err
	}
	page := ports.ConversationPage{Items: items}
	if len(items) > f.Limit {
		page.HasMore = true
		page.Items = items[:f.Limit]
		page.NextCursor = encodeConversationCursor(page.Items[len(page.Items)-1])
	}
	return page, nil
}

func (r *ConversationRepository) InboxOverview(ctx context.Context, f ports.ConversationInboxFilter) (ports.ConversationInboxOverview, error) {
	if r == nil || r.adapter == nil {
		return ports.ConversationInboxOverview{}, ErrPoolClosed
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.ConversationInboxOverview{}, err
	}
	result := ports.ConversationInboxOverview{StateCounts: map[string]int{}}
	base := ` FROM conversations c LEFT JOIN customers cu ON cu.business_id=c.business_id AND cu.id=c.customer_id WHERE c.business_id=$1::uuid AND ` + inboxVisibility + ` AND ($4='' OR cu.profile->>'display_name' ILIKE '%'||$4||'%' OR c.id::text ILIKE '%'||$4||'%')`
	err = executor.QueryRow(ctx, `SELECT count(*)::int,count(*) FILTER(WHERE `+activeConversationAssignment+` AND c.assignment_reference=$2)::int,count(*) FILTER(WHERE NOT `+activeConversationAssignment+`)::int`+base, f.BusinessID, f.Access.PrincipalID, f.Access.Supervisor, strings.TrimSpace(f.Search)).Scan(&result.Total, &result.AssignedToMe, &result.Unassigned)
	if err != nil {
		return result, err
	}
	rows, err := executor.Query(ctx, `SELECT CASE WHEN c.state='open' AND c.ownership='human' THEN 'human_handling' WHEN c.state='open' AND c.ownership='ai' THEN 'ai_handling' ELSE c.state END,count(*)::int`+base+` AND ($5='' OR $5='all' OR ($5='mine' AND `+activeConversationAssignment+` AND c.assignment_reference=$2) OR ($5='unassigned' AND NOT `+activeConversationAssignment+`)) GROUP BY 1`, f.BusinessID, f.Access.PrincipalID, f.Access.Supervisor, strings.TrimSpace(f.Search), f.Scope)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return result, err
		}
		result.StateCounts[state] = count
	}
	return result, rows.Err()
}

var _ ports.ConversationInboxRepository = (*ConversationRepository)(nil)

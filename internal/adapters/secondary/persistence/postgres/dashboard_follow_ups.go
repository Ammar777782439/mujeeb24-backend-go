package postgres

import (
	"context"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"strings"
	"time"
)

func (r *DashboardRepository) ListFollowUps(ctx context.Context, businessID string, access ports.ConversationAccess, now time.Time, limit int) ([]ports.DashboardFollowUpRecord, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(access.PrincipalID) == "" {
		return nil, invalidRepositoryInput("dashboard.follow_ups", "business and principal are required")
	}
	if limit < 1 || limit > 6 {
		limit = 6
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	// All correlations include tenant and customer scope. Conversation visibility
	// uses the same predicate as the inbox; no arbitrary customer conversation is substituted.
	const query = `WITH candidates AS (
 SELECT l.id::text AS lead_id,l.customer_id::text AS customer_id,
 COALESCE(NULLIF(btrim(cu.profile->>'display_name'),''),'') AS customer_name,l.status,
 c.id::text AS conversation_id,channel.channel,assignee.display_name,
 NULLIF(left(item.name,200),'') AS product_name,NULLIF(left(btrim(l.qualification_reason),500),'') AS interest_reason,
 NULLIF(left(last_inbound.text_content,600),'') AS last_customer_message,last_inbound.occurred_at,
 l.next_action_at,review.id::text AS workflow_id,l.updated_at,
 CASE WHEN c.state='waiting_human' THEN 'waiting_human'
      WHEN review.id IS NOT NULL THEN 'awaiting_review'
      WHEN l.next_action_at <= $4 THEN 'follow_up_due'
      WHEN c.id IS NOT NULL AND (c.ownership<>'human' OR NULLIF(btrim(c.assignment_reference),'') IS NULL) THEN 'unassigned'
      ELSE l.status END AS attention
 FROM leads l JOIN customers cu ON cu.business_id=l.business_id AND cu.id=l.customer_id AND cu.status='active'
 LEFT JOIN LATERAL (
  SELECT c.* FROM conversations c
  WHERE c.business_id=l.business_id AND c.customer_id=l.customer_id AND ` + inboxVisibility + `
  AND (EXISTS(SELECT 1 FROM conversation_references sr WHERE sr.business_id=l.business_id AND sr.id=l.source_conversation_reference_id AND sr.conversation_id=c.id)
       OR EXISTS(SELECT 1 FROM lead_attributions la WHERE la.business_id=l.business_id AND la.lead_id=l.id AND la.source_conversation_id=c.id))
  ORDER BY EXISTS(SELECT 1 FROM conversation_references sr WHERE sr.business_id=l.business_id AND sr.id=l.source_conversation_reference_id AND sr.conversation_id=c.id) DESC,c.last_activity_at DESC,c.id DESC LIMIT 1
 ) c ON true
 LEFT JOIN LATERAL (
  SELECT a.catalog_item_id FROM lead_attributions a WHERE a.business_id=l.business_id AND a.lead_id=l.id
  AND (a.source_conversation_id=c.id OR (c.id IS NULL AND a.source_conversation_id IS NULL)) ORDER BY a.captured_at DESC,a.id DESC LIMIT 1
 ) attr ON true
 LEFT JOIN catalog_items item ON item.business_id=l.business_id AND item.id=attr.catalog_item_id
 LEFT JOIN LATERAL (
  SELECT cc.channel FROM conversation_references cr JOIN channel_connections cc ON cc.business_id=cr.business_id AND cc.id=cr.connection_id
  WHERE cr.business_id=l.business_id AND cr.conversation_id=c.id ORDER BY cr.is_current DESC,cr.created_at DESC,cr.id DESC LIMIT 1
 ) channel ON true
 LEFT JOIN LATERAL (
  SELECT p.display_name FROM business_memberships bm JOIN principals p ON p.id=bm.principal_id
  WHERE bm.business_id=l.business_id AND bm.status='active' AND c.ownership='human' AND bm.principal_id::text=c.assignment_reference LIMIT 1
 ) assignee ON true
 LEFT JOIN LATERAL (
  SELECT m.text_content,m.occurred_at FROM communication_messages m JOIN conversation_references mr ON mr.business_id=m.business_id AND mr.id=m.conversation_reference_id
  WHERE m.business_id=l.business_id AND mr.conversation_id=c.id AND m.direction='inbound' AND m.visibility='public' AND m.origin='customer'
  ORDER BY m.occurred_at DESC,m.created_at DESC,m.id DESC LIMIT 1
 ) last_inbound ON true
 LEFT JOIN LATERAL (
  SELECT w.id FROM sales_workflows w WHERE w.business_id=l.business_id AND w.customer_id=l.customer_id AND w.state='awaiting_review'
  AND ((w.lead_id=l.id AND (w.conversation_id IS NULL OR w.conversation_id=c.id)) OR (w.lead_id IS NULL AND w.conversation_id=c.id))
  ORDER BY w.updated_at DESC,w.id DESC LIMIT 1
 ) review ON true
 WHERE l.business_id=$1::uuid AND l.status IN ('new','interested','qualified') AND ($3::boolean OR c.id IS NOT NULL)
 AND (review.id IS NOT NULL OR l.next_action_at <= $4 OR c.id IS NULL OR (c.state NOT IN ('closed','waiting_customer') AND (c.ownership<>'ai' OR c.state='waiting_human')))
 ), ranked AS (SELECT candidates.*, row_number() OVER (PARTITION BY customer_id ORDER BY CASE attention WHEN 'waiting_human' THEN 0 WHEN 'awaiting_review' THEN 1 WHEN 'follow_up_due' THEN 2 WHEN 'unassigned' THEN 3 WHEN 'qualified' THEN 4 WHEN 'interested' THEN 5 ELSE 6 END, next_action_at ASC NULLS LAST,updated_at DESC,lead_id DESC) AS customer_rank FROM candidates) SELECT lead_id,customer_id,customer_name,status,conversation_id,channel,display_name,product_name,interest_reason,last_customer_message,occurred_at,next_action_at,workflow_id,attention,updated_at
 FROM ranked WHERE customer_rank=1 ORDER BY CASE attention WHEN 'waiting_human' THEN 0 WHEN 'awaiting_review' THEN 1 WHEN 'follow_up_due' THEN 2 WHEN 'unassigned' THEN 3 WHEN 'qualified' THEN 4 WHEN 'interested' THEN 5 ELSE 6 END,
 next_action_at ASC NULLS LAST,updated_at DESC,lead_id DESC LIMIT $5`
	rows, err := executor.Query(ctx, query, businessID, access.PrincipalID, access.Supervisor, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ports.DashboardFollowUpRecord, 0, limit)
	for rows.Next() {
		var x ports.DashboardFollowUpRecord
		if err := rows.Scan(&x.LeadID, &x.CustomerID, &x.CustomerName, &x.LeadStatus, &x.ConversationID, &x.Channel, &x.AssigneeName, &x.ProductName, &x.InterestReason, &x.LastCustomerMessage, &x.LastCustomerMessageAt, &x.NextActionAt, &x.WorkflowID, &x.Attention, &x.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, x)
	}
	return items, rows.Err()
}

var _ ports.DashboardFollowUpRepository = (*DashboardRepository)(nil)

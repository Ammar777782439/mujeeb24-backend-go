package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"time"
)

func (r *SalesWorkflowRepository) GetWorkflow(ctx context.Context, business, id string) (ports.SalesWorkflowRecord, error) {
	e, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SalesWorkflowRecord{}, err
	}
	return scanSalesWorkflow(e.QueryRow(ctx, `SELECT id::text,customer_id::text,conversation_id::text,lead_id::text,transaction_id::text,COALESCE(pending_action,''),source_message_id::text,state,proposal,policy_version,resource_version FROM sales_workflows WHERE business_id=$1::uuid AND id=$2::uuid`, business, id))
}
func (r *SalesWorkflowRepository) ListWorkflows(ctx context.Context, business, customer, state string, limit int, cursor string) (ports.SalesWorkflowPage, error) {
	var result ports.SalesWorkflowPage
	e, err := r.adapter.Executor(ctx)
	if err != nil {
		return result, err
	}
	limit, decoded, err := salesPageArgs(limit, cursor, "sales.workflow.list")
	if err != nil {
		return result, err
	}
	var at, id any
	if decoded != nil {
		at = decoded.At
		id = decoded.ID
	}
	rows, err := e.Query(ctx, `SELECT id::text,customer_id::text,conversation_id::text,lead_id::text,transaction_id::text,COALESCE(pending_action,''),source_message_id::text,state,proposal,policy_version,resource_version,updated_at FROM sales_workflows WHERE business_id=$1::uuid AND ($2='' OR customer_id::text=$2) AND ($3='' OR state=$3) AND ($4::timestamptz IS NULL OR (updated_at,id)<($4,$5::uuid)) ORDER BY updated_at DESC,id DESC LIMIT $6`, business, customer, state, at, id, limit+1)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	result.Items = []ports.SalesWorkflowRecord{}
	var lastAt time.Time
	for rows.Next() {
		var w ports.SalesWorkflowRecord
		var conversation *string
		var data []byte
		if err := rows.Scan(&w.ID, &w.CustomerID, &conversation, &w.LeadID, &w.TransactionID, &w.PendingAction, &w.SourceMessageID, &w.State, &data, &w.PolicyVersion, &w.ResourceVersion, &lastAt); err != nil {
			return result, err
		}
		if conversation != nil {
			w.ConversationID = *conversation
		}
		if err := json.Unmarshal(data, &w.Proposal); err != nil {
			return result, err
		}
		result.Items = append(result.Items, w)
		if len(result.Items) == limit {
			result.NextCursor = encodeSalesCursor(lastAt, w.ID)
		}
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	result.HasMore = len(result.Items) > limit
	if result.HasMore {
		result.Items = result.Items[:limit]
	} else {
		result.NextCursor = ""
	}
	return result, nil
}
func (r *SalesWorkflowRepository) ReviewWorkflow(ctx context.Context, in ports.SalesWorkflowReview) (ports.SalesWorkflowResult, error) {
	var result ports.SalesWorkflowResult
	e, err := r.adapter.Executor(ctx)
	if err != nil {
		return result, err
	}
	w, err := r.GetWorkflow(ctx, in.BusinessID, in.WorkflowID)
	if err != nil {
		return result, err
	}
	if w.ConversationID == "" {
		return result, invalidRepositoryInput("sales.review", "source history was purged; create a new request")
	}
	// Conversation-first locking agrees with the customer executor and button worker.
	var conv string
	if err := e.QueryRow(ctx, `SELECT id::text FROM conversations WHERE business_id=$1::uuid AND id=$2::uuid FOR UPDATE`, in.BusinessID, w.ConversationID).Scan(&conv); err != nil {
		return result, err
	}
	w, err = scanSalesWorkflow(e.QueryRow(ctx, `SELECT id::text,customer_id::text,conversation_id::text,lead_id::text,transaction_id::text,COALESCE(pending_action,''),source_message_id::text,state,proposal,policy_version,resource_version FROM sales_workflows WHERE business_id=$1::uuid AND id=$2::uuid FOR UPDATE`, in.BusinessID, in.WorkflowID))
	if err != nil {
		return result, err
	}
	if w.ResourceVersion != in.ExpectedVersion {
		return result, &RepositoryError{Operation: "sales.review", Kind: RepositoryStale, Err: errors.New("workflow changed")}
	}
	if w.State != "awaiting_review" {
		return result, invalidRepositoryInput("sales.review", "workflow does not await review")
	}
	if !in.Approved {
		w.State = "declined"
		w.ResourceVersion++
		if err := saveSalesWorkflow(ctx, e, in.BusinessID, w, in.Now); err != nil {
			return result, err
		}
		result.Workflow = w
		return result, nil
	}
	if w.PendingAction != "confirm_transaction" {
		if w.SourceMessageID == nil || len(w.Proposal.EvidenceMessageIDs) == 0 {
			return result, invalidRepositoryInput("sales.review", "customer evidence missing")
		}
		proposal := w.Proposal
		proposal.WorkflowID = w.ID
		proposal.ExpectedVersion = w.ResourceVersion
		capability := ports.CustomerSalesCapability(w.PendingAction)
		if capability != ports.CustomerSalesCapabilityCreateDraft && capability != ports.CustomerSalesCapabilityUpdateDraft && capability != ports.CustomerSalesCapabilityRecordLead {
			return result, invalidRepositoryInput("sales.review", "unavailable pending capability")
		}
		return r.Apply(ctx, ports.SalesWorkflowInput{BusinessID: in.BusinessID, CustomerID: w.CustomerID, ConversationID: w.ConversationID, SourceMessageID: *w.SourceMessageID, IdempotencyKey: fmt.Sprintf("approve:%s:%d", w.ID, w.ResourceVersion), Capability: capability, Proposal: proposal, HumanApproved: true, ActorReference: in.ActorReference, Now: in.Now})
	}
	// Customer accepted the exact agreement; the employee approves its commercial review.
	tr, err := NewTransactionRepository(r.adapter).Get(ctx, in.BusinessID, *w.TransactionID)
	if err != nil {
		return result, err
	}
	if _, err := NewTransactionRepository(r.adapter).DecideReview(ctx, ports.TransactionReviewDecisionPatch{ID: tr.ID, BusinessID: in.BusinessID, ExpectedVersion: tr.ResourceVersion, ReviewerReference: in.ActorReference, Approved: true, Reason: "sales workflow approved", UpdatedAt: in.Now}); err != nil {
		return result, err
	}
	tr, err = NewTransactionRepository(r.adapter).Get(ctx, in.BusinessID, tr.ID)
	if err != nil {
		return result, err
	}
	if _, err := NewTransactionRepository(r.adapter).Confirm(ctx, ports.TransactionConfirmPatch{ID: tr.ID, BusinessID: in.BusinessID, ExpectedVersion: tr.ResourceVersion, EvidenceReference: "human-review:" + w.ID, PolicyVersion: fmt.Sprint(w.PolicyVersion), ConfirmedBy: "human_agent", UpdatedAt: in.Now}); err != nil {
		return result, err
	}
	result.Workflow, err = r.GetWorkflow(ctx, in.BusinessID, w.ID)
	result.TotalAmount = tr.TotalAmount
	result.Currency = tr.Currency
	return result, err
}
func (r *SalesWorkflowRepository) PutRequirement(ctx context.Context, business string, req ports.SalesRequirement) error {
	e, err := r.adapter.Executor(ctx)
	if err != nil {
		return err
	}
	_, err = e.Exec(ctx, `INSERT INTO sales_transaction_requirements(business_id,transaction_type,schema_version,definitions) VALUES($1::uuid,$2,$3,$4::jsonb)`, business, req.TransactionType, req.SchemaVersion, mustSalesJSON(req.Definitions))
	if err != nil {
		return classifyRepositoryWriteError("sales.requirements", err)
	}
	return nil
}
func (r *SalesWorkflowRepository) ListRequirements(ctx context.Context, business string) ([]ports.SalesRequirement, error) {
	e, err := r.adapter.Executor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := e.Query(ctx, `SELECT transaction_type,schema_version,definitions FROM sales_transaction_requirements WHERE business_id=$1::uuid ORDER BY transaction_type,schema_version`, business)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ports.SalesRequirement{}
	for rows.Next() {
		var req ports.SalesRequirement
		var data []byte
		if err := rows.Scan(&req.TransactionType, &req.SchemaVersion, &data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &req.Definitions); err != nil {
			return nil, err
		}
		result = append(result, req)
	}
	return result, rows.Err()
}

var _ ports.SalesWorkflowAdminRepository = (*SalesWorkflowRepository)(nil)

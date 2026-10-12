package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"strconv"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/catalogvalidation"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type SalesWorkflowRepository struct{ adapter *Adapter }

func NewSalesWorkflowRepository(a *Adapter) *SalesWorkflowRepository {
	return &SalesWorkflowRepository{a}
}

func (r *SalesWorkflowRepository) GetContext(ctx context.Context, business, conversation, customer string) (ports.SalesWorkflowContext, error) {
	e, err := r.adapter.Executor(ctx)
	if err != nil {
		return ports.SalesWorkflowContext{}, err
	}
	policy, err := NewBusinessRepository(r.adapter).GetRuntimePolicy(ctx, business)
	if err != nil {
		return ports.SalesWorkflowContext{}, err
	}
	result := ports.SalesWorkflowContext{Enabled: policy.SalesWorkflowEnabled, AllowInteractions: policy.AllowSalesInteractions, Active: []ports.SalesWorkflowRecord{}, Requirements: []ports.SalesRequirement{}}
	if !result.Enabled {
		return result, nil
	}
	rows, err := e.Query(ctx, `SELECT id::text,customer_id::text,conversation_id::text,lead_id::text,transaction_id::text,COALESCE(pending_action,''),source_message_id::text,state,proposal,policy_version,resource_version FROM sales_workflows WHERE business_id=$1::uuid AND customer_id=$2::uuid AND conversation_id=$3::uuid AND state IN ('gathering','draft','awaiting_confirmation','awaiting_review') ORDER BY created_at DESC,id DESC LIMIT 11`, business, customer, conversation)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		w, err := scanSalesWorkflow(rows)
		if err != nil {
			rows.Close()
			return result, err
		}
		result.Active = append(result.Active, w)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return result, err
	}
	if len(result.Active) > 10 {
		return result, invalidRepositoryInput("sales.context", "too many open workflows; a human must select the current request")
	}
	rows, err = e.Query(ctx, `SELECT DISTINCT ON (transaction_type) transaction_type,schema_version,definitions FROM sales_transaction_requirements WHERE business_id=$1::uuid ORDER BY transaction_type,schema_version DESC`, business)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var req ports.SalesRequirement
		var data []byte
		if err := rows.Scan(&req.TransactionType, &req.SchemaVersion, &data); err != nil {
			rows.Close()
			return result, err
		}
		if err := json.Unmarshal(data, &req.Definitions); err != nil {
			rows.Close()
			return result, err
		}
		result.Requirements = append(result.Requirements, req)
	}
	rows.Close()
	return result, rows.Err()
}

func (r *SalesWorkflowRepository) Apply(ctx context.Context, in ports.SalesWorkflowInput) (ports.SalesWorkflowResult, error) {
	var result ports.SalesWorkflowResult
	if _, ok := transactionFromContext(ctx); !ok {
		return result, errors.New("sales workflow requires an application transaction")
	}
	e, err := r.adapter.Executor(ctx)
	if err != nil {
		return result, err
	}
	if in.BusinessID == "" || in.CustomerID == "" || in.ConversationID == "" || in.SourceMessageID == "" || in.IdempotencyKey == "" || in.Now.IsZero() {
		return result, invalidRepositoryInput("sales.apply", "trusted identity, source, idempotency and time required")
	}
	// Serialize this conversation across workers and bind it to the verified customer.
	var conv string
	if err := e.QueryRow(ctx, `SELECT id::text FROM conversations WHERE business_id=$1::uuid AND id=$2::uuid AND customer_id=$3::uuid AND ($4 OR (ownership<>'human' AND state<>'waiting_human')) FOR UPDATE`, in.BusinessID, in.ConversationID, in.CustomerID, in.HumanApproved).Scan(&conv); err != nil {
		return result, classifyRepositoryGetError("sales.conversation", err)
	}
	source, err := salesSourceMessage(ctx, e, in.BusinessID, in.ConversationID, in.CustomerID, in.SourceMessageID)
	if err != nil {
		return result, err
	}
	hash := salesHash(in.Proposal)
	cached, found, err := readSalesCommand(ctx, e, in.BusinessID, "workflow", in.IdempotencyKey, hash)
	if err != nil {
		return result, err
	}
	if found {
		if err := json.Unmarshal(cached, &result); err != nil {
			return result, err
		}
		result.Duplicate = true
		return result, nil
	}
	policy, err := NewBusinessRepository(r.adapter).GetRuntimePolicy(ctx, in.BusinessID)
	if err != nil {
		return result, err
	}
	if !policy.SalesWorkflowEnabled || !policy.AllowSalesInteractions || (policy.AIMode == "disabled" && !in.HumanApproved) {
		return result, invalidRepositoryInput("sales.policy", "sales workflow or interactions are disabled")
	}
	if !in.RequiresApproval && !in.HumanApproved {
		if in.Capability == ports.CustomerSalesCapabilityRecordLead && !policy.AllowAutoLeadCreation {
			return result, invalidRepositoryInput("sales.policy", "automatic lead creation is disabled")
		}
		if in.Capability != ports.CustomerSalesCapabilityRecordLead && !policy.AllowAutoTransactionDraft {
			return result, invalidRepositoryInput("sales.policy", "automatic drafts are disabled")
		}
	}
	w := ports.SalesWorkflowRecord{ID: uuid.NewString(), CustomerID: in.CustomerID, ConversationID: in.ConversationID, State: "gathering", Proposal: in.Proposal, PolicyVersion: policy.ResourceVersion, ResourceVersion: 1, SourceMessageID: &source}
	if in.Proposal.WorkflowID != "" {
		w, err = scanSalesWorkflow(e.QueryRow(ctx, `SELECT id::text,customer_id::text,conversation_id::text,lead_id::text,transaction_id::text,COALESCE(pending_action,''),source_message_id::text,state,proposal,policy_version,resource_version FROM sales_workflows WHERE business_id=$1::uuid AND id=$2::uuid AND customer_id=$3::uuid AND conversation_id=$4::uuid FOR UPDATE`, in.BusinessID, in.Proposal.WorkflowID, in.CustomerID, in.ConversationID))
		if err != nil {
			return result, err
		}
		if w.ResourceVersion != in.Proposal.ExpectedVersion {
			return result, &RepositoryError{Operation: "sales.workflow", Kind: RepositoryStale, Err: errors.New("workflow changed")}
		}
		if w.State == "confirmed" || w.State == "cancelled" {
			return result, invalidRepositoryInput("sales.workflow", "workflow is closed")
		}
		w.ResourceVersion++
		w.Proposal = in.Proposal
		w.PolicyVersion = policy.ResourceVersion
		w.SourceMessageID = &source
	} else {
		if in.Capability == ports.CustomerSalesCapabilityUpdateDraft {
			return result, invalidRepositoryInput("sales.workflow", "update requires an exposed workflow reference")
		}
		if _, err := e.Exec(ctx, `INSERT INTO sales_workflows(id,business_id,customer_id,conversation_id,state,proposal,policy_version,created_at,updated_at) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'gathering',$5::jsonb,$6,$7,$7)`, w.ID, in.BusinessID, in.CustomerID, in.ConversationID, mustSalesJSON(in.Proposal), policy.ResourceVersion, in.Now); err != nil {
			return result, err
		}
	}
	// Approval mode persists a proposal only. No lead or transaction is created yet.
	if in.RequiresApproval {
		w.State = "awaiting_review"
		w.PendingAction = string(in.Capability)
	} else {
		w.PendingAction = ""
		if w.LeadID == nil && (policy.AllowAutoLeadCreation || in.HumanApproved) {
			lead, err := NewLeadRepository(r.adapter).Create(ctx, ports.LeadDraft{ID: uuid.NewString(), BusinessID: in.BusinessID, CustomerID: in.CustomerID, QualificationContext: mustSalesJSON(map[string]any{"evidence_message_ids": in.Proposal.EvidenceMessageIDs}), CreatedBy: salesOrigin(in.HumanApproved), CreatedAt: in.Now, UpdatedAt: in.Now})
			if err != nil {
				return result, err
			}
			w.LeadID = &lead.ID
			if _, err := e.Exec(ctx, `UPDATE leads SET source_conversation_reference_id=(SELECT conversation_reference_id FROM communication_messages WHERE business_id=$1::uuid AND id=$3::uuid),source_channel=(SELECT cc.channel FROM conversation_references cr JOIN channel_connections cc ON cc.business_id=cr.business_id AND cc.id=cr.connection_id WHERE cr.business_id=$1::uuid AND cr.conversation_id=$4::uuid AND cr.is_current AND cr.system='provider' LIMIT 1) WHERE business_id=$1::uuid AND id=$2::uuid`, in.BusinessID, lead.ID, source, in.ConversationID); err != nil {
				return result, err
			}
			if _, err := e.Exec(ctx, `INSERT INTO lead_attributions(id,business_id,lead_id,source_conversation_id,source_interaction_reference,captured_at) VALUES(gen_random_uuid(),$1::uuid,$2::uuid,$3::uuid,$4,$5)`, in.BusinessID, lead.ID, in.ConversationID, source, in.Now); err != nil {
				return result, err
			}
		}
		if in.Capability != ports.CustomerSalesCapabilityRecordLead {
			if err := r.prepareDraft(ctx, e, in, &w, &result); err != nil {
				return result, err
			}
		}
	}
	if err := saveSalesWorkflow(ctx, e, in.BusinessID, w, in.Now); err != nil {
		return result, err
	}
	result.Workflow = w
	if err := writeSalesCommand(ctx, e, in.BusinessID, "workflow", in.IdempotencyKey, hash, result, in.Now); err != nil {
		return result, err
	}
	return result, nil
}

func (r *SalesWorkflowRepository) prepareDraft(ctx context.Context, e SQLExecutor, in ports.SalesWorkflowInput, w *ports.SalesWorkflowRecord, result *ports.SalesWorkflowResult) error {
	if !validSalesType(in.Proposal.TransactionType) || len(in.Proposal.Lines) == 0 {
		return invalidRepositoryInput("sales.draft", "transaction type and lines required")
	}
	var definitions []ports.SalesRequirementDefinition
	var reqJSON []byte
	schemaVersion := 1
	err := e.QueryRow(ctx, `SELECT schema_version,definitions FROM sales_transaction_requirements WHERE business_id=$1::uuid AND transaction_type=$2 ORDER BY schema_version DESC LIMIT 1`, in.BusinessID, in.Proposal.TransactionType).Scan(&schemaVersion, &reqJSON)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if len(reqJSON) > 0 {
		if err := json.Unmarshal(reqJSON, &definitions); err != nil {
			return err
		}
	}
	schema := salesRequirementSchema(definitions)
	if err := catalogvalidation.ValidatePartialAttributes(schema, in.Proposal.TransactionData); err != nil {
		return invalidRepositoryInput("sales.requirements", err.Error())
	}
	complete := catalogvalidation.ValidateAttributes(schema, in.Proposal.TransactionData) == nil
	lines := make([]ports.TransactionLineDraft, 0, len(in.Proposal.Lines))
	for _, l := range in.Proposal.Lines {
		if err := ports.ValidateSalesQuantity(l.Quantity); err != nil {
			return invalidRepositoryInput("sales.quantity", err.Error())
		}
		lines = append(lines, ports.TransactionLineDraft{ID: uuid.NewString(), CatalogItemID: l.ItemID, OfferID: l.OfferID, VariantID: l.VariantID, Quantity: l.Quantity, SelectedAttributes: mustSalesJSON(l.SelectedAttributes)})
	}
	repo := NewTransactionRepository(r.adapter)
	var tr ports.TransactionRecord
	if w.TransactionID == nil {
		tr, err = repo.CreateDraft(ctx, ports.TransactionDraft{ID: uuid.NewString(), BusinessID: in.BusinessID, CustomerID: in.CustomerID, LeadID: w.LeadID, TransactionType: in.Proposal.TransactionType, SchemaVersion: schemaVersion, Lines: lines, CreatedAt: in.Now, UpdatedAt: in.Now})
		if err != nil {
			return err
		}
		w.TransactionID = &tr.ID
	} else {
		tr, err = repo.Get(ctx, in.BusinessID, *w.TransactionID)
		if err != nil {
			return err
		}
		if tr.CustomerID != in.CustomerID || tr.TransactionType != in.Proposal.TransactionType {
			return invalidRepositoryInput("sales.draft", "customer/type cannot change on an existing draft")
		}
		tr, err = repo.UpdateDraft(ctx, ports.TransactionPatch{ID: tr.ID, BusinessID: in.BusinessID, ExpectedVersion: tr.ResourceVersion, Lines: lines, UpdatedAt: in.Now})
		if err != nil {
			return err
		}
	}
	if _, err := e.Exec(ctx, `UPDATE commercial_transactions SET transaction_data=$3::jsonb,schema_version=$4,source_conversation_reference_id=(SELECT id FROM conversation_references WHERE business_id=$1::uuid AND conversation_id=$5::uuid AND is_current AND system='provider' LIMIT 1) WHERE business_id=$1::uuid AND id=$2::uuid`, in.BusinessID, tr.ID, mustSalesJSON(in.Proposal.TransactionData), schemaVersion, in.ConversationID); err != nil {
		return err
	}
	result.TotalAmount = tr.TotalAmount
	result.Currency = tr.Currency
	w.State = "draft"
	rows, err := e.Query(ctx, `SELECT item_name_snapshot,quantity::text,line_total_snapshot::text,currency FROM order_lines WHERE business_id=$1::uuid AND transaction_id=$2::uuid ORDER BY created_at,id`, in.BusinessID, tr.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var l ports.SalesReceiptLine
		if err := rows.Scan(&l.Name, &l.Quantity, &l.Total, &l.Currency); err != nil {
			rows.Close()
			return err
		}
		result.Lines = append(result.Lines, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, d := range definitions {
		if v, ok := in.Proposal.TransactionData[d.Key]; ok {
			result.Details = append(result.Details, ports.SalesReceiptDetail{Label: d.Label, Value: v})
		}
	}
	if !complete {
		return nil
	}
	// Missing a specialized requirement schema blocks automatic confirmation, not draft creation.
	requestID := uuid.NewString()
	accept := "m24.sales:" + requestID + ":accept"
	decline := "m24.sales:" + requestID + ":decline"
	if _, err := e.Exec(ctx, `UPDATE sales_confirmation_requests SET status='superseded',updated_at=$3 WHERE business_id=$1::uuid AND workflow_id=$2::uuid AND status IN ('pending','accepted')`, in.BusinessID, w.ID, in.Now); err != nil {
		return err
	}
	if _, err := e.Exec(ctx, `INSERT INTO sales_confirmation_requests(id,business_id,workflow_id,customer_id,transaction_id,transaction_version,confirm_token_hash,decline_token_hash,status,expires_at,created_at,updated_at) VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,$7,$8,'pending',$9,$10,$10)`, requestID, in.BusinessID, w.ID, in.CustomerID, tr.ID, tr.AgreementVersion, salesHash(accept), salesHash(decline), in.Now.Add(30*time.Minute), in.Now); err != nil {
		return err
	}
	if _, err := e.Exec(ctx, `UPDATE commercial_transactions SET state='awaiting_confirmation' WHERE business_id=$1::uuid AND id=$2::uuid`, in.BusinessID, tr.ID); err != nil {
		return err
	}
	w.State = "awaiting_confirmation"
	result.ConfirmationID = requestID
	return nil
}

func scanSalesWorkflow(row interface{ Scan(...any) error }) (ports.SalesWorkflowRecord, error) {
	var w ports.SalesWorkflowRecord
	var data []byte
	var conversation *string
	err := row.Scan(&w.ID, &w.CustomerID, &conversation, &w.LeadID, &w.TransactionID, &w.PendingAction, &w.SourceMessageID, &w.State, &data, &w.PolicyVersion, &w.ResourceVersion)
	if err != nil {
		return w, classifyRepositoryGetError("sales.workflow", err)
	}
	if conversation != nil {
		w.ConversationID = *conversation
	}
	err = json.Unmarshal(data, &w.Proposal)
	return w, err
}
func saveSalesWorkflow(ctx context.Context, e SQLExecutor, business string, w ports.SalesWorkflowRecord, now time.Time) error {
	_, err := e.Exec(ctx, `UPDATE sales_workflows SET lead_id=$3::uuid,transaction_id=$4::uuid,state=$5,proposal=$6::jsonb,policy_version=$7,resource_version=$8,updated_at=$9,pending_action=NULLIF($10,''),source_message_id=$11::uuid WHERE business_id=$1::uuid AND id=$2::uuid`, business, w.ID, w.LeadID, w.TransactionID, w.State, mustSalesJSON(w.Proposal), w.PolicyVersion, w.ResourceVersion, now, w.PendingAction, w.SourceMessageID)
	return err
}
func salesHash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func mustSalesJSON(v any) []byte {
	if v == nil {
		return []byte(`{}`)
	}
	b, _ := json.Marshal(v)
	if string(b) == "null" {
		return []byte(`{}`)
	}
	return b
}
func validSalesType(t string) bool {
	switch t {
	case "order", "booking", "appointment", "service_request", "reservation", "quote", "subscription":
		return true
	}
	return false
}
func salesSourceMessage(ctx context.Context, e SQLExecutor, business, conversation, customer, source string) (string, error) {
	var id string
	err := e.QueryRow(ctx, `SELECT m.id::text FROM communication_messages m JOIN conversation_references r ON r.business_id=m.business_id AND r.id=m.conversation_reference_id JOIN conversations c ON c.business_id=r.business_id AND c.id=r.conversation_id WHERE m.business_id=$1::uuid AND c.id=$2::uuid AND c.customer_id=$3::uuid AND m.direction='inbound' AND m.origin='customer' AND (m.id::text=$4 OR m.provider_message_id=$4) ORDER BY m.created_at DESC LIMIT 1`, business, conversation, customer, source).Scan(&id)
	return id, classifyRepositoryGetError("sales.source", err)
}
func readSalesCommand(ctx context.Context, e SQLExecutor, business, op, key, hash string) ([]byte, bool, error) {
	if len(key) > 250 {
		return nil, false, invalidRepositoryInput("sales.idempotency", "idempotency key too long")
	}
	if _, err := e.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, business+"/"+op+"/"+key); err != nil {
		return nil, false, err
	}
	var previous string
	var data []byte
	err := e.QueryRow(ctx, `SELECT request_hash,result FROM sales_command_results WHERE business_id=$1::uuid AND operation=$2 AND idempotency_key=$3`, business, op, key).Scan(&previous, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if previous != hash {
		return nil, false, invalidRepositoryInput("sales.idempotency", "key was used for a different request")
	}
	return data, true, nil
}
func writeSalesCommand(ctx context.Context, e SQLExecutor, business, op, key, hash string, result any, now time.Time) error {
	_, err := e.Exec(ctx, `INSERT INTO sales_command_results(business_id,operation,idempotency_key,request_hash,result,created_at) VALUES($1::uuid,$2,$3,$4,$5::jsonb,$6)`, business, op, key, hash, mustSalesJSON(result), now)
	return err
}

func (r *SalesWorkflowRepository) ApplyButton(ctx context.Context, in ports.SalesButtonAction) (ports.SalesWorkflowResult, error) {
	var result ports.SalesWorkflowResult
	if _, ok := transactionFromContext(ctx); !ok {
		return result, errors.New("sales action requires an application transaction")
	}
	parts := strings.Split(in.Payload, ":")
	if len(parts) != 3 || parts[0] != "m24.sales" || (parts[2] != "accept" && parts[2] != "decline") {
		return result, invalidRepositoryInput("sales.action", "invalid action")
	}
	if _, err := uuid.Parse(parts[1]); err != nil {
		return result, invalidRepositoryInput("sales.action", "invalid request reference")
	}
	e, err := r.adapter.Executor(ctx)
	if err != nil {
		return result, err
	}
	// Same lock order as model-driven updates: conversation, workflow, confirmation, transaction.
	var id string
	if err := e.QueryRow(ctx, `SELECT id::text FROM conversations WHERE business_id=$1::uuid AND id=$2::uuid AND customer_id=$3::uuid FOR UPDATE`, in.BusinessID, in.ConversationID, in.CustomerID).Scan(&id); err != nil {
		return result, err
	}
	source, err := salesSourceMessage(ctx, e, in.BusinessID, in.ConversationID, in.CustomerID, in.SourceMessageID)
	if err != nil {
		return result, err
	}
	cached, found, err := readSalesCommand(ctx, e, in.BusinessID, "button", source, salesHash(in.Payload))
	if err != nil {
		return result, err
	}
	if found {
		if err := json.Unmarshal(cached, &result); err != nil {
			return result, err
		}
		result.Duplicate = true
		return result, nil
	}
	var workflowID string
	if err := e.QueryRow(ctx, `SELECT workflow_id::text FROM sales_confirmation_requests WHERE business_id=$1::uuid AND id=$2::uuid AND customer_id=$3::uuid`, in.BusinessID, parts[1], in.CustomerID).Scan(&workflowID); err != nil {
		return result, classifyRepositoryGetError("sales.action", err)
	}
	w, err := scanSalesWorkflow(e.QueryRow(ctx, `SELECT id::text,customer_id::text,conversation_id::text,lead_id::text,transaction_id::text,COALESCE(pending_action,''),source_message_id::text,state,proposal,policy_version,resource_version FROM sales_workflows WHERE business_id=$1::uuid AND id=$2::uuid AND customer_id=$3::uuid AND conversation_id=$4::uuid FOR UPDATE`, in.BusinessID, workflowID, in.CustomerID, in.ConversationID))
	if err != nil {
		return result, err
	}
	var status, acceptHash, declineHash string
	var expires time.Time
	var trVersion int64
	if err := e.QueryRow(ctx, `SELECT status,expires_at,transaction_version,confirm_token_hash,decline_token_hash FROM sales_confirmation_requests WHERE business_id=$1::uuid AND id=$2::uuid FOR UPDATE`, in.BusinessID, parts[1]).Scan(&status, &expires, &trVersion, &acceptHash, &declineHash); err != nil {
		return result, err
	}
	if (parts[2] == "accept" && salesHash(in.Payload) != acceptHash) || (parts[2] == "decline" && salesHash(in.Payload) != declineHash) {
		return result, invalidRepositoryInput("sales.action", "invalid payload")
	}
	// A second event for the same already accepted/declined button has no effects.
	if status == "accepted" || status == "declined" {
		result.Workflow = w
		result.Duplicate = true
		return result, nil
	}
	if status != "pending" || !expires.After(in.Now) {
		return result, invalidRepositoryInput("sales.action", "confirmation expired or was superseded")
	}
	policy, err := NewBusinessRepository(r.adapter).GetRuntimePolicy(ctx, in.BusinessID)
	if err != nil {
		return result, err
	}
	if !policy.SalesWorkflowEnabled || !policy.AllowSalesInteractions {
		return result, invalidRepositoryInput("sales.policy", "sales is disabled")
	}
	repo := NewTransactionRepository(r.adapter)
	tr, err := repo.Get(ctx, in.BusinessID, *w.TransactionID)
	if err != nil {
		return result, err
	}
	if tr.AgreementVersion != trVersion || tr.CustomerID != in.CustomerID || tr.State != "awaiting_confirmation" {
		return result, invalidRepositoryInput("sales.action", "agreement changed")
	}
	outcome := "declined"
	w.State = "declined"
	if parts[2] == "accept" {
		outcome = "accepted"
		w.State = "awaiting_review"
		w.PendingAction = "confirm_transaction"
	}
	if _, err := e.Exec(ctx, `UPDATE sales_confirmation_requests SET status=$3,response_message_id=$4::uuid,updated_at=$5 WHERE business_id=$1::uuid AND id=$2::uuid`, in.BusinessID, parts[1], outcome, source, in.Now); err != nil {
		return result, err
	}
	w.ResourceVersion++
	w.PolicyVersion = policy.ResourceVersion
	if outcome == "accepted" {
		var autoSafe bool
		// Bounded stock/schedule/supplier flows need a reservation executor; none is invented here.
		if err := e.QueryRow(ctx, `SELECT COALESCE(bool_and(o.availability_mode='always_available'),false) FROM order_lines l JOIN offers o ON o.business_id=l.business_id AND o.id=l.offer_id WHERE l.business_id=$1::uuid AND l.transaction_id=$2::uuid`, in.BusinessID, tr.ID).Scan(&autoSafe); err != nil {
			return result, err
		}
		autoSafe = autoSafe && tr.TransactionType == "order"
		if policy.AIMode == "restricted_auto" && policy.AllowAutoConfirmation && !policy.DefaultHumanReview && autoSafe {
			_, err = repo.Confirm(ctx, ports.TransactionConfirmPatch{ID: tr.ID, BusinessID: in.BusinessID, ExpectedVersion: tr.ResourceVersion, EvidenceReference: "confirmation:" + parts[1], PolicyVersion: strconv.FormatInt(policy.ResourceVersion, 10), ConfirmedBy: "customer", UpdatedAt: in.Now})
			if err != nil {
				return result, err
			}
			w.State = "confirmed"
			w.PendingAction = ""
		} else {
			// Review never changes the agreement version; it records approval for that version.
			if _, err := e.Exec(ctx, `UPDATE commercial_transactions SET requires_human_review=true WHERE business_id=$1::uuid AND id=$2::uuid`, in.BusinessID, tr.ID); err != nil {
				return result, err
			}
			if _, err := e.Exec(ctx, `INSERT INTO transaction_reviews(id,business_id,transaction_id,required,status,reason_codes,created_at,updated_at) VALUES(gen_random_uuid(),$1::uuid,$2::uuid,true,'pending','["customer_confirmed"]'::jsonb,$3,$3) ON CONFLICT(business_id,transaction_id) DO UPDATE SET required=true,status='pending',reviewer_reference=NULL,decided_at=NULL,updated_at=$3`, in.BusinessID, tr.ID, in.Now); err != nil {
				return result, err
			}
		}
	}
	if err := saveSalesWorkflow(ctx, e, in.BusinessID, w, in.Now); err != nil {
		return result, err
	}
	result.Workflow = w
	result.TotalAmount = tr.TotalAmount
	result.Currency = tr.Currency
	if err := writeSalesCommand(ctx, e, in.BusinessID, "button", source, salesHash(in.Payload), result, in.Now); err != nil {
		return result, err
	}
	return result, nil
}

var _ ports.SalesWorkflowRepository = (*SalesWorkflowRepository)(nil)

func salesOrigin(human bool) string {
	if human {
		return "human"
	}
	return "ai"
}

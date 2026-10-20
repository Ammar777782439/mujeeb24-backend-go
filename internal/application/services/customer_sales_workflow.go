package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

// The sales executor has no LLM or transport client. All effects share one local transaction.
type CustomerSalesWorkflowService struct {
	SalesStarts ports.SalesStartRepository
	Repository   ports.SalesWorkflowRepository
	Transactions ports.TransactionManager
	References   ports.ConversationReferenceRepository
	Connections  ports.ChannelConnectionRepository
	Outbound     ports.OutboundMessageRepository
	Messages     ports.MessageRepository
	Outbox       ports.OutboxStore
	Audit        ports.AuditEventRepository
}

func (s CustomerSalesWorkflowService) Execute(ctx context.Context, in ports.SalesWorkflowInput) (ports.SalesWorkflowResult, error) {
	result, err := s.Repository.Apply(ctx, in)
	if err != nil {
		return result, err
	}
	if result.Duplicate {
		return result, nil
	}
	if err := s.record(ctx, in.BusinessID, in.DecisionID, in.SourceMessageID, result, in.Now); err != nil {
		return result, err
	}
	// Human approval mode stores a proposal without claiming a sale or sending AI text.
	if !in.RequiresApproval {
		err = s.notify(ctx, in.BusinessID, result, in.IdempotencyKey, in.Now)
	}
	return result, err
}

func (s CustomerSalesWorkflowService) HandleButton(ctx context.Context, in ports.SalesButtonAction) (ports.SalesWorkflowResult, error) {
	var result ports.SalesWorkflowResult
	if s.Repository == nil || s.Transactions == nil {
		return result, fmt.Errorf("sales button handler is not configured")
	}
	err := s.Transactions.Within(ctx, func(tx context.Context) error {
		var err error
		result, err = s.Repository.ApplyButton(tx, in)
		if err != nil || result.Duplicate {
			return err
		}
		if err := s.record(tx, in.BusinessID, "", in.SourceMessageID, result, in.Now); err != nil {
			return err
		}
		return s.notify(tx, in.BusinessID, result, "sales-button:"+in.SourceMessageID, in.Now)
	})
	return result, err
}


 // HandleStartButton consumes customer consent and opens at most one existing workflow.
 // Its transaction never calls AI. The durable start request records continuation_pending
 // for the later conversation-resumption phase.
func (s CustomerSalesWorkflowService) HandleStartButton(ctx context.Context, in ports.SalesButtonAction) (ports.SalesWorkflowResult, error) {
	var result ports.SalesWorkflowResult
	if s.SalesStarts == nil || s.Repository == nil || s.Transactions == nil {
		return result, fmt.Errorf("sales start consent dependencies are required")
	}
	err := s.Transactions.Within(ctx, func(tx context.Context) error {
		decision, err := s.SalesStarts.Decide(tx, in)
		if err != nil || decision.Duplicate || !decision.Accepted {
			return err
		}
		result, err = s.Execute(tx, ports.SalesWorkflowInput{
			BusinessID: decision.BusinessID, CustomerID: decision.CustomerID,
			ConversationID: decision.ConversationID, SourceMessageID: in.SourceMessageID,
			SalesStartRequestID: decision.ID, IdempotencyKey: "sales-start:" + decision.ID,
			Capability: ports.CustomerSalesCapabilityRecordLead,
			Proposal: ports.SalesWorkflowProposal{EvidenceMessageIDs: []string{decision.SourceMessageID}},
			RequiresApproval: decision.RequiresApproval, Now: in.Now,
		})
		return err
	})
	return result, err
}

func (s CustomerSalesWorkflowService) record(ctx context.Context, business, decision, source string, result ports.SalesWorkflowResult, now time.Time) error {
	if s.Audit == nil {
		return fmt.Errorf("sales audit repository is required")
	}
	metadata, _ := json.Marshal(map[string]any{"workflow_id": result.Workflow.ID, "state": result.Workflow.State, "source_message_reference": source, "resource_version": result.Workflow.ResourceVersion})
	resource := result.Workflow.ID
	_, err := s.Audit.Append(ctx, ports.AuditEventDraft{ID: uuid.NewString(), BusinessID: business, ActorType: "system", Action: "sales.workflow." + result.Workflow.State, ResourceType: "sales_workflow", ResourceID: &resource, Metadata: metadata, DecisionReference: optionalSalesString(decision), OccurredAt: now, CreatedAt: now, SchemaVersion: 1, RedactionVersion: 1})
	return err
}

func (s CustomerSalesWorkflowService) notify(ctx context.Context, business string, result ports.SalesWorkflowResult, key string, now time.Time) error {
	if s.Connections == nil || s.References == nil || s.Outbound == nil || s.Outbox == nil || s.Messages == nil {
		return fmt.Errorf("sales notification dependencies are required")
	}
	reference, err := s.References.GetCurrentByConversation(ctx, business, result.Workflow.ConversationID, "provider")
	if err != nil {
		return err
	}
	if reference.ConnectionID == nil {
		return fmt.Errorf("sales connection missing")
	}
	connection, err := s.Connections.GetByID(ctx, business, *reference.ConnectionID)
	if err != nil {
		return err
	}
	if connection.Status != "active" {
		return fmt.Errorf("sales connection is not active")
	}
	message := renderSalesReceipt(result)
	encoded, err := EncodeInteractiveContentReference(message)
	if err != nil {
		return err
	}
	outbound, err := s.Outbound.CreatePending(ctx, ports.OutboundMessageDraft{ID: uuid.NewString(), BusinessID: business, ConversationID: result.Workflow.ConversationID, ConversationReferenceID: reference.ID, ConnectionID: *reference.ConnectionID, ProviderRef: reference.ProviderRef, Channel: connection.Channel, Origin: "system", Transport: "provider", ContentReference: encoded, ProviderIdempotencyKey: key})
	if err != nil {
		return err
	}
	_, err = s.Messages.Record(ctx, ports.CommunicationMessageDraft{ID: uuid.NewString(), BusinessID: business, ConversationReferenceID: reference.ID, OutboundMessageID: &outbound.ID, Direction: "outbound", Origin: "system", Transport: "provider", ContentType: "text", TextContent: &message.Text, ContentReference: encoded, Visibility: "public", OccurredAt: now, CreatedAt: now})
	if err != nil {
		return err
	}
	_, err = s.Outbox.Enqueue(ctx, ports.OutboxEntryDraft{ID: uuid.NewString(), BusinessID: business, OutboundMessageID: outbound.ID, CommandType: OutboundSendCommandType, DedupeKey: key, AvailableAt: now, CreatedAt: now, UpdatedAt: now})
	return err
}

// Receipts describe committed states. Gemini cannot supply success text before execution.
func renderSalesReceipt(r ports.SalesWorkflowResult) ports.InteractiveMessage {
	m := ports.InteractiveMessage{}
	switch r.Workflow.State {
	case "awaiting_confirmation":
		amount := "السعر يحتاج مراجعة"
		if r.TotalAmount != nil && r.Currency != nil {
			amount = *r.TotalAmount + " " + *r.Currency
		}
		var details strings.Builder
		for _, line := range r.Lines {
			fmt.Fprintf(&details, "%s × %s\n", line.Name, line.Quantity)
		}
		for _, d := range r.Details {
			fmt.Fprintf(&details, "%s: %v\n", d.Label, d.Value)
		}
		m.Text = "تفاصيل طلبك:\n" + details.String() + "الإجمالي: " + amount + ". هل تؤكد الطلب؟"
		m.Buttons = []ports.InteractiveButton{{Title: "نعم، أؤكد", Payload: "m24.sales:" + r.ConfirmationID + ":accept"}, {Title: "لا، لا أؤكد", Payload: "m24.sales:" + r.ConfirmationID + ":decline"}}
	case "awaiting_review":
		m.Text = "طلبك بانتظار مراجعة الموظف واعتماده."
		if r.Workflow.PendingAction == "confirm_transaction" {
			m.Text = "وصلت موافقتك على الطلب، وهو بانتظار مراجعة الموظف واعتماده."
		}
	case "confirmed":
		m.Text = "تم اعتماد طلبك."
	case "declined":
		m.Text = "لم يتم اعتماد الطلب بناءً على اختيارك."
	case "draft":
		m.Text = "تم حفظ مسودة طلبك، وتحتاج استكمال البيانات المطلوبة."
		if len(r.MissingFields) > 0 {
			m.Text = "لاستكمال طلبك، نحتاج: " + strings.Join(r.MissingFields, "، ") + "."
		}
		if r.NeedsMerchantReview {
			m.Text += " السعر أو تفاصيل الخدمة تحتاج مراجعة الموظف قبل عرض التأكيد."
		}
	default:
		m.Text = "تم تسجيل طلبك للمتابعة."
	}
	return m
}

func optionalSalesString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

const interactiveContentPrefix = "inline-message:"

func EncodeInteractiveContentReference(message ports.InteractiveMessage) (string, error) {
	if err := validateInteractiveMessage(message); err != nil {
		return "", err
	}
	data, err := json.Marshal(message)
	if err != nil {
		return "", err
	}
	return interactiveContentPrefix + base64.RawURLEncoding.EncodeToString(data), nil
}
func DecodeInteractiveContentReference(reference string) (ports.InteractiveMessage, error) {
	if !strings.HasPrefix(reference, interactiveContentPrefix) {
		text, err := DecodeInlineTextContentReference(reference)
		return ports.InteractiveMessage{Text: text}, err
	}
	var m ports.InteractiveMessage
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(reference, interactiveContentPrefix))
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, err
	}
	return m, validateInteractiveMessage(m)
}
func validateInteractiveMessage(m ports.InteractiveMessage) error {
	if strings.TrimSpace(m.Text) == "" || len(m.Buttons) > 3 {
		return fmt.Errorf("interactive message needs text and at most three buttons")
	}
	seen := map[string]bool{}
	for _, b := range m.Buttons {
		if strings.TrimSpace(b.Title) == "" || len([]rune(b.Title)) > 20 || b.Payload == "" || len(b.Payload) > 1000 || seen[b.Payload] {
			return fmt.Errorf("invalid interactive button")
		}
		seen[b.Payload] = true
	}
	return nil
}

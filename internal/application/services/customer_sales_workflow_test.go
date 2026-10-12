package services

import (
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"strings"
	"testing"
)

func TestInteractiveReceiptBindsButtonsToSavedConfirmation(t *testing.T) {
	total, currency := "5000.0000", "YER"
	r := ports.SalesWorkflowResult{Workflow: ports.SalesWorkflowRecord{State: "awaiting_confirmation"}, ConfirmationID: "request-id", TotalAmount: &total, Currency: &currency, Lines: []ports.SalesReceiptLine{{Name: "سماعة", Quantity: "1"}}}
	m := renderSalesReceipt(r)
	if len(m.Buttons) != 2 || m.Buttons[0].Payload != "m24.sales:request-id:accept" || m.Buttons[1].Payload != "m24.sales:request-id:decline" {
		t.Fatalf("buttons not bound to agreement: %+v", m)
	}
	ref, err := EncodeInteractiveContentReference(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeInteractiveContentReference(ref)
	if err != nil || got.Text != m.Text || got.Buttons[0] != m.Buttons[0] {
		t.Fatalf("outbox content lost interaction: %+v %v", got, err)
	}
	legacy, err := DecodeInteractiveContentReference(EncodeInlineTextContentReference("hello"))
	if err != nil || legacy.Text != "hello" || len(legacy.Buttons) != 0 {
		t.Fatalf("legacy content changed: %+v %v", legacy, err)
	}
}
func TestSalesProposalRejectsPreExecutionClaimsAndUnmatchedSelections(t *testing.T) {
	d := ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityCreateDraft, Sales: &ports.SalesWorkflowProposal{TransactionType: "order", EvidenceMessageIDs: []string{"m"}, Lines: []ports.SalesWorkflowLine{{ItemID: "item", Quantity: "1"}}}, Selected: []ports.SelectedReference{{ItemID: "item"}}}
	if err := validateSalesProposal(d); err != nil {
		t.Fatal(err)
	}
	d.ResponseText = "تم تأكيد الطلب"
	if err := validateSalesProposal(d); err == nil {
		t.Fatal("accepted model success text before execution")
	}
	d.ResponseText = ""
	d.Selected[0].ItemID = "other"
	if err := validateSalesProposal(d); err == nil {
		t.Fatal("accepted a different selection than the draft line")
	}
}

func TestIncompleteSalesReceiptExplainsNextStepWithoutConfirmation(t *testing.T) {
	m := renderSalesReceipt(ports.SalesWorkflowResult{Workflow: ports.SalesWorkflowRecord{State: "draft"}, MissingFields: []string{"موعد الحجز"}, NeedsMerchantReview: true})
	if len(m.Buttons) != 0 || !strings.Contains(m.Text, "موعد الحجز") || !strings.Contains(m.Text, "مراجعة الموظف") {
		t.Fatalf("incomplete draft did not explain next step: %+v", m)
	}
	pending := renderSalesReceipt(ports.SalesWorkflowResult{Workflow: ports.SalesWorkflowRecord{State: "awaiting_review", PendingAction: "create_transaction_draft"}})
	if strings.Contains(pending.Text, "وصلت موافقتك") {
		t.Fatal("proposal review claimed a customer confirmation")
	}
}

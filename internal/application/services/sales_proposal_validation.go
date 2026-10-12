package services

import (
	"encoding/json"
	"fmt"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func validateSalesProposal(d ports.CapabilityDecision) error {
	if d.Sales == nil || d.ResponseText != "" || len(d.Sales.EvidenceMessageIDs) == 0 {
		return fmt.Errorf("sales capability requires structured customer evidence and no pre-execution success text")
	}
	if d.Sales.WorkflowID != "" && d.Sales.ExpectedVersion <= 0 {
		return fmt.Errorf("workflow changes require current version")
	}
	if d.Capability == ports.CustomerSalesCapabilityUpdateDraft && d.Sales.WorkflowID == "" {
		return fmt.Errorf("update requires workflow id")
	}
	if d.Capability == ports.CustomerSalesCapabilityRecordLead {
		if len(d.Sales.Lines) > 0 {
			return fmt.Errorf("record_lead cannot create transaction lines")
		}
		return nil
	}
	if !validTransactionType(d.Sales.TransactionType) || len(d.Sales.Lines) == 0 {
		return fmt.Errorf("draft requires transaction type and lines")
	}
	if len(d.Selected) != len(d.Sales.Lines) {
		return fmt.Errorf("draft catalog references must match lines")
	}
	for i, l := range d.Sales.Lines {
		if err := ports.ValidateSalesQuantity(l.Quantity); err != nil {
			return err
		}
		expected := ports.SelectedReference{ItemID: l.ItemID, VariantID: l.VariantID, OfferID: l.OfferID}
		a, _ := json.Marshal(expected)
		b, _ := json.Marshal(d.Selected[i])
		if string(a) != string(b) {
			return fmt.Errorf("catalog selection differs from draft line")
		}
	}
	return nil
}
func encodeSalesDecisionArguments(d ports.CapabilityDecision) []byte {
	if d.Sales == nil {
		return []byte(`{}`)
	}
	data, _ := json.Marshal(d.Sales)
	return data
}

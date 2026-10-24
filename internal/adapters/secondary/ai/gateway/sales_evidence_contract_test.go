package gateway

import (
	"encoding/json"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestSalesToolEvidenceIDsAreConstrainedToRealCustomerMessages(t *testing.T) {
	context := &ports.CustomerSalesContext{RecentMessages: []ports.CustomerSalesRecentMessageEvidence{
		{Reference: "customer-before", Direction: "inbound", Origin: "customer", Text: "أريد اشتراك"},
		{Reference: "outbound-assistant", Direction: "outbound", Origin: "ai", Text: "تم"},
		{Reference: "old-employee", Direction: "outbound", Origin: "employee", Text: "مرحبا"},
	}}
	tools := customerSalesToolsForEvidence([]ports.CustomerSalesCapability{
		ports.CustomerSalesCapabilityRecordLead,
		ports.CustomerSalesCapabilityCreateDraft,
		ports.CustomerSalesCapabilityUpdateDraft,
	}, "current-provider-message", context)
	makeCall := func(name, evidence string) error {
		var arguments string
		if name == "record_lead" {
			arguments = `{"sales":{"evidence_message_ids":["` + evidence + `"]}}`
		} else {
			arguments = `{"sales":{"workflow_id":"workflow-1","expected_version":2,"transaction_type":"subscription","lines":[{"item_id":"catalog-1","offer_id":null,"variant_id":null,"quantity":"1","selected_attributes_json":"{}"}],"transaction_data_json":"{}","evidence_message_ids":["` + evidence + `"]}}`
		}
		_, err := parseCustomerSalesCapability(capabilityResponse{Calls: []capabilityCall{{
			ID: "call-1", Name: name, Arguments: json.RawMessage(arguments),
		}}}, tools)
		return err
	}
	for _, tool := range tools {
		if err := validateStrictSchema(tool.Parameters); err != nil {
			t.Fatalf("invalid strict tool schema for %s: %v", tool.Name, err)
		}
		for _, allowed := range []string{"current-provider-message", "customer-before"} {
			if err := makeCall(tool.Name, allowed); err != nil {
				t.Fatalf("%s rejected real customer reference %q: %v", tool.Name, allowed, err)
			}
		}
		for _, forbidden := range []string{"outbound-assistant", "old-employee", "fabricated-ref", "workflow-1"} {
			if err := makeCall(tool.Name, forbidden); err == nil {
				t.Fatalf("%s accepted unexposed or outbound evidence %q", tool.Name, forbidden)
			}
		}
	}
}

package gateway

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestCustomerCapabilityBoundary(t *testing.T) {
	tools := customerSalesTools(ports.CustomerSalesCapabilities(true))
	tests := []struct {
		name, response string
		capability     ports.CustomerSalesCapability
		invalid        bool
	}{
		{"greeting", `{"steps":[{"type":"function_call","id":"c","name":"respond_without_action","arguments":{"response_text":"وعليكم السلام"}}]}`, ports.CustomerSalesCapabilityRespondWithoutAction, false},
		{"clarify", `{"steps":[{"type":"function_call","id":"c","name":"ask_customer","arguments":{"response_text":"ما العدد؟"}}]}`, ports.CustomerSalesCapabilityAsk, false},
		{"handoff", `{"steps":[{"type":"function_call","id":"c","name":"handoff_to_human","arguments":{"response_text":"سأحوّل طلبك إلى الفريق"}}]}`, ports.CustomerSalesCapabilityHandoff, false},
		{"catalog", `{"steps":[{"type":"function_call","id":"c","name":"evaluate_full_catalog","arguments":{}}]}`, ports.CustomerSalesCapabilityFullCatalog, false},
		{"obsolete presentation", `{"steps":[{"type":"function_call","id":"c","name":"evaluate_full_catalog","arguments":{"presentation":"detail"}}]}`, "", true},
		{"no call", `{"steps":[{"type":"model_output","content":[{"type":"text","text":"hello"}]}]}`, "", true},
		{"unknown", `{"steps":[{"type":"function_call","id":"c","name":"create_order_draft","arguments":{}}]}`, "", true},
		{"tenant argument", `{"steps":[{"type":"function_call","id":"c","name":"evaluate_full_catalog","arguments":{"business_id":"other"}}]}`, "", true},
		{"filter", `{"steps":[{"type":"function_call","id":"c","name":"evaluate_full_catalog","arguments":{"query":"phone"}}]}`, "", true},
		{"empty response", `{"steps":[{"type":"function_call","id":"c","name":"handoff_to_human","arguments":{"response_text":" "}}]}`, "", true},
		{"authority", `{"steps":[{"type":"function_call","id":"c","name":"respond_without_action","arguments":{"response_text":"hi","authorized":true}}]}`, "", true},
		{"invented status", `{"steps":[{"type":"function_call","id":"c","name":"respond_without_action","arguments":{"response_text":"hi","status":"resolved"}}]}`, "", true},
		{"nested authority", `{"steps":[{"type":"function_call","id":"c","name":"respond_without_action","arguments":{"response_text":"hi","selected":[{"item_id":"a","business_id":"b"}]}}]}`, "", true},
		{"missing ID", `{"steps":[{"type":"function_call","name":"respond_without_action","arguments":{"response_text":"hi"}}]}`, "", true},
		{"parallel", `{"steps":[{"type":"function_call","id":"c","name":"evaluate_full_catalog","arguments":{}},{"type":"function_call","id":"d","name":"respond_without_action","arguments":{"response_text":"hi"}}]}`, "", true},
		{"null", `{"steps":[{"type":"function_call","id":"c","name":"evaluate_full_catalog","arguments":null}]}`, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var response interactionResponse
			if err := json.Unmarshal([]byte(tc.response), &response); err != nil {
				t.Fatal(err)
			}
			decision, err := parseCustomerSalesCapability(response, tools)
			if tc.invalid {
				if err == nil {
					t.Fatal("invalid capability accepted")
				}
				return
			}
			if err != nil || decision.Capability != tc.capability {
				t.Fatalf("%s %+v %v", decision.Capability, decision, err)
			}
		})
	}
	var response interactionResponse
	_ = json.Unmarshal([]byte(tests[3].response), &response)
	if _, err := parseCustomerSalesCapability(response, customerSalesTools(ports.CustomerSalesCapabilities(false))); err == nil {
		t.Fatal("catalog call accepted when unavailable or after complete coverage")
	}
}

func TestCatalogCapabilityHasNoPresentationArguments(t *testing.T) {
	tools := customerSalesTools([]ports.CustomerSalesCapability{ports.CustomerSalesCapabilityFullCatalog})
	if len(tools) != 1 {
		t.Fatal("catalog capability is unavailable")
	}
	props := tools[0].Parameters["properties"].(map[string]any)
	if len(props) != 0 {
		t.Fatalf("evaluate_full_catalog must not expose presentation or retrieval filters: %+v", props)
	}

	var response interactionResponse
	if err := json.Unmarshal([]byte(`{"steps":[{"type":"function_call","id":"c","name":"evaluate_full_catalog","arguments":{}}]}`), &response); err != nil {
		t.Fatal(err)
	}
	decision, err := parseCustomerSalesCapability(response, tools)
	if err != nil || decision.Capability != ports.CustomerSalesCapabilityFullCatalog {
		t.Fatalf("catalog retrieval capability was not accepted: %+v %v", decision, err)
	}

	if err := json.Unmarshal([]byte(`{"steps":[{"type":"function_call","id":"c","name":"evaluate_full_catalog","arguments":{"presentation":"hierarchy"}}]}`), &response); err != nil {
		t.Fatal(err)
	}
	if _, err := parseCustomerSalesCapability(response, tools); err == nil {
		t.Fatal("obsolete presentation argument was accepted")
	}
}

func TestCustomerCapabilityDescriptionsEnforceOperationBoundary(t *testing.T) {
	tools := customerSalesTools(ports.CustomerSalesCapabilities(true))
	byName := make(map[string]customerSalesTool, len(tools))
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	if _, ok := byName["reply_customer"]; ok {
		t.Fatal("obsolete broad reply_customer capability must not be offered")
	}
	respond, ok := byName[string(ports.CustomerSalesCapabilityRespondWithoutAction)]
	if !ok {
		t.Fatal("respond_without_action capability missing")
	}
	if !strings.Contains(respond.Description, "without starting, confirming, completing, changing, or promising an external business operation") ||
		!strings.Contains(respond.Description, "use handoff_to_human") {
		t.Fatalf("respond_without_action description lost operation boundary: %q", respond.Description)
	}
	respondText := respond.Parameters["properties"].(map[string]any)["response_text"].(map[string]any)["description"].(string)
	if !strings.Contains(respondText, "must not ask the customer to confirm an unsupported external operation") ||
		!strings.Contains(respondText, "must not claim") {
		t.Fatalf("respond_without_action response schema lost truth boundary: %q", respondText)
	}
	ask := byName[string(ports.CustomerSalesCapabilityAsk)]
	if !strings.Contains(ask.Description, "Never reconfirm a clear request") {
		t.Fatalf("ask_customer can regress into redundant confirmation: %q", ask.Description)
	}
	handoff := byName[string(ports.CustomerSalesCapabilityHandoff)]
	if !strings.Contains(handoff.Description, "start, confirm, complete, or change an external operation") ||
		!strings.Contains(handoff.Description, "Short confirmations inherit the prior conversational goal") {
		t.Fatalf("handoff_to_human description lost unsupported-operation boundary: %q", handoff.Description)
	}
}

func TestStrictCustomerProposalRejectsUnknownFields(t *testing.T) {
	var proposal ports.CapabilityDecision
	err := decodeStrictStructuredJSON([]byte(`{"status":"resolved","action":"answer","response_text":"ok","authorized":true}`), &proposal)
	if err == nil {
		t.Fatal("unknown top-level proposal field must be rejected")
	}
}

func TestStrictCustomerProposalRejectsUnknownSelectedFields(t *testing.T) {
	var proposal ports.CapabilityDecision
	err := decodeStrictStructuredJSON([]byte(`{"status":"resolved","action":"answer","response_text":"ok","selected":[{"item_id":"item-1","payment_confirmed":true}]}`), &proposal)
	if err == nil {
		t.Fatal("unknown selected[] field must be rejected")
	}
}

func TestCustomerCapabilitySchemaDisallowsAdditionalProperties(t *testing.T) {
	schema := customerSalesResponseParameters(ports.CustomerSalesCapabilityRespondWithoutAction)
	if schema["additionalProperties"] != false {
		t.Fatalf("proposal schema must close top-level properties: %#v", schema["additionalProperties"])
	}
	selected := schema["properties"].(map[string]any)["selected"].(map[string]any)
	item := selected["items"].(map[string]any)
	if item["additionalProperties"] != false {
		t.Fatalf("selected item schema must close additional properties: %#v", item["additionalProperties"])
	}
}

func TestBatchCandidateParserRejectsUnknownFields(t *testing.T) {
	resp := catalogCandidateResponse{
		Candidates: []batchCandidate{{
			Content: batchContent{Parts: []batchPart{{
				Text: `{"candidates":[{"item_id":"item-1","authorized":true}]}`,
			}}},
		}},
	}
	if _, err := parseBatchCandidates(resp); err == nil {
		t.Fatal("unknown batch candidate field must be rejected")
	}
}

func TestFinalCapabilityRejectsUnknownFields(t *testing.T) {
	var response interactionResponse
	if err := json.Unmarshal([]byte(`{"steps":[{"type":"function_call","id":"c","name":"respond_without_action","arguments":{"response_text":"ok","requires_approval":true}}]}`), &response); err != nil {
		t.Fatal(err)
	}
	if _, err := parseCustomerSalesCapability(response, customerSalesTools(ports.CustomerSalesCapabilities(false))); err == nil {
		t.Fatal("unknown final authority accepted")
	}
}

func TestBatchCandidateSchemaDisallowsAdditionalProperties(t *testing.T) {
	schema := batchCandidateResponseSchema()
	if schema["additionalProperties"] != false {
		t.Fatalf("batch schema must close top-level properties: %#v", schema["additionalProperties"])
	}
	candidates := schema["properties"].(map[string]any)["candidates"].(map[string]any)
	item := candidates["items"].(map[string]any)
	if item["additionalProperties"] != false {
		t.Fatalf("candidate schema must close additional properties: %#v", item["additionalProperties"])
	}
}

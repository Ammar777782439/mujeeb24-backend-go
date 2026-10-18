package gateway

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/prompts"
)

func TestStrictCapabilitySchemasAndSingleCall(t *testing.T) {
	tools := customerSalesTools(ports.CustomerSalesCapabilities(true, true))
	for _, tool := range tools {
		if err := validateStrictSchema(tool.Parameters); err != nil {
			t.Fatalf("tool %s is not a strict JSON schema: %v", tool.Name, err)
		}
	}
	allowed := customerSalesTools(ports.CustomerSalesCapabilities(true))
	cases := []struct {
		name, tool, args string
		valid            bool
	}{
		{"greeting", "respond_without_action", `{"response_text":"مرحبا","selected":[]}`, true},
		{"ask", "ask_customer", `{"response_text":"ما العدد؟","selected":[]}`, true},
		{"handoff", "handoff_to_human", `{"response_text":"سيُراجع الموظف الطلب","selected":[]}`, true},
		{"catalog", "evaluate_full_catalog", `{}`, true},
		{"obsolete_filter", "evaluate_full_catalog", `{"query":"phone"}`, false},
		{"unknown", "create_order_draft", `{}`, false},
		{"missing_selected", "respond_without_action", `{"response_text":"مرحبا"}`, false},
		{"null_selected", "respond_without_action", `{"response_text":"مرحبا","selected":null}`, false},
		{"unknown_nested", "respond_without_action", `{"response_text":"مرحبا","selected":[{"item_id":"x","variant_id":null,"offer_id":null,"admin":true}]}`, false},
		{"missing_ref", "respond_without_action", `{"response_text":"مرحبا","selected":[{"item_id":"x","variant_id":null}]}`, false},
		{"unknown_authority", "respond_without_action", `{"response_text":"مرحبا","selected":[],"admin":true}`, false},
		{"null_args", "evaluate_full_catalog", `null`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCustomerSalesCapability(capabilityResponse{Calls: []capabilityCall{{ID: "call-1", Name: tc.tool, Arguments: json.RawMessage(tc.args)}}}, allowed)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t capability=%+v error=%v", tc.valid, got, err)
			}
			if tc.valid && got.Capability != ports.CustomerSalesCapability(tc.tool) {
				t.Fatal("wrong capability")
			}
		})
	}
	if _, err := parseCustomerSalesCapability(capabilityResponse{Calls: []capabilityCall{
		{ID: "1", Name: "evaluate_full_catalog", Arguments: json.RawMessage(`{}`)},
		{ID: "2", Name: "ask_customer", Arguments: json.RawMessage(`{"response_text":"?","selected":[]}`)},
	}}, allowed); err == nil {
		t.Fatal("parallel capabilities accepted")
	}
}

func TestStrictSalesToolPreservesDynamicProposal(t *testing.T) {
	tools := customerSalesTools([]ports.CustomerSalesCapability{ports.CustomerSalesCapabilityCreateDraft})
	args := `{"sales":{"workflow_id":null,"expected_version":null,"transaction_type":"order","lines":[{"item_id":"item-1","offer_id":null,"variant_id":null,"quantity":"2","selected_attributes_json":"{\"storage\":\"256GB\"}"}],"transaction_data_json":"{\"delivery_address\":\"صنعاء\"}","evidence_message_ids":["message-1"]}}`
	// The model must return JSON strings containing JSON objects, not open-ended schema fields.
	var obj map[string]any
	if err := json.Unmarshal([]byte(args), &obj); err != nil {
		t.Fatalf("invalid fixture: %v", err)
	}
	got, err := parseCustomerSalesCapability(capabilityResponse{Calls: []capabilityCall{{ID: "call-1", Name: string(ports.CustomerSalesCapabilityCreateDraft), Arguments: json.RawMessage(args)}}}, tools)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sales == nil || got.Sales.TransactionType != "order" || got.Sales.TransactionData["delivery_address"] != "صنعاء" ||
		len(got.Sales.Lines) != 1 || got.Sales.Lines[0].SelectedAttributes["storage"] != "256GB" {
		t.Fatalf("sales proposal lost: %+v", got)
	}
	if _, err := parseCustomerSalesCapability(capabilityResponse{Calls: []capabilityCall{{ID: "1", Name: string(ports.CustomerSalesCapabilityCreateDraft), Arguments: json.RawMessage(strings.Replace(args, `"transaction_data_json":"{\"delivery_address\":\"صنعاء\"}"`, `"transaction_data_json":"not-json"`, 1))}}}, tools); err == nil {
		t.Fatal("invalid dynamic JSON accepted")
	}
}

func TestStrictBatchCandidateDecoder(t *testing.T) {
	schema := batchCandidateResponseSchema()
	if err := validateStrictSchema(schema); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"candidates":[]}`, true},
		{`{"candidates":[{"item_id":"item-1","variant_ids":[],"offer_ids":[],"reason":"matches"}]}`, true},
		{`{"candidates":[{"item_id":"item-1","variant_ids":[],"offer_ids":[]}]}`, false},
		{`{"candidates":[{"item_id":"item-1","variant_ids":[],"offer_ids":[],"reason":"ok","approved":true}]}`, false},
	} {
		err := validateStrictArguments([]byte(tc.body), schema)
		if (err == nil) != tc.valid {
			t.Fatalf("body=%s unexpected validation %v", tc.body, err)
		}
	}
}

func TestToolDescriptionsAreSemanticAuthorityAndSummaryIsNotExecution(t *testing.T) {
	for _, name := range []string{"respond_without_action", "ask_customer", "handoff_to_human", "evaluate_full_catalog", "record_lead", "create_transaction_draft", "update_transaction_draft"} {
		if strings.Contains(prompts.CustomerSalesSystemPrompt, name) || strings.Contains(prompts.FinalEvaluationSystemPrompt, name) {
			t.Fatalf("tool-specific rule duplicated in global prompt: %s", name)
		}
	}
	ctx := &ports.CustomerSalesContext{
		Conversation:        ports.CustomerSalesContextConversation{State: "waiting_customer", Ownership: "ai"},
		ConversationState:   &ports.ConversationStateRecord{Summary: "تم تحويل المحادثة"},
		ConversationSummary: "تم تحويل المحادثة",
	}
	first := customerSalesPromptContextFrom(ctx)
	batch := catalogBatchPromptContextFrom(ctx)
	if first.VerifiedExecutionState.Source != "conversation_repository" || first.VerifiedExecutionState.ConversationState != "waiting_customer" ||
		batch.VerifiedExecutionState.ConversationState != "waiting_customer" {
		t.Fatal("summary overrode repository state")
	}
	if first.ConversationState.Summary != "" || batch.ConversationState.Summary != "" ||
		first.ConversationSummary != ctx.ConversationSummary || ctx.ConversationState.Summary == "" {
		t.Fatal("summary is duplicated or lost across model trust boundaries")
	}
	seen := map[string]bool{}
	for _, tool := range customerSalesTools(ports.CustomerSalesCapabilities(true, true)) {
		if strings.TrimSpace(tool.Description) == "" {
			t.Fatalf("missing description %s", tool.Name)
		}
		seen[tool.Name] = true
	}
	if len(seen) != 7 {
		t.Fatalf("expected 7 tool definitions, got %d", len(seen))
	}
}

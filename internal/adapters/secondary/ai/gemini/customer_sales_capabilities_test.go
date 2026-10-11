package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
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

func TestFullCatalogCapabilityIsAnInternalRequest(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request interactionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.Tools) != 4 || request.GenerationConfig.ToolChoice != "any" {
			t.Fatalf("wrong capabilities: %+v", request)
		}
		_, _ = w.Write([]byte(`{"status":"requires_action","steps":[{"type":"function_call","id":"catalog-call","name":"evaluate_full_catalog","arguments":{}}],"usage":{"total_input_tokens":31,"total_output_tokens":8}}`))
	}))
	defer server.Close()
	base, _ := NewGeminiHTTPClient(GeminiHTTPClientConfig{BaseURL: server.URL, APIKey: "test", Model: "test"})
	adapter, _ := NewGeminiCustomerSalesAdapter(base)
	out, err := adapter.Decide(context.Background(), ports.CustomerSalesDecisionInput{Request: ports.CustomerSalesDecisionRequest{Text: "شيء مناسب للعائلة"}, Capabilities: ports.CustomerSalesCapabilities(true)})
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision.Capability != ports.CustomerSalesCapabilityFullCatalog || out.Decision.ResponseText != "" || calls != 1 || out.Usage.ModelRequests != 1 {
		t.Fatalf("catalog request must not become final reply: %+v", out)
	}
}

func TestFinalCatalogDecisionUsesOnlyTerminalCapabilitiesAndCountsTools(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(r.URL.Path, ":countTokens") {
			envelope := request["generateContentRequest"].(map[string]any)
			if len(envelope["tools"].([]any)) != 1 {
				t.Fatal("token count omitted function declarations")
			}
			if _, ok := envelope["generationConfig"].(map[string]any)["responseJsonSchema"]; ok {
				t.Fatal("final tokenizer uses obsolete output schema")
			}
			_, _ = w.Write([]byte(`{"totalTokens":125}`))
			return
		}
		tools := request["tools"].([]any)
		if len(tools) != 3 {
			t.Fatalf("unexpected final tools: %+v", tools)
		}
		for _, tool := range tools {
			if tool.(map[string]any)["name"] == "evaluate_full_catalog" {
				t.Fatal("final evaluation must not restart retrieval")
			}
		}
		if _, ok := request["response_format"]; ok {
			t.Fatal("final still uses proposal schema")
		}
		system := request["system_instruction"].(string)
		if !strings.Contains(system, "FINAL EVALUATION") || strings.Contains(system, "لا تصدر الرد النهائي للعميل") {
			t.Fatal("final prompt has conflicting batch instructions")
		}
		_, _ = w.Write([]byte(`{"status":"requires_action","steps":[{"type":"function_call","id":"final","name":"handoff_to_human","arguments":{"response_text":"سأحوّل طلبك للفريق"}}],"usage":{"total_input_tokens":125,"total_output_tokens":20}}`))
	}))
	defer server.Close()
	client, _ := NewBatchClient(BatchClientConfig{BaseURL: server.URL, APIKey: "test", Model: "test"})
	input := services.FinalEvaluationInput{EntityContract: services.BuildCatalogEntityContractPayload()}
	if tokens, err := client.CountFinalTokens(context.Background(), input, "verified candidates"); err != nil || tokens != 125 {
		t.Fatalf("count %d %v", tokens, err)
	}
	proposal, usage, err := client.FinalEvaluateWithDetails(context.Background(), input, "verified candidates")
	if err != nil || proposal.Capability != ports.CustomerSalesCapabilityHandoff || proposal.ResponseText != "سأحوّل طلبك للفريق" || usage.InputTokens != 125 || calls != 2 {
		t.Fatalf("final %+v usage %+v err %v", proposal, usage, err)
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

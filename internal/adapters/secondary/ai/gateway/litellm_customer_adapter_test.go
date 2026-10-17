package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/litellm"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func chatToolResponse(name, arguments string) string {
	encoded, _ := json.Marshal(arguments)
	return `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"`+name+`","arguments":`+string(encoded)+`}}]}}],"usage":{"prompt_tokens":23,"completion_tokens":8,"prompt_tokens_details":{"cached_tokens":5}}}`
}

func TestCustomerChatCompletionTrustSeparationAndToolContract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("wrong LiteLLM endpoint %s %s", r.Method, r.URL.Path)
		}
		var request chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil { t.Error(err) }
		if request.Model != "mujeeb-sales" || request.ToolChoice != "required" ||
			request.ParallelToolCalls == nil || *request.ParallelToolCalls ||
			len(request.Messages) != 2 || request.Messages[0].Role != "system" ||
			request.Messages[1].Role != "user" || !strings.Contains(request.Messages[0].Content, "Catalog Entity Contract") ||
			!strings.Contains(request.Messages[1].Content, "السلام عليكم") {
			t.Errorf("Chat Completions contract violated: %+v", request)
		}
		if len(request.Tools) != 1 || request.Tools[0].Function.Name != "respond_without_action" ||
			!request.Tools[0].Function.Strict || validateStrictSchema(request.Tools[0].Function.Parameters) != nil {
			t.Error("strict capability tool invalid")
		}
		_, _ = w.Write([]byte(chatToolResponse("respond_without_action", `{"response_text":"وعليكم السلام","selected":[]}`)))
	}))
	defer srv.Close()
	client, err := litellm.NewClient(litellm.Config{BaseURL: srv.URL, APIKey: "test-key"})
	if err != nil { t.Fatal(err) }
	adapter, err := NewLiteLLMCustomerSalesAdapter(client, "mujeeb-sales", 2048, 48000)
	if err != nil { t.Fatal(err) }
	out, err := adapter.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Capabilities: []ports.CustomerSalesCapability{ports.CustomerSalesCapabilityRespondWithoutAction},
		Request: ports.CustomerSalesDecisionRequest{Text: "السلام عليكم"},
		EntityContractPayload: []byte(`{"catalog":"contract"}`),
	})
	if err != nil { t.Fatal(err) }
	if out.Decision.Capability != ports.CustomerSalesCapabilityRespondWithoutAction ||
		out.Decision.ResponseText != "وعليكم السلام" || out.Usage.InputTokens != 23 ||
		out.Usage.CachedTokens != 5 || out.Usage.OutputTokens != 8 {
		t.Fatalf("bad decision or billing metadata: %+v", out)
	}
}

func TestCustomerChatRejectsUntrustedOrIncompleteResponses(t *testing.T) {
	tests := []struct{name, body string}{
		{"unknown", chatToolResponse("other", `{}`)},
		{"unexpected_field", chatToolResponse("respond_without_action", `{"response_text":"hi","selected":[],"authorized":true}`)},
		{"missing_required", chatToolResponse("respond_without_action", `{"response_text":"hi"}`)},
		{"missing_call_id", strings.Replace(chatToolResponse("respond_without_action", `{"response_text":"hi","selected":[]}`), `"id":"call-1"`, `"id":""`, 1)},
		{"text_only", `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"hello"}}]}`},
		{"truncated", `{"choices":[{"finish_reason":"length","message":{"role":"assistant","content":null}}]}`},
		{"parallel", `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"respond_without_action","arguments":"{}"}},{"id":"b","type":"function","function":{"name":"respond_without_action","arguments":"{}"}}]}}]}`},
		{"mixed", `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"unverified","tool_calls":[{"id":"a","type":"function","function":{"name":"respond_without_action","arguments":"{}"}}]}}]}`},
		{"invalid_json_arguments", chatToolResponse("respond_without_action", `not-json`)},
		{"refusal", `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","refusal":"no","content":"no"}}]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.body)) }))
			defer srv.Close()
			client, err := litellm.NewClient(litellm.Config{BaseURL: srv.URL, APIKey: "key"})
			if err != nil { t.Fatal(err) }
			adapter, err := NewLiteLLMCustomerSalesAdapter(client, "mujeeb-sales", 2048, 48000)
			if err != nil { t.Fatal(err) }
			_, err = adapter.Decide(context.Background(), ports.CustomerSalesDecisionInput{
				Capabilities: []ports.CustomerSalesCapability{ports.CustomerSalesCapabilityRespondWithoutAction},
				Request: ports.CustomerSalesDecisionRequest{Text: "hello"},
			})
			if err == nil { t.Fatal("unsafe model response accepted") }
		})
	}
}

func TestDecisionDiagnosticsNeverIncludeCustomerArguments(t *testing.T) {
	secret := "customer-private-text"
	report := summarizeDecisionResponseShape(normalizedLiteLLMResponse{
		FunctionCalls: []capabilityCall{{ID:"x",Name:"respond_without_action",Arguments:json.RawMessage(`{"response_text":"`+secret+`"}`)}},
	})
	if strings.Contains(report, secret) || strings.Contains(report, "response_text") { t.Fatalf("leaked customer content: %s",report) }
}

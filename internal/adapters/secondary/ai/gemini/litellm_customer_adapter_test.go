package gemini

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

func TestLiteLLMCustomerSalesUsesExistingCapabilityBoundary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model             string                      `json:"model"`
			Store             bool                        `json:"store"`
			Tools             []customerSalesTool         `json:"tools"`
			GenerationConfig  interactionGenerationConfig `json:"generation_config"`
			SystemInstruction string                      `json:"system_instruction"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Model != "mujeeb-customer" || request.Store || request.GenerationConfig.ToolChoice != "any" {
			t.Errorf("contract changed: %+v", request)
		}
		if len(request.Tools) != 1 || request.Tools[0].Name != "respond_without_action" ||
			!strings.Contains(request.SystemInstruction, "Catalog Entity Contract") {
			t.Errorf("capability or system contract changed")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"completed","outputs":[{"type":"function_call","id":"call-1","name":"respond_without_action","arguments":"{"response_text":"وعليكم السلام"}"}],"usage":{"total_input_tokens":23,"total_output_tokens":8}}`))
	}))
	defer server.Close()

	client, err := litellm.NewClient(litellm.Config{BaseURL: server.URL, APIKey: "test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewLiteLLMCustomerSalesAdapter(client, "mujeeb-customer", 4096, 48000)
	if err != nil {
		t.Fatal(err)
	}
	out, err := adapter.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Capabilities:          []ports.CustomerSalesCapability{ports.CustomerSalesCapabilityRespondWithoutAction},
		Request:               ports.CustomerSalesDecisionRequest{Text: "السلام عليكم"},
		EntityContractPayload: []byte(`{"catalog":"contract"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision.Capability != ports.CustomerSalesCapabilityRespondWithoutAction ||
		out.Decision.ResponseText != "وعليكم السلام" || out.Usage.Provider != "litellm" ||
		out.Usage.Model != "mujeeb-customer" || out.Usage.InputTokens != 23 || out.Usage.OutputTokens != 8 {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestLiteLLMCustomerSalesRejectsUnknownAndParallelTools(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{"unknown", `{"status":"completed","outputs":[{"type":"function_call","id":"id-1","name":"unsupported_tool","arguments":"{}"}]}`},
		{"parallel", `{"status":"completed","outputs":[{"type":"function_call","id":"id-1","name":"respond_without_action","arguments":"{"response_text":"yes"}"},{"type":"function_call","id":"id-2","name":"respond_without_action","arguments":"{"response_text":"yes"}"}]}`},
		{"text_only", `{"status":"completed","outputs":[{"type":"text","text":"unverified"}]}`},
		{"incomplete", `{"status":"incomplete","outputs":[{"type":"function_call","id":"id-1","name":"respond_without_action","arguments":"{"response_text":"hi"}"}]}`},
		{"extra_arguments", `{"status":"completed","outputs":[{"type":"function_call","id":"id-1","name":"respond_without_action","arguments":"{"response_text":"hi","business_id":"other"}"}]}`},
		{"missing_call_id", `{"status":"completed","outputs":[{"type":"function_call","name":"respond_without_action","arguments":"{"response_text":"hi"}"}]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			client, err := litellm.NewClient(litellm.Config{BaseURL: server.URL, APIKey: "test-key"})
			if err != nil {
				t.Fatal(err)
			}
			adapter, err := NewLiteLLMCustomerSalesAdapter(client, "mujeeb-customer", 4096, 48000)
			if err != nil {
				t.Fatal(err)
			}
			_, err = adapter.Decide(context.Background(), ports.CustomerSalesDecisionInput{
				Capabilities: []ports.CustomerSalesCapability{ports.CustomerSalesCapabilityRespondWithoutAction},
				Request:      ports.CustomerSalesDecisionRequest{Text: "Test"},
			})
			if err == nil {
				t.Fatalf("unsafe model output was accepted: %s", test.body)
			}
		})
	}
}

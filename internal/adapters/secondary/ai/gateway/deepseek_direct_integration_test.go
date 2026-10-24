package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// End-to-end, offline verification of customer capability selection over
// DeepSeek's actual HTTP wire adapter. No Gemini key or provider is constructed.
func TestDeepSeekOnlyCustomerModelChoosesCatalogCapability(t *testing.T) {
	const model = "deepseek-flash"
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/beta/chat/completions" {
			t.Errorf("unexpected DeepSeek method/path: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-deepseek-key" {
			t.Error("missing DeepSeek transport credential")
		}
		var body struct {
			Model string `json:"model"`
			ToolChoice string `json:"tool_choice"`
			ParallelToolCalls *bool `json:"parallel_tool_calls"`
			Thinking struct {
				Type string `json:"type"`
			} `json:"thinking"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
					Strict bool `json:"strict"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Model != model || body.ToolChoice != "required" || body.Thinking.Type != "disabled" ||
			body.ParallelToolCalls == nil || *body.ParallelToolCalls {
			t.Errorf("unsafe request envelope: %+v", body)
		}
		found := false
		for _, tool := range body.Tools {
			if !tool.Function.Strict {
				t.Errorf("tool %s did not preserve strict schema", tool.Function.Name)
			}
			if tool.Function.Name == "evaluate_full_catalog" {
				found = true
			}
		}
		if !found {
			t.Error("catalog capability missing from offered tools")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"model":"deepseek-flash","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"call-catalog","type":"function","function":{"name":"evaluate_full_catalog","arguments":"{}"}}]}}],"usage":{"prompt_tokens":123,"completion_tokens":16,"prompt_cache_hit_tokens":3}}`)
	}))
	defer mock.Close()
	provider, err := native.NewDeepSeek(native.DeepSeekConfig{
		Model: model, APIKey: "test-deepseek-key", BaseURL: mock.URL + "/beta", Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	router, err := native.NewRouter(provider, nil, native.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := native.NewCompletionClient(router)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewNativeCustomerSalesAdapter(client, "mujeeb-sales", 1024, 48000)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Request: ports.CustomerSalesDecisionRequest{Text: "كم سعر سامسونج 256 جيجا؟"},
		Capabilities: ports.CustomerSalesCapabilities(true, false),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision.Capability != ports.CustomerSalesCapabilityFullCatalog ||
		result.Usage.Provider != "deepseek" || result.Usage.Model != model ||
		result.Usage.ModelRequests != 1 || result.Usage.InputTokens != 123 ||
		result.Usage.OutputTokens != 16 || result.Usage.CachedTokens != 3 {
		t.Fatalf("wrong DeepSeek capability or telemetry: %+v", result)
	}
}

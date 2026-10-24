package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type customerNativeProvider struct {
	name        string
	unavailable bool
	calls       int
}

func (p *customerNativeProvider) Name() string { return p.name }
func (p *customerNativeProvider) Generate(_ context.Context, _ native.Request) (native.Response, error) {
	p.calls++
	if p.unavailable {
		return native.Response{}, &native.StatusError{Provider: p.name, Code: http.StatusServiceUnavailable}
	}
	args, err := json.Marshal(map[string]any{"response_text": "مرحبًا", "selected": []any{}})
	if err != nil {
		return native.Response{}, err
	}
	response, err := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"finish_reason": "tool_calls", "message": map[string]any{
				"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{
					"id": "call-1", "type": "function", "function": map[string]any{
						"name": "respond_without_action", "arguments": string(args),
					},
				}},
			},
		}},
		"usage": map[string]any{"prompt_tokens": 11, "completion_tokens": 5,
			"prompt_tokens_details": map[string]any{"cached_tokens": 2}},
	})
	if err != nil {
		return native.Response{}, err
	}
	return native.Response{Payload: response, Model: p.name + "-model", InputTokens: 11, CachedTokens: 2, OutputTokens: 5}, nil
}
func TestNativeCustomerDecisionUsesExactCanonicalCapabilities(t *testing.T) {
	a, b := &customerNativeProvider{name: "gemini"}, &customerNativeProvider{name: "deepseek"}
	router, err := native.NewRouter(a, b, native.Policy{ConsecutiveFailures: 1, Cooldown: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	client, err := native.NewCompletionClient(router)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewNativeCustomerSalesAdapter(client, "mujeeb-sales", 1024, 64000)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Capabilities: []ports.CustomerSalesCapability{ports.CustomerSalesCapabilityRespondWithoutAction},
		Request:      ports.CustomerSalesDecisionRequest{Text: "السلام عليكم"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision.Capability != ports.CustomerSalesCapabilityRespondWithoutAction ||
		result.Decision.ResponseText != "مرحبًا" || result.Usage.Provider != "gemini" ||
		result.Usage.InputTokens != 11 || result.Usage.CachedTokens != 2 || result.Usage.ModelRequests != 1 || b.calls != 0 {
		t.Fatalf("wrong canonical customer result: %+v", result)
	}
}
func TestNativeCustomerUsesDeepSeekOnlyWhenGemini503(t *testing.T) {
	a, b := &customerNativeProvider{name: "gemini", unavailable: true}, &customerNativeProvider{name: "deepseek"}
	router, err := native.NewRouter(a, b, native.Policy{ConsecutiveFailures: 1, Cooldown: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	client, err := native.NewCompletionClient(router)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewNativeCustomerSalesAdapter(client, "mujeeb-sales", 1024, 64000)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Capabilities: []ports.CustomerSalesCapability{ports.CustomerSalesCapabilityRespondWithoutAction},
		Request:      ports.CustomerSalesDecisionRequest{Text: "مرحبا"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Usage.Provider != "deepseek" || result.Usage.ModelRequests != 2 ||
		a.calls != 1 || b.calls != 1 || result.Decision.Capability != ports.CustomerSalesCapabilityRespondWithoutAction {
		t.Fatalf("customer fallback violated canonical contract: %+v", result)
	}
}

package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestCustomerSalesUsesSingleCapabilityAndCanonicalHistory(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/interactions" {
			t.Fatalf("expected /v1/interactions, got %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Fatalf("missing x-goog-api-key header")
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"int-2","status":"requires_action","steps":[{"type":"function_call","id":"call-1","name":"respond_without_action","arguments":{"response_text":"تم","selected":[]}}],"usage":{"total_input_tokens":21,"total_cached_tokens":4,"total_output_tokens":7,"total_tokens":28}}`))
	}))
	defer server.Close()

	client, err := NewGeminiHTTPClient(GeminiHTTPClientConfig{
		BaseURL:        server.URL,
		APIKey:         "test-key",
		Model:          "gemini-test",
		RequestTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("build client: %v", err)
	}
	adapter, err := NewGeminiCustomerSalesAdapter(client)
	if err != nil {
		t.Fatalf("build adapter: %v", err)
	}

	out, err := adapter.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Request: ports.CustomerSalesDecisionRequest{
			BusinessID:     "business-1",
			ConversationID: "conversation-1",
			Text:           "مرحبا",
		},
		GeminiInteraction: ports.GeminiInteractionContext{
			PreviousInteractionID: "int-1",
			Store:                 true,
		},
		EntityContractPayload: []byte(`{"entity_contract":{"catalog":{"id":"UUID"}}}`),
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	if captured["model"] != "gemini-test" {
		t.Fatalf("model mismatch: %#v", captured["model"])
	}
	if _, ok := captured["previous_interaction_id"]; ok {
		t.Fatal("terminal capability must use canonical history rather than unfinished provider calls")
	}
	if captured["store"] != false {
		t.Fatal("capability decision must be stateless")
	}
	if _, ok := captured["contents"]; ok {
		t.Fatal("Interactions request must not use generateContent contents")
	}
	if _, ok := captured["generationConfig"]; ok {
		t.Fatal("Interactions request must use generation_config snake_case")
	}
	if _, ok := captured["response_format"]; ok {
		t.Fatal("function calling must not use the old proposal schema")
	}
	tools, ok := captured["tools"].([]any)
	if !ok || len(tools) != 3 {
		t.Fatalf("executable tools missing: %#v", captured["tools"])
	}
	if captured["generation_config"].(map[string]any)["tool_choice"] != "any" {
		t.Fatal("one capability selection must be required")
	}
	if out.GeminiInteraction.ResultingInteractionID != "" || out.Decision.Capability != ports.CustomerSalesCapabilityRespondWithoutAction {
		t.Fatalf("unexpected continuity/capability: %+v", out)
	}
	if out.Usage.InputTokens != 21 || out.Usage.CachedTokens != 4 || out.Usage.OutputTokens != 7 {
		t.Fatalf("usage mismatch: %+v", out.Usage)
	}
	if out.Decision.ResponseText != "تم" {
		t.Fatalf("proposal response = %q", out.Decision.ResponseText)
	}
}

func TestCustomerSalesDropsPreviousInteractionWhenStoreDisabled(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"id":"int-new","status":"requires_action","steps":[{"type":"function_call","id":"call-2","name":"respond_without_action","arguments":{"response_text":"ok"}}]}`))
	}))
	defer server.Close()

	client, _ := NewGeminiHTTPClient(GeminiHTTPClientConfig{BaseURL: server.URL, APIKey: "k", Model: "m"})
	adapter, _ := NewGeminiCustomerSalesAdapter(client)
	_, err := adapter.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Request:           ports.CustomerSalesDecisionRequest{BusinessID: "b", ConversationID: "c", Text: "hello"},
		GeminiInteraction: ports.GeminiInteractionContext{PreviousInteractionID: "must-not-send", Store: false},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if _, ok := captured["previous_interaction_id"]; ok {
		t.Fatal("previous_interaction_id must be omitted when store=false")
	}
}

func TestCustomerSalesEnforcesMaxInputOnSerializedInteraction(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewGeminiHTTPClient(GeminiHTTPClientConfig{
		BaseURL:            server.URL,
		APIKey:             "test-key",
		Model:              "gemini-test",
		MaxInputCharacters: 80,
		RequestTimeout:     5 * time.Second,
	})
	if err != nil {
		t.Fatalf("build client: %v", err)
	}
	adapter, err := NewGeminiCustomerSalesAdapter(client)
	if err != nil {
		t.Fatalf("build adapter: %v", err)
	}

	_, err = adapter.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Request: ports.CustomerSalesDecisionRequest{
			BusinessID:     "business-1",
			ConversationID: "conversation-1",
			Text:           "hi",
		},
		EntityContractPayload: []byte(`{"entity_contract":{"catalog":{"description":"this makes the serialized system instruction intentionally larger than the configured limit"}}}`),
	})
	if err == nil {
		t.Fatal("expected serialized interaction input limit error")
	}
	if called {
		t.Fatal("HTTP request must not be sent when serialized interaction exceeds the configured input limit")
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

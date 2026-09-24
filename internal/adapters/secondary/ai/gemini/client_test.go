package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type mockCapabilityDispatcher struct {
	defs      []ports.AICapabilityDefinition
	executeFn func(ctx context.Context, execCtx ports.AICapabilityExecutionContext, name string, rawParams []byte) (ports.AICapabilityResult, error)
}

func (m mockCapabilityDispatcher) Definitions() []ports.AICapabilityDefinition {
	return m.defs
}

func (m mockCapabilityDispatcher) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, name string, rawParams []byte) (ports.AICapabilityResult, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, execCtx, name, rawParams)
	}
	return ports.AICapabilityResult{}, nil
}

func TestClientDecideSendsStructuredGeminiRequest(t *testing.T) {
	const apiKey = "test-gemini-key"
	var received struct {
		SystemInstruction struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"systemInstruction"`
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
		Tools []struct {
			FunctionDeclarations []struct {
				Name string `json:"name"`
			} `json:"functionDeclarations"`
		} `json:"tools"`
		GenerationConfig struct {
			MaxOutputTokens int `json:"maxOutputTokens"`
		} `json:"generationConfig"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.String(), "key="+apiKey) {
			t.Errorf("key not in URL: %s", r.URL.String())
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"intent_base\": \"information_request\", \"requested_action\": \"answer\", \"requires_human\": false, \"response_text\": \"تم استلام رسالتك\", \"confidence_value\": \"0.95\", \"confidence_band\": \"high\", \"policy_decision\": \"allowed\"}"}],"role":"model"},"finishReason":"STOP"}]}`))
	}))
	defer server.Close()

	dispatcher := mockCapabilityDispatcher{
		defs: []ports.AICapabilityDefinition{
			{
				Name:        "catalog_data",
				Description: "Retrieve factual merchant catalog data",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"operation": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	client, err := NewClient(Config{
		BaseURL:         server.URL,
		APIKey:          apiKey,
		Model:           "test-gemini-model",
		RequestTimeout:  time.Second,
		MaxOutputTokens: 321,
		Capabilities:    dispatcher,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	proposal, err := client.Decide(context.Background(), ports.AIDecisionInput{
		BusinessID:             "business-1",
		ConversationID:         "conversation-1",
		SourceMessageReference: "message-1",
		Text:                   "مرحبا",
		Channel:                "whatsapp",
		PolicyVersion:          "auto-reply-v1",
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if received.GenerationConfig.MaxOutputTokens != 321 {
		t.Fatalf("unexpected generationConfig: %#v", received.GenerationConfig)
	}
	if len(received.Contents) != 1 || received.Contents[0].Role != "user" || !strings.Contains(received.Contents[0].Parts[0].Text, "مرحبا") {
		t.Fatalf("unexpected contents: %#v", received.Contents)
	}
	if !strings.Contains(received.SystemInstruction.Parts[0].Text, "المساعد الذكي لخدمة عملاء") {
		t.Fatalf("system prompt missing customer rules: %#v", received.SystemInstruction)
	}
	if len(received.Tools) == 0 || len(received.Tools[0].FunctionDeclarations) == 0 || received.Tools[0].FunctionDeclarations[0].Name != "catalog_data" {
		t.Fatalf("catalog_data tool declaration missing: %#v", received.Tools)
	}
	if proposal.IntentBase != "information_request" || proposal.RequestedAction != "answer" || proposal.PolicyDecision != "allowed" || proposal.ResponseText != "تم استلام رسالتك" || proposal.ModelReference != "gemini/test-gemini-model" {
		t.Fatalf("unexpected proposal: %#v", proposal)
	}
	if proposal.CatalogRetrievalState != ports.CatalogRetrievalNoneRequired {
		t.Fatalf("expected CatalogRetrievalState=none_required, got %s", proposal.CatalogRetrievalState)
	}
	if len(proposal.CatalogStreams) != 0 || proposal.CatalogIncomplete || proposal.SafetyBudgetExhausted {
		t.Fatalf("expected empty streams and no incomplete/safety budget flags: %#v", proposal)
	}
}

func TestClientExecutesApplicationCapabilityToolCall(t *testing.T) {
	const apiKey = "test-gemini-key"
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// Model calls catalog_data
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"catalog_data","args":{"operation":"list_offers","item_id":"item-1"}}}],"role":"model"},"finishReason":"STOP"}]}`))
			return
		}
		// Model receives functionResponse and returns structured decision
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"intent_base\": \"information_request\", \"requested_action\": \"answer\", \"requires_human\": false, \"response_text\": \"آيفون 15 متوفر بسعر 250000 ريال\", \"confidence_value\": \"0.95\", \"confidence_band\": \"high\", \"policy_decision\": \"allowed\"}"}],"role":"model"},"finishReason":"STOP"}]}`))
	}))
	defer server.Close()

	executedCapability := false
	dispatcher := mockCapabilityDispatcher{
		defs: []ports.AICapabilityDefinition{
			{Name: "catalog_data", Description: "Catalog data access"},
		},
		executeFn: func(ctx context.Context, execCtx ports.AICapabilityExecutionContext, name string, rawParams []byte) (ports.AICapabilityResult, error) {
			if name != "catalog_data" {
				t.Fatalf("unexpected capability name: %s", name)
			}
			if execCtx.BusinessID != "business-1" {
				t.Fatalf("unexpected business ID in execution context: %s", execCtx.BusinessID)
			}
			executedCapability = true
			return ports.AICapabilityResult{
				Data: map[string]any{
					"offers": []map[string]any{
						{"id": "offer-1", "name": "iPhone 15 Offer", "amount": "250000", "currency": "YER"},
					},
				},
				OfferEvidence: []ports.AIOfferEvidence{
					{Reference: "offer-1", CatalogItemReference: "item-1", Name: "iPhone 15 Offer", Amount: "250000", Currency: "YER", AvailabilityState: "available", Status: "active"},
				},
			}, nil
		},
	}

	client, err := NewClient(Config{
		BaseURL:        server.URL,
		APIKey:         apiKey,
		Model:          "test-gemini-model",
		RequestTimeout: time.Second,
		Capabilities:   dispatcher,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctxValue := &ports.AIContext{
		CatalogEvidence: []ports.AICatalogEvidence{
			{Reference: "item-1", CatalogReference: "cat-1", Name: "iPhone 15", ItemType: "device", Status: "active"},
		},
	}

	proposal, err := client.Decide(context.Background(), ports.AIDecisionInput{
		BusinessID:             "business-1",
		ConversationID:         "conversation-1",
		SourceMessageReference: "message-1",
		Text:                   "كم سعر الآيفون 15؟",
		Channel:                "whatsapp",
		PolicyVersion:          "auto-reply-v1",
		Context:                ctxValue,
	})
	if err != nil {
		t.Fatalf("Decide with tool call failed: %v", err)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 turns (1 tool call + 1 final answer), got %d", callCount)
	}
	if !executedCapability {
		t.Fatal("expected application capability to be executed")
	}
	// Verify Gemini provider DID NOT mutate AIContext directly
	if len(ctxValue.OfferEvidence) != 0 {
		t.Fatalf("expected provider NOT to mutate AIContext directly, got %d offer evidence", len(ctxValue.OfferEvidence))
	}
	// Verify discovered evidence is returned in proposal
	if len(proposal.DiscoveredOfferEvidence) != 1 || proposal.DiscoveredOfferEvidence[0].Reference != "offer-1" {
		t.Fatalf("expected DiscoveredOfferEvidence in proposal, got %#v", proposal.DiscoveredOfferEvidence)
	}
	// Verify application layer merges evidence
	ports.IncorporateProposalEvidence(ctxValue, proposal)
	if len(ctxValue.OfferEvidence) != 1 || ctxValue.OfferEvidence[0].Reference != "offer-1" {
		t.Fatalf("expected OfferEvidence to be incorporated by application helper, got %#v", ctxValue.OfferEvidence)
	}
	if proposal.IntentBase != "information_request" || proposal.RequestedAction != "answer" || proposal.ResponseText != "آيفون 15 متوفر بسعر 250000 ريال" {
		t.Fatalf("unexpected proposal from tool call: %#v", proposal)
	}
}

func TestClientTechnicalSafetyBudgetExhaustion(t *testing.T) {
	const apiKey = "test-gemini-key"
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		// Model infinitely loops calling catalog_data
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"catalog_data","args":{"operation":"list_catalog_items","catalog_id":"cat-1"}}}],"role":"model"},"finishReason":"STOP"}]}`))
	}))
	defer server.Close()

	dispatcher := mockCapabilityDispatcher{
		defs: []ports.AICapabilityDefinition{
			{Name: "catalog_data", Description: "Catalog data access"},
		},
		executeFn: func(ctx context.Context, execCtx ports.AICapabilityExecutionContext, name string, rawParams []byte) (ports.AICapabilityResult, error) {
			return ports.AICapabilityResult{
				Data:            map[string]any{"items": []any{"item-1"}, "has_more": true},
				CatalogEvidence: []ports.AICatalogEvidence{{Reference: "item-1", Name: "Phone 15"}},
			}, nil
		},
	}

	// Set small safety turn budget
	client, err := NewClient(Config{
		BaseURL:          server.URL,
		APIKey:           apiKey,
		Model:            "test-gemini-model",
		RequestTimeout:   time.Second,
		Capabilities:     dispatcher,
		SafetyTurnBudget: 3,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	proposal, err := client.Decide(context.Background(), ports.AIDecisionInput{
		BusinessID:     "business-1",
		ConversationID: "conversation-1",
		Text:           "ما هي المنتجات؟",
		Context:        &ports.AIContext{},
	})
	if err != nil {
		t.Fatalf("expected safety budget exhaustion to return proposal, got err: %v", err)
	}
	if callCount != 3 {
		t.Fatalf("expected safety budget of 3 turns, got %d calls", callCount)
	}
	if !proposal.SafetyBudgetExhausted {
		t.Fatal("expected SafetyBudgetExhausted=true")
	}
	if !proposal.CatalogIncomplete {
		t.Fatal("expected CatalogIncomplete=true")
	}
	if proposal.CatalogRetrievalState != ports.CatalogRetrievalSafetyBudgetExhausted {
		t.Fatalf("expected CatalogRetrievalState=safety_budget_exhausted, got %s", proposal.CatalogRetrievalState)
	}
	if !proposal.RequiresHuman || proposal.PolicyDecision != "requires_approval" {
		t.Fatalf("expected human review required upon safety budget exhaustion: %#v", proposal)
	}
	if !strings.Contains(string(proposal.ReasonCodes), "safety_budget_exhausted") {
		t.Fatalf("expected safety_budget_exhausted in ReasonCodes: %s", string(proposal.ReasonCodes))
	}
}

func TestClientRejectsMissingAPIKey(t *testing.T) {
	if _, err := NewClient(Config{BaseURL: "https://example.com", APIKey: "", Model: "gemini-1.5-flash"}); err == nil {
		t.Fatal("expected missing API key error")
	}
}

func TestClientRejectsInvalidOrOversizedResponses(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "invalid json", body: `not-json`},
		{name: "empty candidates", body: `{"candidates":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test", Model: "test-model"})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			_, err = client.Decide(context.Background(), ports.AIDecisionInput{Text: "hi"})
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

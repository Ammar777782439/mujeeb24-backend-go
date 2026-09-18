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
			MaxOutputTokens  int            `json:"maxOutputTokens"`
			ResponseMimeType string         `json:"responseMimeType"`
			ResponseSchema   map[string]any `json:"responseSchema"`
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
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"intent_base\":\"information_request\",\"domain_context\":\"commerce\",\"entities\":{},\"evidence_references\":[],\"requested_action\":\"answer\",\"response_text\":\"تم استلام رسالتك\",\"confidence_value\":\"0.92\",\"confidence_band\":\"high\",\"requires_human\":false,\"missing_information\":[],\"reason_codes\":[\"greeting\"],\"policy_decision\":\"allowed\",\"policy_version\":\"auto-reply-v1\",\"knowledge_version\":\"none\",\"schema_version\":1}"}],"role":"model"},"finishReason":"STOP"}]}`))
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
	if received.GenerationConfig.MaxOutputTokens != 321 || received.GenerationConfig.ResponseMimeType != "application/json" {
		t.Fatalf("unexpected generationConfig: %#v", received.GenerationConfig)
	}
	if len(received.Contents) != 1 || received.Contents[0].Role != "user" || !strings.Contains(received.Contents[0].Parts[0].Text, "مرحبا") {
		t.Fatalf("unexpected contents: %#v", received.Contents)
	}
	if !strings.Contains(received.SystemInstruction.Parts[0].Text, "المساعد الذكي لخدمة العملاء") {
		t.Fatalf("system prompt missing general rules: %#v", received.SystemInstruction)
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
		// Model receives functionResponse and returns final proposal
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"intent_base\":\"product_inquiry\",\"domain_context\":\"commerce\",\"entities\":{},\"evidence_references\":[\"item-1\",\"offer-1\"],\"requested_action\":\"answer\",\"response_text\":\"آيفون 15 متوفر بسعر 250000 ريال\",\"confidence_value\":\"0.98\",\"confidence_band\":\"high\",\"requires_human\":false,\"missing_information\":[],\"reason_codes\":[\"catalog_hit\"],\"policy_decision\":\"allowed\",\"policy_version\":\"auto-reply-v1\",\"knowledge_version\":\"none\",\"schema_version\":1}"}],"role":"model"},"finishReason":"STOP"}]}`))
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
	if proposal.IntentBase != "product_inquiry" || proposal.RequestedAction != "answer" || proposal.ResponseText != "آيفون 15 متوفر بسعر 250000 ريال" {
		t.Fatalf("unexpected proposal from tool call: %#v", proposal)
	}
}

func TestClientDataDrivenCatalogPaginationCompleteness(t *testing.T) {
	const apiKey = "test-gemini-key"
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// Model calls page 1
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"catalog_data","args":{"operation":"list_catalog_items","catalog_id":"cat-1","limit":1}}}],"role":"model"},"finishReason":"STOP"}]}`))
			return
		}
		if callCount == 2 {
			// Model calls page 2 with cursor
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"catalog_data","args":{"operation":"list_catalog_items","catalog_id":"cat-1","limit":1,"cursor":"cursor-page-2"}}}],"role":"model"},"finishReason":"STOP"}]}`))
			return
		}
		// Model receives exhausted page and formulates complete proposal
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"intent_base\":\"catalog_inquiry\",\"domain_context\":\"commerce\",\"entities\":{},\"evidence_references\":[\"item-1\",\"item-2\"],\"requested_action\":\"answer\",\"response_text\":\"لدينا هاتف 15 وهاتف 16\",\"confidence_value\":\"0.99\",\"confidence_band\":\"high\",\"requires_human\":false,\"missing_information\":[],\"reason_codes\":[\"catalog_complete\"],\"policy_decision\":\"allowed\",\"policy_version\":\"auto-reply-v1\",\"knowledge_version\":\"none\",\"schema_version\":1}"}],"role":"model"},"finishReason":"STOP"}]}`))
	}))
	defer server.Close()

	dispatcher := mockCapabilityDispatcher{
		defs: []ports.AICapabilityDefinition{
			{Name: "catalog_data", Description: "Catalog data access"},
		},
		executeFn: func(ctx context.Context, execCtx ports.AICapabilityExecutionContext, name string, rawParams []byte) (ports.AICapabilityResult, error) {
			var params struct {
				Cursor string `json:"cursor"`
			}
			_ = json.Unmarshal(rawParams, &params)
			if params.Cursor == "" {
				return ports.AICapabilityResult{
					Data:            map[string]any{"items": []any{"item-1"}, "has_more": true, "next_cursor": "cursor-page-2"},
					CatalogEvidence: []ports.AICatalogEvidence{{Reference: "item-1", Name: "Phone 15"}},
					HasMore:         true,
					NextCursor:      "cursor-page-2",
				}, nil
			}
			return ports.AICapabilityResult{
				Data:            map[string]any{"items": []any{"item-2"}, "has_more": false, "next_cursor": ""},
				CatalogEvidence: []ports.AICatalogEvidence{{Reference: "item-2", Name: "Phone 16"}},
				HasMore:         false,
				NextCursor:      "",
			}, nil
		},
	}

	client, err := NewClient(Config{
		BaseURL:          server.URL,
		APIKey:           apiKey,
		Model:            "test-gemini-model",
		RequestTimeout:   time.Second,
		Capabilities:     dispatcher,
		SafetyTurnBudget: 10,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctxValue := &ports.AIContext{}
	proposal, err := client.Decide(context.Background(), ports.AIDecisionInput{
		BusinessID:     "business-1",
		ConversationID: "conversation-1",
		Text:           "ما هي المنتجات المتوفرة؟",
		Context:        ctxValue,
	})
	if err != nil {
		t.Fatalf("Decide with pagination failed: %v", err)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 turns, got %d", callCount)
	}
	if proposal.CatalogIncomplete {
		t.Fatal("expected CatalogIncomplete=false after exhausting pages")
	}
	if proposal.SafetyBudgetExhausted {
		t.Fatal("expected SafetyBudgetExhausted=false")
	}
	if proposal.CatalogRetrievalState != ports.CatalogRetrievalExhausted {
		t.Fatalf("expected CatalogRetrievalState=exhausted, got %s", proposal.CatalogRetrievalState)
	}
	if len(proposal.CatalogStreams) != 1 || proposal.CatalogStreams[0].HasMore || proposal.CatalogStreams[0].PagesFetched != 2 {
		t.Fatalf("unexpected catalog streams: %#v", proposal.CatalogStreams)
	}
	if len(proposal.DiscoveredCatalogEvidence) != 2 {
		t.Fatalf("expected 2 discovered catalog items, got %d", len(proposal.DiscoveredCatalogEvidence))
	}
}

func TestClientMarksIncompleteIfModelStopsBeforeExhaustingPages(t *testing.T) {
	const apiKey = "test-gemini-key"
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// Model calls page 1
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"catalog_data","args":{"operation":"list_catalog_items","catalog_id":"cat-1","limit":1}}}],"role":"model"},"finishReason":"STOP"}]}`))
			return
		}
		// Model prematurely stops and returns proposal without requesting remaining pages
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"intent_base\":\"catalog_inquiry\",\"domain_context\":\"commerce\",\"entities\":{},\"evidence_references\":[\"item-1\"],\"requested_action\":\"answer\",\"response_text\":\"لدينا هاتف 15 فقط\",\"confidence_value\":\"0.50\",\"confidence_band\":\"low\",\"requires_human\":false,\"missing_information\":[],\"reason_codes\":[],\"policy_decision\":\"allowed\",\"policy_version\":\"auto-reply-v1\",\"knowledge_version\":\"none\",\"schema_version\":1}"}],"role":"model"},"finishReason":"STOP"}]}`))
	}))
	defer server.Close()

	dispatcher := mockCapabilityDispatcher{
		defs: []ports.AICapabilityDefinition{
			{Name: "catalog_data", Description: "Catalog data access"},
		},
		executeFn: func(ctx context.Context, execCtx ports.AICapabilityExecutionContext, name string, rawParams []byte) (ports.AICapabilityResult, error) {
			return ports.AICapabilityResult{
				Operation:       "list_catalog_items",
				StreamKey:       "list_catalog_items:catalog=cat-1:limit=1",
				Data:            map[string]any{"items": []any{"item-1"}, "has_more": true, "next_cursor": "cursor-page-2"},
				CatalogEvidence: []ports.AICatalogEvidence{{Reference: "item-1", Name: "Phone 15"}},
				HasMore:         true,
				NextCursor:      "cursor-page-2",
			}, nil
		},
	}

	client, err := NewClient(Config{
		BaseURL:          server.URL,
		APIKey:           apiKey,
		Model:            "test-gemini-model",
		RequestTimeout:   time.Second,
		Capabilities:     dispatcher,
		SafetyTurnBudget: 10,
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
		t.Fatalf("Decide failed: %v", err)
	}
	if !proposal.CatalogIncomplete {
		t.Fatal("expected CatalogIncomplete=true when model stops before exhausting has_more=true")
	}
	if proposal.CatalogRetrievalState != ports.CatalogRetrievalInProgress {
		t.Fatalf("expected CatalogRetrievalState=in_progress, got %s", proposal.CatalogRetrievalState)
	}
	if len(proposal.CatalogStreams) != 1 || !proposal.CatalogStreams[0].HasMore {
		t.Fatalf("unexpected catalog streams: %#v", proposal.CatalogStreams)
	}
}

func TestClientNonPaginatedGetCatalogOperationCompleteness(t *testing.T) {
	const apiKey = "test-gemini-key"
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// Model calls get_catalog_item (single item lookup)
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"catalog_data","args":{"operation":"get_catalog_item","item_id":"item-123"}}}],"role":"model"},"finishReason":"STOP"}]}`))
			return
		}
		// Model returns final proposal
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"intent_base\":\"product_inquiry\",\"domain_context\":\"commerce\",\"entities\":{},\"evidence_references\":[\"item-123\"],\"requested_action\":\"answer\",\"response_text\":\"المنتج هاتف ذكي ممتاز\",\"confidence_value\":\"0.95\",\"confidence_band\":\"high\",\"requires_human\":false,\"missing_information\":[],\"reason_codes\":[\"item_found\"],\"policy_decision\":\"allowed\",\"policy_version\":\"auto-reply-v1\",\"knowledge_version\":\"none\",\"schema_version\":1}"}],"role":"model"},"finishReason":"STOP"}]}`))
	}))
	defer server.Close()

	dispatcher := mockCapabilityDispatcher{
		defs: []ports.AICapabilityDefinition{
			{Name: "catalog_data", Description: "Catalog data access"},
		},
		executeFn: func(ctx context.Context, execCtx ports.AICapabilityExecutionContext, name string, rawParams []byte) (ports.AICapabilityResult, error) {
			return ports.AICapabilityResult{
				Operation:       "get_catalog_item",
				StreamKey:       "get_catalog_item:id=item-123",
				Data:            map[string]any{"item": map[string]any{"id": "item-123", "name": "Phone 15"}},
				CatalogEvidence: []ports.AICatalogEvidence{{Reference: "item-123", Name: "Phone 15"}},
				HasMore:         false,
				NextCursor:      "",
			}, nil
		},
	}

	client, err := NewClient(Config{
		BaseURL:          server.URL,
		APIKey:           apiKey,
		Model:            "test-gemini-model",
		RequestTimeout:   time.Second,
		Capabilities:     dispatcher,
		SafetyTurnBudget: 10,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	proposal, err := client.Decide(context.Background(), ports.AIDecisionInput{
		BusinessID:     "business-1",
		ConversationID: "conversation-1",
		Text:           "معلومات عن المنتج item-123",
		Context:        &ports.AIContext{},
	})
	if err != nil {
		t.Fatalf("Decide failed: %v", err)
	}
	if proposal.CatalogIncomplete {
		t.Fatal("expected CatalogIncomplete=false for get operation with has_more=false")
	}
	if proposal.CatalogRetrievalState != ports.CatalogRetrievalExhausted {
		t.Fatalf("expected CatalogRetrievalState=exhausted, got %s", proposal.CatalogRetrievalState)
	}
	if len(proposal.CatalogStreams) != 1 || proposal.CatalogStreams[0].HasMore || proposal.CatalogStreams[0].Operation != "get_catalog_item" {
		t.Fatalf("unexpected catalog streams: %#v", proposal.CatalogStreams)
	}
	if len(proposal.DiscoveredCatalogEvidence) != 1 || proposal.DiscoveredCatalogEvidence[0].Reference != "item-123" {
		t.Fatalf("unexpected discovered evidence: %#v", proposal.DiscoveredCatalogEvidence)
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
				Operation:       "list_catalog_items",
				StreamKey:       "list_catalog_items:catalog=cat-1",
				Data:            map[string]any{"items": []any{"item-1"}, "has_more": true},
				CatalogEvidence: []ports.AICatalogEvidence{{Reference: "item-1", Name: "Phone 15"}},
				HasMore:         true,
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
		{name: "invalid json", body: `{"candidates":[{"content":{"parts":[{"text":"not-json"}]}}]}`},
		{name: "wrong schema version", body: `{"candidates":[{"content":{"parts":[{"text":"{\"intent_base\":\"x\",\"domain_context\":\"x\",\"entities\":{},\"evidence_references\":[],\"requested_action\":\"answer\",\"response_text\":\"x\",\"confidence_value\":\"\",\"confidence_band\":\"medium\",\"requires_human\":false,\"missing_information\":[],\"reason_codes\":[],\"policy_decision\":\"allowed\",\"policy_version\":\"v1\",\"knowledge_version\":\"none\",\"schema_version\":99}"}],"role":"model"}}]}`},
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

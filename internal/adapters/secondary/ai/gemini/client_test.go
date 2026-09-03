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

func TestClientDecideSendsStructuredGeminiRequest(t *testing.T) {
	const apiKey = "test-gemini-key"
	var received struct {
		SystemInstruction struct {
			Parts []struct{ Text string `json:"text"` } `json:"parts"`
		} `json:"systemInstruction"`
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct{ Text string `json:"text"` } `json:"parts"`
		} `json:"contents"`
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

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: apiKey, Model: "test-gemini-model", RequestTimeout: time.Second, MaxOutputTokens: 321})
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
	if !strings.Contains(received.SystemInstruction.Parts[0].Text, "Mujeeb 24") {
		t.Fatalf("system prompt missing: %#v", received.SystemInstruction)
	}
	if proposal.IntentBase != "information_request" || proposal.RequestedAction != "answer" || proposal.PolicyDecision != "allowed" || proposal.ResponseText != "تم استلام رسالتك" || proposal.ModelReference != "gemini/test-gemini-model" {
		t.Fatalf("unexpected proposal: %#v", proposal)
	}
}

func TestClientRejectsMissingAPIKey(t *testing.T) {
	if _, err := NewClient(Config{BaseURL: "https://example.com", APIKey: "", Model: "gemini-1.5-flash"}); err == nil {
		t.Fatal("expected missing API key error")
	}
}

func TestClientRejectsInvalidOrOversizedResponses(t *testing.T) {
	cases := []struct {
		name    string
		body    string
	}{
		{name: "invalid json", body: `{"candidates":[{"content":{"parts":[{"text":"not-json"}]}}]}`},
		{name: "wrong schema version", body: `{"candidates":[{"content":{"parts":[{"text":"{\"intent_base\":\"x\",\"domain_context\":\"x\",\"entities\":{},\"evidence_references\":[],\"requested_action\":\"answer\",\"response_text\":\"x\",\"confidence_value\":\"\",\"confidence_band\":\"medium\",\"requires_human\":false,\"missing_information\":[],\"reason_codes\":[],\"policy_decision\":\"allowed\",\"policy_version\":\"v1\",\"knowledge_version\":\"none\",\"schema_version\":99}"}],"role":"model"}}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", Model: "model"})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			if _, err := client.Decide(context.Background(), ports.AIDecisionInput{Text: "test"}); err == nil {
				t.Fatal("Decide accepted invalid response")
			}
		})
	}
}

func TestBuildUserPromptIncludesEvidenceButExcludesSensitive(t *testing.T) {
	ctx := &ports.AIContext{
		SchemaVersion: 1,
		Freshness:     "fresh",
		Business:      ports.AIContextBusiness{Reference: "business-1", Name: "متجر", Locale: "ar-YE"},
		Customer: ports.AIContextCustomer{
			Reference:     "customer-1",
			Profile:       []byte(`{"secret":"profile-secret"}`),
			ContactPoints: []byte(`{"phone":"contact-secret"}`),
		},
		CatalogEvidence: []ports.AICatalogEvidence{{Reference: "item-1", Name: "قميص رجالي", Status: "active", Attributes: []byte(`{"color":"black"}`), EvidenceState: "fresh"}},
		OfferEvidence:   []ports.AIOfferEvidence{{Reference: "offer-1", Name: "قميص", AvailabilityState: "available", Amount: "12000", Currency: "YER", EvidenceState: "fresh"}},
		RecentMessages:  []ports.AIRecentMessageEvidence{{Reference: "message-1", Direction: "inbound", Text: "هل هو متوفر؟", EvidenceState: "fresh"}},
		PolicyEvidence:  ports.AIPolicyEvidence{Version: "auto-reply-v1", State: "application_policy_only"},
	}
	prompt := buildUserPrompt(ports.AIDecisionInput{BusinessID: "business-1", ConversationID: "conversation-1", Text: "كم سعر القميص؟", Context: ctx})
	for _, expected := range []string{"قميص رجالي", "available", "12000", "YER", "هل هو متوفر؟"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt omitted evidence %q: %s", expected, prompt)
		}
	}
	for _, secret := range []string{"profile-secret", "contact-secret"} {
		if strings.Contains(prompt, secret) {
			t.Fatalf("prompt leaked secret %q", secret)
		}
	}
}

func TestClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{}"}]}}]}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", Model: "model", RequestTimeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.Decide(context.Background(), ports.AIDecisionInput{Text: "hello"})
	if err == nil || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("expected timeout, got %v", err)
	}
}

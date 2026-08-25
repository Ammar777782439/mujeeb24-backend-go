package openaicompatible

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

func TestClientDecideSendsStrictStructuredProposalRequest(t *testing.T) {
	const apiKey = "test-only-key"
	var received struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		ResponseFormat struct {
			Type       string `json:"type"`
			JSONSchema struct {
				Name   string         `json:"name"`
				Strict bool           `json:"strict"`
				Schema map[string]any `json:"schema"`
			} `json:"json_schema"`
		} `json:"response_format"`
		MaxCompletionTokens int `json:"max_completion_tokens"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
			t.Errorf("authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"intent_base\":\"information_request\",\"domain_context\":\"customer_support\",\"entities\":{},\"evidence_references\":[],\"requested_action\":\"answer\",\"response_text\":\"تم استلام رسالتك\",\"confidence_value\":\"0.92\",\"confidence_band\":\"high\",\"requires_human\":false,\"missing_information\":[],\"reason_codes\":[\"greeting\"],\"policy_decision\":\"allowed\",\"policy_version\":\"auto-reply-v1\",\"knowledge_version\":\"none\",\"schema_version\":1}"}}]}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: apiKey, Model: "test-model", RequestTimeout: time.Second, MaxOutputTokens: 321})
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
	if received.Model != "test-model" || received.MaxCompletionTokens != 321 || received.ResponseFormat.Type != "json_schema" || received.ResponseFormat.JSONSchema.Name != "mujeeb_ai_decision_proposal" || !received.ResponseFormat.JSONSchema.Strict {
		t.Fatalf("unexpected request: %#v", received)
	}
	if received.ResponseFormat.JSONSchema.Schema["additionalProperties"] != false {
		t.Fatalf("schema must reject additional properties: %#v", received.ResponseFormat.JSONSchema.Schema)
	}
	if len(received.Messages) != 2 || received.Messages[0].Role != "system" || received.Messages[1].Role != "user" || !strings.Contains(received.Messages[1].Content, "مرحبا") {
		t.Fatalf("unexpected messages: %#v", received.Messages)
	}
	if proposal.IntentBase != "information_request" || proposal.RequestedAction != "answer" || proposal.PolicyDecision != "allowed" || proposal.ResponseText != "تم استلام رسالتك" || proposal.ModelReference != "openai-compatible/test-model" || proposal.SchemaVersion != 1 {
		t.Fatalf("unexpected proposal: %#v", proposal)
	}
}

func TestClientSupportsMaxTokensField(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"intent_base\":\"information_request\",\"domain_context\":\"support\",\"entities\":{},\"evidence_references\":[],\"requested_action\":\"no_action\",\"response_text\":\"\",\"confidence_value\":\"\",\"confidence_band\":\"low\",\"requires_human\":true,\"missing_information\":[],\"reason_codes\":[],\"policy_decision\":\"requires_approval\",\"policy_version\":\"v1\",\"knowledge_version\":\"none\",\"schema_version\":1}"}}]}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", Model: "model", OutputTokensField: "max_tokens"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.Decide(context.Background(), ports.AIDecisionInput{Text: "test"}); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if _, ok := received["max_tokens"]; !ok {
		t.Fatalf("expected max_tokens field: %#v", received)
	}
	if _, ok := received["max_completion_tokens"]; ok {
		t.Fatalf("did not expect max_completion_tokens field: %#v", received)
	}
}

func TestClientRejectsInvalidOrOversizedResponses(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "invalid json", content: "not-json"},
		{name: "wrong schema version", content: `{"intent_base":"x","domain_context":"x","entities":{},"evidence_references":[],"requested_action":"answer","response_text":"x","confidence_value":"","confidence_band":"medium","requires_human":false,"missing_information":[],"reason_codes":[],"policy_decision":"allowed","policy_version":"v1","knowledge_version":"none","schema_version":99}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + mustJSONString(tc.content) + `}}]}`))
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

func mustJSONString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func TestBuildUserPromptIncludesEvidenceButExcludesSensitiveCustomerDocuments(t *testing.T) {
	contextValue := &ports.AIContext{
		SchemaVersion: 1,
		Freshness:     "fresh",
		Business:      ports.AIContextBusiness{Reference: "business-1", Name: "متجر", Locale: "ar-YE"},
		Customer: ports.AIContextCustomer{
			Reference:     "customer-1",
			Profile:       []byte(`{"secret":"profile-secret"}`),
			ContactPoints: []byte(`{"phone":"contact-secret"}`),
		},
		CatalogEvidence: []ports.AICatalogEvidence{{Reference: "item-1", Name: "iPhone 15", Status: "active", Attributes: []byte(`{"color":"black"}`), EvidenceState: "fresh"}},
		OfferEvidence:   []ports.AIOfferEvidence{{Reference: "offer-1", Name: "iPhone offer", AvailabilityState: "available", Amount: "250000", Currency: "YER", EvidenceState: "fresh"}},
		RecentMessages:  []ports.AIRecentMessageEvidence{{Reference: "message-1", Direction: "inbound", Text: "هل هو متوفر؟", EvidenceState: "fresh"}},
		PolicyEvidence:  ports.AIPolicyEvidence{Version: "auto-reply-v1", State: "application_policy_only"},
	}
	prompt := buildUserPrompt(ports.AIDecisionInput{BusinessID: "business-1", ConversationID: "conversation-1", Text: "هل الآيفون متوفر؟", Context: contextValue})
	for _, expected := range []string{"iPhone 15", "available", "250000", "YER", "هل هو متوفر؟"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt omitted grounded evidence %q: %s", expected, prompt)
		}
	}
	for _, secret := range []string{"profile-secret", "contact-secret"} {
		if strings.Contains(prompt, secret) {
			t.Fatalf("prompt leaked sensitive customer document %q", secret)
		}
	}
}

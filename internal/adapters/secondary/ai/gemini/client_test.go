// Package gemini — Legacy Client tests.
//
// The legacy Client.Decide method (which returns the legacy AIDecisionProposal)
// is kept for migration safety; production code now uses ContractClient.DecideContract
// which returns the contract ④ §4 AIGeminiProposal.
//
// Tests for the legacy catalog retrieval session tracking (DiscoveredCatalogEvidence,
// CatalogStreams, CatalogRetrievalState, SafetyBudgetExhausted, CatalogIncomplete)
// were DELETED per contract ④ §5 + ⑥ §20: these fields mixed Gemini's output
// with Mujeeb's tracking — explicitly forbidden by contract ④ §5.
//
// Coverage for the contract-aligned path lives in:
//   - internal/application/services/ai_contracts_test.go (lifecycle, retry, validation)
//   - internal/application/services/auto_reply_test.go (end-to-end AutoReply flow)
//   - internal/adapters/secondary/ai/gemini/client_contracts.go (ContractClient impl)

package gemini

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// TestLegacyClientDecideReturnsContract4Action verifies that the legacy
// Client.Decide method (when called with a properly mocked Gemini response)
// returns a proposal with the contract ④ §4 action value.
//
// Per contract ④ §4, the closed action set is:
//
//	answer, clarification, human_request, lead_draft, order_draft.
//
// The legacy Client.Decide is being phased out in favor of ContractClient.DecideContract
// which enforces Structured Output via responseSchema. This test is kept until
// the legacy method is removed.
func TestLegacyClientDecideReturnsContract4Action(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"{\"intent_base\":\"resolved\",\"domain_context\":\"support\",\"entities\":{},\"evidence_references\":[],\"requested_action\":\"answer\",\"response_text\":\"تم استلام رسالتك\",\"confidence_value\":\"0.9\",\"confidence_band\":\"high\",\"requires_human\":false,\"missing_information\":[],\"reason_codes\":[\"greeting\"],\"policy_decision\":\"allowed\",\"policy_version\":\"auto-reply-v1\",\"knowledge_version\":\"none\",\"schema_version\":1}"}]}}]}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:        server.URL,
		APIKey:         "test-only-key",
		Model:          "test-model",
		RequestTimeout: 5_000_000_000,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	proposal, err := client.Decide(context.Background(), ports.AIDecisionInput{
		Text:          "مرحبا",
		PolicyVersion: "auto-reply-v1",
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if proposal.RequestedAction != "answer" {
		t.Fatalf("expected action=answer per contract ④ §4, got %q", proposal.RequestedAction)
	}
	if proposal.ResponseText == "" {
		t.Fatalf("expected non-empty response_text per contract ④ §4")
	}
}

// TestLegacyClientDecideRejectsEmptyText verifies that an empty input text
// fails immediately per contract ④ §3 (user_message is required).
func TestLegacyClientDecideRejectsEmptyText(t *testing.T) {
	client, err := NewClient(Config{
		BaseURL:        "http://localhost",
		APIKey:         "test-only-key",
		Model:          "test-model",
		RequestTimeout: 5_000_000_000,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := client.Decide(context.Background(), ports.AIDecisionInput{Text: ""}); err == nil {
		t.Fatal("expected error for empty text per contract ④ §3")
	}
}

// TestLegacyClientDecideRejectsMissingAPIKey verifies configuration validation.
func TestLegacyClientDecideRejectsMissingAPIKey(t *testing.T) {
	_, err := NewClient(Config{
		BaseURL:        "http://localhost",
		APIKey:         "",
		Model:          "test-model",
		RequestTimeout: 5_000_000_000,
	})
	if err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("expected API key error, got %v", err)
	}
}

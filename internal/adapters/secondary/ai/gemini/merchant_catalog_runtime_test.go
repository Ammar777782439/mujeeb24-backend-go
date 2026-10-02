package gemini

import (
	"strings"
	"testing"
)

func TestParseMerchantCatalogProposalRejectsIncompleteInteraction(t *testing.T) {
	resp := merchantCatalogInteractionResponse{
		ID:     "test-interaction",
		Status: "incomplete",
		Steps: []merchantCatalogInteractionStep{
			{
				Type: "thought",
			},
			{
				Type:    "model_output",
				Content: []merchantCatalogOutputPart{{Type: "text", Text: "{\"schema_version\":3,\"status\""}},
			},
		},
	}

	// The runtime must reject incomplete interactions before trying to decode
	// partial JSON. The caller gets the real provider/runtime state instead of
	// the misleading "unexpected end of JSON input".
	err := incompleteMerchantCatalogInteractionError(resp, 700)
	if err == nil {
		t.Fatal("expected incomplete interaction error")
	}
	msg := err.Error()
	for _, want := range []string{"status=incomplete", "steps=2", "max_output_tokens=700"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not contain %q", msg, want)
		}
	}
}

func TestMerchantCatalogThinkingLevel(t *testing.T) {
	for _, model := range []string{
		"gemini-3.5-flash-lite",
		"gemini-3.5-flash-lite-001",
		"gemini-3.5-flash",
		"gemini-3.6-flash",
		"gemini-3-flash-preview",
	} {
		if got := merchantCatalogThinkingLevel(model); got != "minimal" {
			t.Fatalf("model %q thinking level = %q, want minimal", model, got)
		}
	}

	if got := merchantCatalogThinkingLevel("gemini-3.8-flash"); got != "" {
		t.Fatalf("gemini-3.8-flash thinking level = %q, want empty because its supported levels differ", got)
	}
}

func TestIsRetryableGeminiStatus(t *testing.T) {
	for _, status := range []int{408, 429, 500, 502, 503, 504} {
		if !isRetryableGeminiStatus(status) {
			t.Fatalf("status %d should be retryable", status)
		}
	}
	for _, status := range []int{400, 401, 402, 403, 404} {
		if isRetryableGeminiStatus(status) {
			t.Fatalf("status %d should not be retryable", status)
		}
	}
}

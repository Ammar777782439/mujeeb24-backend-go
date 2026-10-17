package litellm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGatewayNativeCostAndCallID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-litellm-call-id", "gateway-call-1")
		w.Header().Set("x-litellm-model-id", "deployment-123")
		w.Header().Set("x-litellm-response-cost", "2.85e-05")
		_, _ = w.Write([]byte(`{"status":"completed"}`))
	}))
	defer srv.Close()
	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "virtual-key"})
	if err != nil {
		t.Fatal(err)
	}
	_, meta, err := client.SendChatCompletionWithMetadata(context.Background(), map[string]any{"model": "mujeeb-customer"})
	if err != nil {
		t.Fatal(err)
	}
	if !meta.HasCost || meta.CostUSD != "2.85e-05" || meta.CallID != "gateway-call-1" || meta.DeploymentID != "deployment-123" {
		t.Fatalf("lost native gateway metadata: %#v", meta)
	}
}

func TestGatewayMissingVsInvalidCost(t *testing.T) {
	for _, tc := range []struct {
		value          string
		known, wantErr bool
	}{
		{"", false, false}, {"0", true, false}, {"NaN", false, true},
		{"Inf", false, true}, {"-0.1", false, true}, {"not-a-number", false, true},
	} {
		h := http.Header{}
		if tc.value != "" {
			h.Set("x-litellm-response-cost", tc.value)
		}
		m, err := metadataFromHeaders(h)
		if (err != nil) != tc.wantErr || (err == nil && m.HasCost != tc.known) {
			t.Errorf("cost %q: got %#v, %v", tc.value, m, err)
		}
	}
}

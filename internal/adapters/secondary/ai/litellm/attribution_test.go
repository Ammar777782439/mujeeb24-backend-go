package litellm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSpendAttributionIsAppliedToGatewayRequests(t *testing.T) {
	var gotCustomer string
	var gotMetadata map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCustomer = r.Header.Get("x-litellm-customer-id")
		if err := json.Unmarshal([]byte(r.Header.Get("x-litellm-spend-logs-metadata")), &gotMetadata); err != nil {
			t.Errorf("metadata header not valid JSON: %v", err)
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "test-virtual-key"})
	if err != nil { t.Fatal(err) }
	ctx := WithSpendAttribution(context.Background(), "business-1", "run-123", "catalog_batch")
	if _, _, err := client.SendInteractionWithMetadata(ctx, map[string]any{"model":"mujeeb-catalog"}); err != nil {
		t.Fatal(err)
	}
	if gotCustomer != "business-1" || gotMetadata["mujeeb_business_id"] != "business-1" ||
		gotMetadata["mujeeb_trace_id"] != "run-123" || gotMetadata["mujeeb_operation"] != "catalog_batch" {
		t.Fatalf("missing LiteLLM native attribution: customer=%q metadata=%v", gotCustomer, gotMetadata)
	}
}

func TestSpendAttributionDoesNotInventCustomer(t *testing.T) {
	req, err := http.NewRequestWithContext(context.Background(), "POST", "http://localhost", nil)
	if err != nil { t.Fatal(err) }
	applySpendAttribution(req)
	if req.Header.Get("x-litellm-customer-id") != "" ||
		req.Header.Get("x-litellm-spend-logs-metadata") != "" {
		t.Fatal("request with no tenant attribution unexpectedly received billing tags")
	}
}

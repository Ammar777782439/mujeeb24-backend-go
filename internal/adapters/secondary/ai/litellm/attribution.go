package litellm

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// spendAttribution remains internal to Mujeeb's trusted backend. LiteLLM
// tracks the customer and per-request metadata across retries and fallbacks.
type spendAttribution struct {
	BusinessID string
	TraceID    string
	Operation  string
}
type spendAttributionKey struct{}

// WithSpendAttribution links LiteLLM's native spend logs to Mujeeb's tenant
// and AI run. It never carries message content, API keys, or personal data.
func WithSpendAttribution(ctx context.Context, businessID, traceID, operation string) context.Context {
	if strings.TrimSpace(businessID) == "" {
		return ctx
	}
	return context.WithValue(ctx, spendAttributionKey{}, spendAttribution{
		BusinessID: businessID,
		TraceID: traceID,
		Operation: operation,
	})
}

func applySpendAttribution(req *http.Request) {
	attribution, ok := req.Context().Value(spendAttributionKey{}).(spendAttribution)
	if !ok || attribution.BusinessID == "" {
		return
	}
	// LiteLLM natively tracks this customer on all routed deployments.
	req.Header.Set("x-litellm-customer-id", attribution.BusinessID)
	// Spend-log metadata, not prompt/body metadata: no changes to the AI schema.
	values := map[string]string{"mujeeb_business_id": attribution.BusinessID}
	if attribution.TraceID != "" {
		values["mujeeb_trace_id"] = attribution.TraceID
	}
	if attribution.Operation != "" {
		values["mujeeb_operation"] = attribution.Operation
	}
	encoded, err := json.Marshal(values)
	if err == nil {
		req.Header.Set("x-litellm-spend-logs-metadata", string(encoded))
	}
}

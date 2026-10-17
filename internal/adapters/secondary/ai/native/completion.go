package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// CompletionClient is the provider-neutral model-generation boundary.
// No business tool is executed here; the caller remains responsible for
// canonical output validation, authorization and business transactions.
type CompletionClient struct {
	router *Router
}

// CompletionInfo is immutable per call and safe for concurrent requests.
// A failed provider attempt does not fabricate a token count or cost.
type CompletionInfo struct {
	Provider        string
	Model           string
	InputTokens     int64
	OutputTokens    int64
	CachedTokens    int64
	ProviderCostUSD float64
	CostKnown       bool
	Trace           Trace
}

func NewCompletionClient(router *Router) (*CompletionClient, error) {
	if router == nil {
		return nil, errors.New("native inference router is required")
	}
	return &CompletionClient{router: router}, nil
}

// Complete serializes an existing canonical inference request and returns the
// same normalized Chat Completions JSON envelope used by the current adapters.
// No provider-specific endpoint, key, or retries reach application services.
func (c *CompletionClient) Complete(ctx context.Context, payload any) (json.RawMessage, CompletionInfo, error) {
	var info CompletionInfo
	if c == nil || c.router == nil {
		return nil, info, errors.New("native inference is unconfigured")
	}
	if err := ctx.Err(); err != nil {
		return nil, info, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, info, fmt.Errorf("serialize canonical inference request: %w", err)
	}
	if len(body) == 0 || !json.Valid(body) || string(body) == "null" {
		return nil, info, errors.New("canonical inference request must be JSON object")
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil || root == nil {
		return nil, info, errors.New("canonical inference request must be JSON object")
	}
	response, trace, err := c.router.Generate(ctx, Request{Payload: body})
	info.Trace = trace
	if err != nil {
		return nil, info, err
	}
	if len(response.Payload) == 0 || !json.Valid(response.Payload) {
		return nil, info, errors.New("provider returned invalid completion JSON")
	}
	if response.InputTokens < 0 || response.OutputTokens < 0 || response.CachedTokens < 0 ||
		response.CachedTokens > response.InputTokens || response.ProviderCostUSD < 0 {
		return nil, info, errors.New("provider returned impossible token or cost counters")
	}
	if response.CostKnown && response.ProviderCostUSD < 0 {
		return nil, info, errors.New("provider cost invalid")
	}
	info.Provider = trace.Attempts[len(trace.Attempts)-1].Provider
	info.Model = response.Model
	info.InputTokens = response.InputTokens
	info.OutputTokens = response.OutputTokens
	info.CachedTokens = response.CachedTokens
	info.ProviderCostUSD = response.ProviderCostUSD
	info.CostKnown = response.CostKnown
	return response.Payload, info, nil
}

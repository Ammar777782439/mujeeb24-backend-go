package litellm

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
)

// ResponseMetadata is supplied by LiteLLM, not calculated from Mujeeb token prices.
// CostUSD is the untouched USD decimal header; HasCost distinguishes missing from free.
// CallID allows reconciliation against LiteLLM spend logs including routed deployments.
type ResponseMetadata struct {
	CallID       string
	DeploymentID string
	CostUSD      string
	HasCost      bool
}

func metadataFromHeaders(headers http.Header) (ResponseMetadata, error) {
	m := ResponseMetadata{
		CallID:       strings.TrimSpace(headers.Get("x-litellm-call-id")),
		DeploymentID: strings.TrimSpace(headers.Get("x-litellm-model-id")),
	}
	cost := strings.TrimSpace(headers.Get("x-litellm-response-cost"))
	if cost == "" {
		return m, nil
	}
	parsed, err := strconv.ParseFloat(cost, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 {
		return ResponseMetadata{}, errors.New("invalid LiteLLM cost metadata")
	}
	m.CostUSD = cost
	m.HasCost = true
	return m, nil
}

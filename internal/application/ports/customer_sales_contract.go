// Package ports contains the application contracts used by Mujeeb AI capabilities.

package ports

import "context"

// CustomerSales contract types are B2C-specific. They define the customer-facing
// proposal, catalog-evaluation results, AI provider conversation continuity, and the
// application port consumed by Customer Sales AI.

// SelectedReference is a per-contract ④ §4 reference to a catalog entity that
// AI provider's proposal depends on. All three IDs are required to be UUIDs that
// were actually present in the evidence Mujeeb sent — never invented by AI provider.
//
// variant_id and offer_id may be nil when the relation does not apply or is
// not required (e.g., a service-type item with no variants).
type SelectedReference struct {
	ItemID    string  `json:"item_id"`
	VariantID *string `json:"variant_id,omitempty"`
	OfferID   *string `json:"offer_id,omitempty"`
}

// CatalogBatchCandidate is per-contract ② §5 — the per-batch evaluation result
// AI provider returns. Mujeeb already knows which items it sent, so AI provider only
// returns the IDs it considered candidates plus a short reason.
//
// variant_ids and offer_ids are plural because one item may produce multiple
// variants/offers that are all candidates.
type CatalogBatchCandidate struct {
	ItemID     string   `json:"item_id"`
	VariantIDs []string `json:"variant_ids,omitempty"`
	OfferIDs   []string `json:"offer_ids,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

// CatalogBatchResult is the contract ② §5 per-batch evaluation output.
//
// Per contract ② §7, AI provider does NOT decide whether a batch was "skipped" or
// "completed". Mujeeb (the Catalog Batch Controller) owns coverage; AI provider only
// returns the candidates it inferred from the items in this batch.
type CatalogBatchResult struct {
	BatchNumber int                     `json:"batch_number"`
	Candidates  []CatalogBatchCandidate `json:"candidates"`
	// Usage telemetry from the AI provider response for this batch call.
	// Per AIUsageTokenTelemetry.md §6: every AI provider call's tokens must be
	// recorded. This field carries the usageMetadata from the batch's
	// AI provider response so the CatalogBatchController can persist it.
	Usage CustomerSalesUsageTelemetry `json:"-"`
	// LatencyMs is the wall-clock duration of the AI provider batch call.
	// Per P2-13: this is the REAL latency from start-of-request to
	// end-of-response, NOT derived from EstimatedCostMicros (which is
	// always 0 — see client_contracts.go). The controller uses this to
	// compute StartedAt = CompletedAt - LatencyMs for the usage record.
	LatencyMs int64 `json:"-"`
}

// ════════════════════════════════════════════════════════════════════════════
// Contract ④ §8 — CustomerSalesDecisionPort
// ════════════════════════════════════════════════════════════════════════════

// CustomerSalesDecisionPort is the contract ④ §8 mapping of Mujeeb Contract to AI provider API.
//
// Per contract ④ §8:
//
//	Mujeeb System Contract → system_instruction
//	Mujeeb Input Context → input (contents)
//	Catalog boundary → Function Calling / tool
//	Mujeeb Output Contract → one offered Function Call
//
// CustomerSalesDecisionPort is the application port for customer-facing sales decisions.
//
// Per contract ⑤ §7, the Catalog Entity Contract is sent as part of system
// instruction; it is passed through as opaque JSON.
type CustomerSalesDecisionPort interface {
	Decide(ctx context.Context, input CustomerSalesDecisionInput) (CustomerSalesDecisionOutput, error)
}

// CustomerSalesDecisionInput is the complete request envelope for CustomerSalesDecisionPort.Decide.
type CustomerSalesDecisionInput struct {
	Capabilities []CustomerSalesCapability

	// Request is the customer-facing business request and grounded Mujeeb context.
	Request CustomerSalesDecisionRequest

	// EntityContractPayload is the Catalog Entity Contract sent to AI provider.
	EntityContractPayload []byte

	// AIRunID links customer-sales tool calls to the current AI Run trace.
	AIRunID string
}

// CustomerSalesDecisionOutput is the output of CustomerSalesDecisionPort.Decide.
type CustomerSalesDecisionOutput struct {
	// Decision remains authoritative through validation, policy and execution.
	Decision CapabilityDecision

	// Usage per contract ⑧ §8 (token counts + estimated cost).
	Usage CustomerSalesUsageTelemetry

	// LatencyMs per contract ⑧ §9 (the AI provider API call latency).
	LatencyMs int64
}

// CustomerSalesUsageTelemetry is the per-call usage data per contract ⑧ §8.
type CustomerSalesUsageTelemetry struct {
	InputTokens         int
	CachedTokens        int
	OutputTokens        int
	Provider            string
	Model               string
	EstimatedCostMicros int64
	// Optional provider-verified USD spend metadata; unknown cost must not be fabricated.
	GatewayCallID       string
	GatewayDeploymentID string
	GatewayCostUSD      string
	GatewayCostKnown    bool
	// LatencyMs is the wall-clock duration of the AI provider call.
	// Per P2-13: the controller uses this to compute StartedAt =
	// CompletedAt - LatencyMs for the usage record. Before this field,
	// the controller derived latency from EstimatedCostMicros (always 0)
	// — resulting in StartedAt == CompletedAt + latency=0.
	LatencyMs int64
	// ModelRequests is the actual count of AI provider API calls made during
	// this customer-sales Decide invocation. Per fix #1: 1 for non-tool path,
	// N for tool loop (one per sendContractRequest call).
	// The caller (recordAIUsage) uses this instead of hardcoding 1.
	ModelRequests int
}

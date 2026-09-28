package ports

import (
	"context"
)

// AICapabilityDefinition specifies the name, description, and parameter JSON
// schema for an application-level AI capability exposed to LLMs.
type AICapabilityDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// AICapabilityExecutionContext encapsulates server-side trusted execution context.
// BusinessID is strictly determined by the server and never by the AI model.
type AICapabilityExecutionContext struct {
	BusinessID     string
	ConversationID string
	PrincipalID    string
	Role           string
	Permissions    []string
	RequestID      string
	CorrelationID  string
}

// AICapabilityResult contains the bounded factual data returned to the LLM
// along with evidence metadata for downstream policy grounding and data-driven completion.
type AICapabilityResult struct {
	Data            any                 `json:"data"`
	CatalogEvidence []AICatalogEvidence `json:"catalog_evidence,omitempty"`
	OfferEvidence   []AIOfferEvidence   `json:"offer_evidence,omitempty"`
	VariantEvidence []AIVariantEvidence `json:"variant_evidence,omitempty"`
	HasMore         bool                `json:"has_more,omitempty"`
	NextCursor      string              `json:"next_cursor,omitempty"`
	Incomplete      bool                `json:"incomplete,omitempty"`
	Operation       string              `json:"operation,omitempty"`
	StreamKey       string              `json:"stream_key,omitempty"`
}

// AICapability represents an individual application-level AI capability.
type AICapability interface {
	Definition() AICapabilityDefinition
	Execute(ctx context.Context, execCtx AICapabilityExecutionContext, rawParams []byte) (AICapabilityResult, error)
}

// AICapabilityDispatcher dispatches capability calls by name and provides definitions.
type AICapabilityDispatcher interface {
	Definitions() []AICapabilityDefinition
	Execute(ctx context.Context, execCtx AICapabilityExecutionContext, name string, rawParams []byte) (AICapabilityResult, error)
}

// AICapabilityRegistry supports registering and dispatching capabilities.
type AICapabilityRegistry interface {
	AICapabilityDispatcher
	Register(capability AICapability) error
	Get(name string) (AICapability, bool)
}

// IncorporateCapabilityEvidence merges evidence returned by capability execution
// into the AIContext so downstream policy validation recognizes verified facts.
func IncorporateCapabilityEvidence(target *AIContext, result AICapabilityResult) {
	if target == nil {
		return
	}
	if len(result.CatalogEvidence) > 0 {
		seen := make(map[string]bool, len(target.CatalogEvidence))
		for _, e := range target.CatalogEvidence {
			seen[e.Reference] = true
		}
		for _, e := range result.CatalogEvidence {
			if !seen[e.Reference] {
				target.CatalogEvidence = append(target.CatalogEvidence, e)
				seen[e.Reference] = true
			}
		}
	}
	if len(result.OfferEvidence) > 0 {
		seen := make(map[string]bool, len(target.OfferEvidence))
		for _, e := range target.OfferEvidence {
			seen[e.Reference] = true
		}
		for _, e := range result.OfferEvidence {
			if !seen[e.Reference] {
				target.OfferEvidence = append(target.OfferEvidence, e)
				seen[e.Reference] = true
			}
		}
	}
	if len(result.VariantEvidence) > 0 {
		seen := make(map[string]bool, len(target.VariantEvidence))
		for _, e := range target.VariantEvidence {
			seen[e.Reference] = true
		}
		for _, e := range result.VariantEvidence {
			if !seen[e.Reference] {
				target.VariantEvidence = append(target.VariantEvidence, e)
				seen[e.Reference] = true
			}
		}
	}
}

// IncorporateProposalEvidence is DEPRECATED per contract ④ §5 + ⑥ §20.
//
// The legacy AIDecisionProposal carried Mujeeb-side discovered evidence
// (DiscoveredCatalogEvidence, etc.) which mixed Gemini's output with Mujeeb's
// tracking — explicitly forbidden by contract ④ §5.
//
// Per contract ⑥ §20, validation is now deterministic via ValidationPipeline
// (in services/ai_validation_pipeline.go). Per contract ⑧ §5, the operational
// trace lives in ai_runs/ai_tool_calls/ai_catalog_batches tables, NOT in
// the AI Proposal.
//
// This function is kept as a no-op for migration safety; it will be removed
// once all callers are migrated to the contract-aligned flow.
func IncorporateProposalEvidence(target *AIContext, proposal AIDecisionProposal) {
	if target == nil {
		return
	}
	// No-op: deprecated. The contract-aligned flow does NOT carry discovered
	// evidence in the AI Proposal; it lives in ai_runs + ai_tool_calls rows.
	_ = proposal
}

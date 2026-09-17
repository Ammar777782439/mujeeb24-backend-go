package services

import (
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestGroundedPolicyEngineRequiresApprovalForMissingAvailabilityEvidence(t *testing.T) {
	proposal := ports.AIDecisionProposal{IntentBase: "availability_inquiry", RequestedAction: AutoReplyActionAnswer, PolicyDecision: "allowed", ConfidenceBand: "high", SchemaVersion: 1}
	contextValue := &ports.AIContext{KnowledgeState: AIContextMissing}
	result := (GroundedPolicyEngine{}).Evaluate(proposal, contextValue)
	if result.PolicyDecision != "requires_approval" || !result.RequiresHuman || string(result.ReasonCodes) != `["verified_offer_evidence_missing_or_stale"]` {
		t.Fatalf("missing availability evidence was not blocked: %#v", result)
	}
}

func TestGroundedPolicyEngineAllowsVerifiedAvailabilityWithPolicyAndReference(t *testing.T) {
	proposal := ports.AIDecisionProposal{IntentBase: "availability_inquiry", RequestedAction: AutoReplyActionAnswer, PolicyDecision: "allowed", ConfidenceBand: "high", SchemaVersion: 1, EvidenceReferences: []byte(`["offer-1"]`)}
	contextValue := &ports.AIContext{
		OfferEvidence:          []ports.AIOfferEvidence{{Reference: "offer-1", AvailabilityState: "available", EvidenceState: AIContextFresh}},
		BusinessPolicyEvidence: []ports.AIBusinessPolicyEvidence{{Reference: "policy-1", Category: "availability", EvidenceState: AIContextFresh}},
	}
	result := (GroundedPolicyEngine{}).Evaluate(proposal, contextValue)
	if result.PolicyDecision != "allowed" || result.RequiresHuman || string(result.EvidenceReferences) != "[\"offer-1\"]" {
		t.Fatalf("verified availability was changed unexpectedly: %#v", result)
	}
}

func TestGroundedPolicyEngineRequiresApprovalForStaleOfferAndMissingPolicy(t *testing.T) {
	proposal := ports.AIDecisionProposal{IntentBase: "price_inquiry", RequestedAction: AutoReplyActionAnswer, PolicyDecision: "allowed", ConfidenceBand: "high", SchemaVersion: 1, EvidenceReferences: []byte(`["offer-1"]`)}
	contextValue := &ports.AIContext{
		OfferEvidence:          []ports.AIOfferEvidence{{Reference: "offer-1", AvailabilityState: "stale", EvidenceState: AIContextStale}},
		BusinessPolicyEvidence: []ports.AIBusinessPolicyEvidence{{Reference: "policy-1", Category: "pricing", EvidenceState: AIContextFresh}},
	}
	result := (GroundedPolicyEngine{}).Evaluate(proposal, contextValue)
	if result.PolicyDecision != "requires_approval" || !result.RequiresHuman || string(result.ReasonCodes) != `["verified_offer_evidence_missing_or_stale"]` {
		t.Fatalf("stale offer was not blocked: %#v", result)
	}
}

func TestGroundedPolicyEngineRequiresBusinessPolicyForHours(t *testing.T) {
	proposal := ports.AIDecisionProposal{IntentBase: "opening_hours", RequestedAction: AutoReplyActionAnswer, PolicyDecision: "allowed", ConfidenceBand: "high", SchemaVersion: 1, EvidenceReferences: []byte(`["knowledge-1"]`)}
	contextValue := &ports.AIContext{KnowledgeEvidence: []ports.AIKnowledgeEvidence{{Reference: "knowledge-1", EvidenceState: AIContextFresh}}}
	result := (GroundedPolicyEngine{}).Evaluate(proposal, contextValue)
	if result.PolicyDecision != "requires_approval" || !result.RequiresHuman {
		t.Fatalf("hours without merchant policy was not blocked: %#v", result)
	}
}

func TestGroundedPolicyEngineDoesNotBlockNonFactualGreeting(t *testing.T) {
	proposal := ports.AIDecisionProposal{IntentBase: "greeting", RequestedAction: AutoReplyActionAnswer, PolicyDecision: "allowed", ConfidenceBand: "high", SchemaVersion: 1}
	contextValue := &ports.AIContext{KnowledgeState: AIContextMissing}
	result := (GroundedPolicyEngine{}).Evaluate(proposal, contextValue)
	if result.PolicyDecision != "allowed" || result.RequiresHuman {
		t.Fatalf("non-factual greeting was blocked: %#v", result)
	}
}

func TestReferencesValidCatalogEvidence(t *testing.T) {
	contextValue := &ports.AIContext{
		CatalogEvidence: []ports.AICatalogEvidence{
			{Reference: "item-1", EvidenceState: AIContextFresh},
			{Reference: "item-2", EvidenceState: AIContextStale},
		},
	}
	cases := []struct {
		name string
		raw  []byte
		want bool
	}{
		{"nil raw", nil, false},
		{"malformed json", []byte(`not-json`), false},
		{"empty array", []byte(`[]`), false},
		{"fresh hit", []byte(`["item-1"]`), true},
		{"stale only", []byte(`["item-2"]`), false},
		{"unknown ref", []byte(`["item-ghost"]`), false},
		{"mixed hit and ghost", []byte(`["item-ghost","item-1"]`), true},
		{"whitespace padded", []byte(`["  item-1  "]`), true},
	}
	for _, tc := range cases {
		if got := referencesValidCatalogEvidence(tc.raw, contextValue); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestGroundedPolicyEngineAllowsVerifiedCatalogWithGeneralPolicy(t *testing.T) {
	proposal := ports.AIDecisionProposal{IntentBase: "product_inquiry", RequestedAction: AutoReplyActionAnswer, PolicyDecision: "allowed", ConfidenceBand: "high", SchemaVersion: 1, EvidenceReferences: []byte(`["item-1"]`)}
	contextValue := &ports.AIContext{
		CatalogEvidence:        []ports.AICatalogEvidence{{Reference: "item-1", EvidenceState: AIContextFresh}},
		BusinessPolicyEvidence: []ports.AIBusinessPolicyEvidence{{Reference: "policy-1", Category: "general", EvidenceState: AIContextFresh}},
	}
	result := (GroundedPolicyEngine{}).Evaluate(proposal, contextValue)
	if result.PolicyDecision != "allowed" || result.RequiresHuman || string(result.EvidenceReferences) != `["item-1"]` {
		t.Fatalf("verified catalog answer was changed unexpectedly: %#v", result)
	}
}

func TestGroundedPolicyEngineRequiresApprovalForStaleCatalogEvidence(t *testing.T) {
	proposal := ports.AIDecisionProposal{IntentBase: "product_inquiry", RequestedAction: AutoReplyActionAnswer, PolicyDecision: "allowed", ConfidenceBand: "high", SchemaVersion: 1, EvidenceReferences: []byte(`["item-2"]`)}
	contextValue := &ports.AIContext{
		CatalogEvidence:        []ports.AICatalogEvidence{{Reference: "item-2", EvidenceState: AIContextStale}},
		BusinessPolicyEvidence: []ports.AIBusinessPolicyEvidence{{Reference: "policy-1", Category: "general", EvidenceState: AIContextFresh}},
	}
	result := (GroundedPolicyEngine{}).Evaluate(proposal, contextValue)
	if result.PolicyDecision != "requires_approval" || !result.RequiresHuman || string(result.ReasonCodes) != `["verified_catalog_evidence_missing"]` {
		t.Fatalf("stale catalog evidence was not blocked: %#v", result)
	}
}

func TestGroundedPolicyEngineRequiresPolicyForCatalogWithoutGeneralPolicy(t *testing.T) {
	proposal := ports.AIDecisionProposal{IntentBase: "منتج", RequestedAction: AutoReplyActionAnswer, PolicyDecision: "allowed", ConfidenceBand: "high", SchemaVersion: 1, EvidenceReferences: []byte(`["item-1"]`)}
	contextValue := &ports.AIContext{
		CatalogEvidence: []ports.AICatalogEvidence{{Reference: "item-1", EvidenceState: AIContextFresh}},
	}
	result := (GroundedPolicyEngine{}).Evaluate(proposal, contextValue)
	if result.PolicyDecision != "requires_approval" || !result.RequiresHuman || string(result.ReasonCodes) != `["business_policy_evidence_missing"]` {
		t.Fatalf("catalog answer without merchant policy was not blocked: %#v", result)
	}
}

func TestGroundedPolicyEngineBlocksIncompleteCatalogData(t *testing.T) {
	proposal := ports.AIDecisionProposal{
		IntentBase:         "product_inquiry",
		RequestedAction:    AutoReplyActionAnswer,
		PolicyDecision:     "allowed",
		ConfidenceBand:     "high",
		SchemaVersion:      1,
		EvidenceReferences: []byte(`["item-1"]`),
		CatalogIncomplete:  true,
	}
	contextValue := &ports.AIContext{
		CatalogEvidence:        []ports.AICatalogEvidence{{Reference: "item-1", EvidenceState: AIContextFresh}},
		BusinessPolicyEvidence: []ports.AIBusinessPolicyEvidence{{Reference: "policy-1", Category: "general", EvidenceState: AIContextFresh}},
	}
	result := (GroundedPolicyEngine{}).Evaluate(proposal, contextValue)
	if result.PolicyDecision != "requires_approval" || !result.RequiresHuman || string(result.ReasonCodes) != `["catalog_retrieval_incomplete"]` {
		t.Fatalf("incomplete catalog data was not blocked: %#v", result)
	}
}

func TestGroundedPolicyEngineBlocksSafetyBudgetExhaustion(t *testing.T) {
	proposal := ports.AIDecisionProposal{
		IntentBase:            "product_inquiry",
		RequestedAction:       AutoReplyActionAnswer,
		PolicyDecision:        "allowed",
		ConfidenceBand:        "high",
		SchemaVersion:         1,
		EvidenceReferences:    []byte(`["item-1"]`),
		SafetyBudgetExhausted: true,
		CatalogIncomplete:     true,
	}
	contextValue := &ports.AIContext{
		CatalogEvidence:        []ports.AICatalogEvidence{{Reference: "item-1", EvidenceState: AIContextFresh}},
		BusinessPolicyEvidence: []ports.AIBusinessPolicyEvidence{{Reference: "policy-1", Category: "general", EvidenceState: AIContextFresh}},
	}
	result := (GroundedPolicyEngine{}).Evaluate(proposal, contextValue)
	if result.PolicyDecision != "requires_approval" || !result.RequiresHuman || string(result.ReasonCodes) != `["safety_budget_exhausted"]` {
		t.Fatalf("safety budget exhaustion was not blocked: %#v", result)
	}
}

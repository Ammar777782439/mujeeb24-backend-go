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

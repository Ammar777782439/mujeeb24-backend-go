package services

import (
	"encoding/json"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// GroundedPolicyEngine is deliberately small and deterministic. It does not
// authorize arbitrary actions; it only tightens a model proposal when verified
// knowledge or business-policy evidence is missing or stale.
type GroundedPolicyEngine struct{}

func (GroundedPolicyEngine) Evaluate(proposal ports.AIDecisionProposal, contextValue *ports.AIContext) ports.AIDecisionProposal {
	if contextValue == nil || proposal.RequestedAction != AutoReplyActionAnswer {
		return proposal
	}
	intent := strings.ToLower(strings.TrimSpace(proposal.IntentBase))
	category := policyCategoryForIntent(intent)
	if category == "" {
		return proposal
	}

	if category == "availability" || category == "pricing" {
		if len(contextValue.OfferEvidence) == 0 || hasStaleOffer(contextValue.OfferEvidence) {
			return requireApproval(proposal, "verified_offer_evidence_missing_or_stale", "offer availability or price requires fresh verified offer evidence")
		}
	}
	if category == "catalog" && len(contextValue.CatalogEvidence) == 0 {
		return requireApproval(proposal, "verified_catalog_evidence_missing", "catalog answer requires matched catalog evidence")
	}
	if !hasPolicyCategory(contextValue.BusinessPolicyEvidence, category) {
		return requireApproval(proposal, "business_policy_evidence_missing", "this business-policy answer requires a published merchant policy")
	}
	if !referencesAnyEvidence(proposal.EvidenceReferences, contextValue, category) {
		return requireApproval(proposal, "proposal_evidence_reference_missing", "factual proposal must reference Mujeeb evidence")
	}
	return proposal
}

func policyCategoryForIntent(intent string) string {
	switch {
	case containsAny(intent, "availability", "stock", "available", "مخزون", "متوفر", "توفر"):
		return "availability"
	case containsAny(intent, "price", "pricing", "cost", "سعر", "بكم", "تكلفة"):
		return "pricing"
	case containsAny(intent, "catalog", "product", "item", "منتج", "جهاز", "هاتف"):
		return "catalog"
	case containsAny(intent, "hour", "hours", "open", "opening", "دوام", "مفتوح", "الجمعة"):
		return "hours"
	case containsAny(intent, "return", "refund", "exchange", "استرجاع", "إرجاع", "استبدال"):
		return "returns"
	case containsAny(intent, "delivery", "shipping", "توصيل", "شحن", "منطقة"):
		return "delivery"
	default:
		return ""
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func hasStaleOffer(offers []ports.AIOfferEvidence) bool {
	for _, offer := range offers {
		if offer.EvidenceState == AIContextStale || strings.EqualFold(offer.AvailabilityState, "unknown") || strings.EqualFold(offer.AvailabilityState, "stale") {
			return true
		}
	}
	return false
}

func hasPolicyCategory(policies []ports.AIBusinessPolicyEvidence, category string) bool {
	for _, policy := range policies {
		if policy.EvidenceState == AIContextFresh && (policy.Category == category || category == "catalog" && policy.Category == "general") {
			return true
		}
	}
	return false
}

func referencesAnyEvidence(raw []byte, contextValue *ports.AIContext, category string) bool {
	if len(raw) == 0 {
		return false
	}
	var references []string
	if json.Unmarshal(raw, &references) != nil || len(references) == 0 {
		return false
	}
	allowed := make(map[string]struct{})
	if category == "availability" || category == "pricing" {
		for _, offer := range contextValue.OfferEvidence {
			allowed[offer.Reference] = struct{}{}
		}
	}
	if category == "catalog" {
		for _, item := range contextValue.CatalogEvidence {
			allowed[item.Reference] = struct{}{}
		}
	}
	if category == "hours" || category == "returns" || category == "delivery" {
		for _, item := range contextValue.KnowledgeEvidence {
			allowed[item.Reference] = struct{}{}
		}
		for _, policy := range contextValue.BusinessPolicyEvidence {
			if policy.Category == category {
				allowed[policy.Reference] = struct{}{}
			}
		}
	}
	for _, reference := range references {
		if _, ok := allowed[strings.TrimSpace(reference)]; ok {
			return true
		}
	}
	return false
}

func requireApproval(proposal ports.AIDecisionProposal, reason, missing string) ports.AIDecisionProposal {
	proposal.PolicyDecision = "requires_approval"
	proposal.RequiresHuman = true
	proposal.ReasonCodes = appendJSONString(proposal.ReasonCodes, reason)
	proposal.MissingInformation = appendJSONString(proposal.MissingInformation, missing)
	return proposal
}

func appendJSONString(raw []byte, value string) []byte {
	var values []string
	if json.Unmarshal(raw, &values) != nil || values == nil {
		values = []string{}
	}
	values = append(values, value)
	encoded, err := json.Marshal(values)
	if err != nil {
		return raw
	}
	return encoded
}

var _ ports.AIPolicyEvaluator = GroundedPolicyEngine{}

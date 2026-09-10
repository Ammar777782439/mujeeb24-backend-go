package services

import (
	"encoding/json"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

const maxPreviousReferences = 5

func validateStateProposal(proposal ports.AIDecisionProposal, ctx *ports.AIContext) (*ports.ConversationStateRecord, bool, error) {
	if proposal.StateProposal == nil {
		return nil, false, nil
	}
	sp := proposal.StateProposal
	kind := strings.ToUpper(strings.TrimSpace(sp.Kind))
	if kind != "RESOLVED" && kind != "AMBIGUOUS" && kind != "NO_REFERENCE" {
		return nil, false, nil
	}
	if kind == "NO_REFERENCE" {
		return nil, false, nil
	}
	if kind == "AMBIGUOUS" {
		// Ambiguity is a safety signal, not a failure. Caller should clarify.
		return nil, true, nil
	}
	// RESOLVED: focus must be present and in candidate set.
	if sp.Focus == nil || strings.TrimSpace(sp.Focus.ID) == "" || strings.TrimSpace(sp.Focus.Type) == "" {
		return nil, true, nil
	}
	if ctx == nil {
		return nil, true, nil
	}
	if !focusInCandidates(sp.Focus, ctx) {
		return nil, true, nil
	}
	// Comparison, if present, must have all IDs in candidates.
	if sp.Comparison != nil && len(sp.Comparison.IDs) > 0 {
		for _, id := range sp.Comparison.IDs {
			if !idInCandidates(id, ctx) {
				return nil, true, nil
			}
		}
	}
	return nil, false, nil
}

func focusInCandidates(focus *ports.ConversationFocus, ctx *ports.AIContext) bool {
	if focus == nil || ctx == nil {
		return false
	}
	id := strings.TrimSpace(focus.ID)
	if id == "" {
		return false
	}
	switch normalizeFocusType(focus.Type) {
	case "offer":
		for _, offer := range ctx.OfferEvidence {
			if offer.Reference == id {
				return true
			}
		}
		return false
	case "variant":
		for _, variant := range ctx.VariantEvidence {
			if variant.Reference == id {
				return true
			}
		}
		return false
	case "item":
		for _, item := range ctx.CatalogEvidence {
			if item.Reference == id {
				return true
			}
		}
		return false
	case "catalog":
		for _, item := range ctx.CatalogEvidence {
			if item.CatalogReference == id {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func idInCandidates(id string, ctx *ports.AIContext) bool {
	id = strings.TrimSpace(id)
	if id == "" || ctx == nil {
		return false
	}
	for _, item := range ctx.CatalogEvidence {
		if item.Reference == id {
			return true
		}
		// The parent catalog itself is a valid citation for general answers
		// (e.g. "what packages do you have?").
		if item.CatalogReference == id {
			return true
		}
	}
	for _, offer := range ctx.OfferEvidence {
		if offer.Reference == id {
			return true
		}
	}
	for _, variant := range ctx.VariantEvidence {
		if variant.Reference == id {
			return true
		}
	}
	for _, doc := range ctx.KnowledgeEvidence {
		if doc.Reference == id {
			return true
		}
	}
	for _, policy := range ctx.BusinessPolicyEvidence {
		if policy.Reference == id {
			return true
		}
	}
	return false
}

func buildValidatedState(current *ports.ConversationStateRecord, businessID, conversationID string, proposal ports.AIDecisionProposal) *ports.ConversationStateRecord {
	base := ports.ConversationStateRecord{
		BusinessID:     businessID,
		ConversationID: conversationID,
	}
	if current != nil {
		base = *current
		base.BusinessID = businessID
		base.ConversationID = conversationID
	}
	if proposal.StateProposal == nil || strings.ToUpper(strings.TrimSpace(proposal.StateProposal.Kind)) != "RESOLVED" {
		// An explicit NO_REFERENCE (general question / sudden topic change)
		// resets a stale comparison so the next turn is not locked into it.
		// Focus and history are preserved; only the comparison is dropped.
		// AMBIGUOUS and missing proposals leave state untouched.
		if proposal.StateProposal != nil &&
			strings.ToUpper(strings.TrimSpace(proposal.StateProposal.Kind)) == "NO_REFERENCE" &&
			current != nil && current.Comparison != nil {
			base := *current
			base.BusinessID = businessID
			base.ConversationID = conversationID
			base.Comparison = nil
			if base.Previous == nil {
				base.Previous = []ports.ConversationFocus{}
			}
			if base.Preferences == nil {
				base.Preferences = []ports.StatePreference{}
			}
			if base.Constraints == nil {
				base.Constraints = []ports.StateConstraint{}
			}
			if base.Pending == nil {
				base.Pending = []ports.StatePending{}
			}
			return &base
		}
		return nil
	}
	focus := proposal.StateProposal.Focus
	if focus == nil {
		return nil
	}
	// Preserve previous: old focus moves to previous (budget: maxPreviousReferences).
	if base.Focus != nil && base.Focus.ID != focus.ID {
		prev := append([]ports.ConversationFocus{*base.Focus}, base.Previous...)
		if len(prev) > maxPreviousReferences {
			prev = prev[:maxPreviousReferences]
		}
		base.Previous = prev
	}
	if base.Previous == nil {
		base.Previous = []ports.ConversationFocus{}
	}
	newFocus := *focus
	base.Focus = &newFocus
	if proposal.StateProposal.Comparison != nil && len(proposal.StateProposal.Comparison.IDs) >= 2 {
		base.Comparison = proposal.StateProposal.Comparison
	} else if proposal.StateProposal.Comparison == nil {
		// Keep existing comparison only if new focus is part of it; otherwise clear.
		// If topic switched away, clear stale comparison.
		if base.Comparison != nil {
			keep := false
			for _, id := range base.Comparison.IDs {
				if id == focus.ID {
					keep = true
					break
				}
			}
			if !keep {
				base.Comparison = nil
			}
		}
	}
	if base.Preferences == nil {
		base.Preferences = []ports.StatePreference{}
	}
	if base.Constraints == nil {
		base.Constraints = []ports.StateConstraint{}
	}
	if base.Pending == nil {
		base.Pending = []ports.StatePending{}
	}
	return &base
}

func validateEvidenceIdentity(proposal ports.AIDecisionProposal, ctx *ports.AIContext) bool {
	if ctx == nil {
		return true
	}
	// Use the reference proposed for THIS turn, not the stored focus from a
	// previous turn. A general question (NO_REFERENCE) or a legitimate topic
	// switch (new RESOLVED focus) must not be judged against the old focus;
	// otherwise every general question after a focused turn is wrongly flagged
	// as entity mixing. Only fall back to the stored focus when the proposal
	// carries no new reference signal (legacy/clients without state proposals).
	if proposal.StateProposal != nil {
		kind := strings.ToUpper(strings.TrimSpace(proposal.StateProposal.Kind))
		switch kind {
		case "NO_REFERENCE", "AMBIGUOUS":
			// No specific entity claimed: any in-context evidence is allowed.
			// Invented references are still rejected by refsInContext.
			return refsInContext(proposal.EvidenceReferences, ctx)
		case "RESOLVED":
			if proposal.StateProposal.Focus != nil && focusInCandidates(proposal.StateProposal.Focus, ctx) {
				return refsMatchFocus(proposal.EvidenceReferences, proposal.StateProposal.Focus, ctx)
			}
			// Invalid proposed focus: validateStateProposal already forces
			// clarification; don't add a second mismatch signal.
			return true
		}
	}
	if ctx.ConversationState == nil || ctx.ConversationState.Focus == nil {
		return true
	}
	return refsMatchFocus(proposal.EvidenceReferences, ctx.ConversationState.Focus, ctx)
}

// refsInContext reports whether every referenced ID exists in the current
// context evidence. It blocks hallucinated references without constraining
// which valid entity a general answer may mention.
func refsInContext(raw []byte, ctx *ports.AIContext) bool {
	if len(raw) == 0 {
		return true
	}
	var refs []string
	if json.Unmarshal(raw, &refs) != nil {
		return false
	}
	for _, ref := range refs {
		if !idInCandidates(strings.TrimSpace(ref), ctx) {
			return false
		}
	}
	return true
}

func refsMatchFocus(raw []byte, focus *ports.ConversationFocus, ctx *ports.AIContext) bool {
	switch normalizeFocusType(focus.Type) {
	case "item":
		if len(raw) == 0 {
			return true
		}
		var refs []string
		if json.Unmarshal(raw, &refs) != nil {
			return false
		}
		if len(refs) == 0 {
			return true
		}
		allowed := map[string]struct{}{focus.ID: {}}
		for _, offer := range ctx.OfferEvidence {
			if offer.CatalogItemReference == focus.ID {
				allowed[offer.Reference] = struct{}{}
			}
		}
		for _, ref := range refs {
			if _, ok := allowed[strings.TrimSpace(ref)]; ok {
				return true
			}
		}
		for _, ref := range refs {
			for _, offer := range ctx.OfferEvidence {
				if offer.Reference == strings.TrimSpace(ref) {
					return false
				}
			}
			for _, item := range ctx.CatalogEvidence {
				if item.Reference == strings.TrimSpace(ref) && ref != focus.ID {
					return false
				}
			}
		}
		return true
	case "offer":
		// handled below
	default:
		return true
	}
	// If focus is a specific offer, factual answer must reference that offer or its item.
	// Prohibits Basic focus answering with Pro evidence.
	if len(raw) == 0 {
		return true
	}
	var refs []string
	if json.Unmarshal(raw, &refs) != nil {
		return false
	}
	if len(refs) == 0 {
		return true
	}
	allowed := map[string]struct{}{
		focus.ID: {},
	}
	if focus.ItemID != nil {
		allowed[*focus.ItemID] = struct{}{}
	}
	// Also allow the item ID derived from offer evidence.
	for _, offer := range ctx.OfferEvidence {
		if offer.Reference == focus.ID {
			allowed[offer.CatalogItemReference] = struct{}{}
			break
		}
	}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if _, ok := allowed[ref]; ok {
			return true
		}
	}
	// If none of the references match the focused offer/item, check if they match
	// other offers in context: that indicates mixing.
	for _, ref := range refs {
		for _, offer := range ctx.OfferEvidence {
			if offer.Reference == ref && ref != focus.ID {
				return false
			}
		}
	}
	return true
}

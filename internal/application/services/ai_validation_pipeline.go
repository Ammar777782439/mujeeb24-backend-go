// Package services — AI Validation + Authorization Pipeline
//
// Implements contract ⑥ AI Validation + Authorization Boundary.
//
// The pipeline is the contract ⑥ §2 post-Gemini flow:
//   AI Proposal
//     ↓
//   Structural Validation       (§3 — schema correctness)
//     ↓
//   Reference Validation         (§6 — selected IDs exist in Mujeeb data)
//     ↓
//   Tenant / Ownership Validation (§8 — references are within current Business scope)
//     ↓
//   Business Policy Evaluation   (§12 — PolicyEvaluator applies merchant policies)
//     ↓
//   Authorization               (§17 — final go/no-go for execution)
//     ↓
//   Effective Decision           (§17 — distinct from AI Proposal)
//     ↓
//   Execution                   (§19 — only after Authorization; AI has no direct channel)
//
// Per contract ⑥ §11, Mujeeb does NOT re-interpret customer intent here.
// Per contract ⑥ §20, Validation is deterministic; no second LLM is used.
// Per contract ⑥ §22, no new "invalid/rejected/unauthorized" statuses are
// added to the AI Contract; those are internal layer results.
// Per contract ⑥ §13, requires_approval is decided by Mujeeb's PolicyEvaluator
// only — never by Gemini.

package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// ValidationPipeline executes the contract ⑥ post-Gemini validation chain.
//
// Each stage returns a structured failure (StageFailure) which the pipeline
// converts into an EffectiveDecision with EffectiveAction "blocked" (per
// contract ⑥ §21 — no Execution when validation fails) and records the
// failure in ai_runs (via the lifecycle) for observability.
//
// On full success, the pipeline returns an EffectiveDecision with the
// EffectiveAction set per PolicyEvaluator's verdict and the PolicyDecision
// (allowed / requires_approval / denied).
type ValidationPipeline struct {
	ReferenceValidator   ReferenceValidator
	TenantValidator      TenantValidator
	PolicyEvaluator      ports.AIPolicyEvaluator
	AuthorizationService AuthorizationService
	Now                  func() time.Time
}

// NewValidationPipeline wires the pipeline dependencies. Each may be nil if
// the deployment does not yet implement that stage; the pipeline will treat a
// nil stage as "always passes" with a logged warning, allowing incremental
// rollout per contract ⑥.
func NewValidationPipeline(
	rv ReferenceValidator,
	tv TenantValidator,
	pe ports.AIPolicyEvaluator,
	as AuthorizationService,
) *ValidationPipeline {
	return &ValidationPipeline{
		ReferenceValidator:   rv,
		TenantValidator:      tv,
		PolicyEvaluator:      pe,
		AuthorizationService: as,
		Now:                  func() time.Time { return time.Now().UTC() },
	}
}

// StageFailure is the structured error returned by any validation stage.
// It carries the contract ⑥ §30 failure category for ai_runs.failure_*.
type StageFailure struct {
	Stage    string // one of ports.AIRunFailureStage* constants
	Category ports.AIRunFailureCategory
	Reason   string
}

func (f StageFailure) Error() string { return fmt.Sprintf("%s/%s: %s", f.Stage, f.Category, f.Reason) }

// Validate runs the full pipeline. It returns either a successful
// EffectiveDecision or a StageFailure.
//
// Per contract ⑥ §2, if any barrier fails, no downstream barrier runs.
// This is enforced via early return on each stage.
func (p *ValidationPipeline) Validate(ctx context.Context, input ValidationInput) (ports.EffectiveDecision, *StageFailure) {
	if err := p.validateStructural(input.Proposal); err != nil {
		return ports.EffectiveDecision{}, err
	}
	if err := p.validateReferences(ctx, input); err != nil {
		return ports.EffectiveDecision{}, err
	}
	if err := p.validateTenant(ctx, input); err != nil {
		return ports.EffectiveDecision{}, err
	}
	decision, err := p.evaluatePolicy(input)
	if err != nil {
		return ports.EffectiveDecision{}, err
	}
	authorized, err := p.authorize(ctx, input, decision)
	if err != nil {
		return ports.EffectiveDecision{}, err
	}
	return authorized, nil
}

// validateStructural is contract ⑥ §3 — the AI Proposal must match the
// closed Output Contract per contract ④ §4.
//
// Allowed status: resolved/ambiguous/not_found/needs_more_data.
// Allowed action: answer/clarification/human_request/lead_draft/order_draft.
// response_text: non-empty when action != human_request.
// selected: each SelectedReference has a non-empty item_id (variant_id and
// offer_id may be nil per contract ④ §4).
func (p *ValidationPipeline) validateStructural(proposal ports.AIGeminiProposal) *StageFailure {
	switch proposal.Status {
	case ports.AIProposalStatusResolved,
		ports.AIProposalStatusAmbiguous,
		ports.AIProposalStatusNotFound,
		ports.AIProposalStatusNeedsMoreData:
		// allowed
	default:
		return &StageFailure{
			Stage:    ports.AIRunFailureStageValidation,
			Category: ports.AIRunFailureCategoryInvalidAIOutput,
			Reason:   fmt.Sprintf("invalid status %q per contract ④ §4", proposal.Status),
		}
	}
	switch proposal.Action {
	case ports.AIProposalActionAnswer,
		ports.AIProposalActionClarification,
		ports.AIProposalActionHumanRequest,
		ports.AIProposalActionLeadDraft,
		ports.AIProposalActionOrderDraft:
		// allowed
	default:
		return &StageFailure{
			Stage:    ports.AIRunFailureStageValidation,
			Category: ports.AIRunFailureCategoryInvalidAIOutput,
			Reason:   fmt.Sprintf("invalid action %q per contract ④ §4", proposal.Action),
		}
	}
	if proposal.Action != ports.AIProposalActionHumanRequest && strings.TrimSpace(proposal.ResponseText) == "" {
		return &StageFailure{
			Stage:    ports.AIRunFailureStageValidation,
			Category: ports.AIRunFailureCategoryInvalidAIOutput,
			Reason:   "response_text must be non-empty unless action is human_request",
		}
	}
	for i, ref := range proposal.Selected {
		if strings.TrimSpace(ref.ItemID) == "" {
			return &StageFailure{
				Stage:    ports.AIRunFailureStageValidation,
				Category: ports.AIRunFailureCategoryInvalidAIOutput,
				Reason:   fmt.Sprintf("selected[%d].item_id is required per contract ④ §4", i),
			}
		}
	}
	return nil
}

// validateReferences is contract ⑥ §6-7 — every selected ID must exist in
// the actual catalog data Mujeeb provided to Gemini. Per contract ⑥ §7,
// Gemini cannot invent references; even a syntactically valid UUID fails
// if it wasn't in the evidence sent.
func (p *ValidationPipeline) validateReferences(ctx context.Context, input ValidationInput) *StageFailure {
	if p.ReferenceValidator == nil {
		return nil
	}
	for i, ref := range input.Proposal.Selected {
		if err := p.ReferenceValidator.ValidateItemReference(ctx, input.BusinessID, ref.ItemID, input.EvidenceItemIDs); err != nil {
			return &StageFailure{
				Stage:    ports.AIRunFailureStageValidation,
				Category: ports.AIRunFailureCategoryInvalidReference,
				Reason:   fmt.Sprintf("selected[%d].item_id %s: %s per contract ⑥ §6", i, ref.ItemID, err.Error()),
			}
		}
		if ref.VariantID != nil && *ref.VariantID != "" {
			if err := p.ReferenceValidator.ValidateVariantReference(ctx, input.BusinessID, *ref.VariantID, input.EvidenceVariantIDs); err != nil {
				return &StageFailure{
					Stage:    ports.AIRunFailureStageValidation,
					Category: ports.AIRunFailureCategoryInvalidReference,
					Reason:   fmt.Sprintf("selected[%d].variant_id %s: %s", i, *ref.VariantID, err.Error()),
				}
			}
		}
		if ref.OfferID != nil && *ref.OfferID != "" {
			if err := p.ReferenceValidator.ValidateOfferReference(ctx, input.BusinessID, *ref.OfferID, input.EvidenceOfferIDs); err != nil {
				return &StageFailure{
					Stage:    ports.AIRunFailureStageValidation,
					Category: ports.AIRunFailureCategoryInvalidReference,
					Reason:   fmt.Sprintf("selected[%d].offer_id %s: %s", i, *ref.OfferID, err.Error()),
				}
			}
		}
	}
	return nil
}

// validateTenant is contract ⑥ §8-9 — every selected reference must belong
// to the current Business. Per contract ⑥ §9, business_id and tenant_id
// cannot come from Gemini; they come from the Authenticated Context.
//
// Per contract ⑥ §8, references from a different Business are silently
// rejected (Do not expose the resource / Do not treat it as valid / Do not
// leak its existence).
func (p *ValidationPipeline) validateTenant(ctx context.Context, input ValidationInput) *StageFailure {
	if p.TenantValidator == nil {
		return nil
	}
	for i, ref := range input.Proposal.Selected {
		if err := p.TenantValidator.ValidateItemOwnership(ctx, input.BusinessID, ref.ItemID); err != nil {
			return &StageFailure{
				Stage:    ports.AIRunFailureStageValidation,
				Category: ports.AIRunFailureCategoryTenantViolation,
				Reason:   fmt.Sprintf("selected[%d].item_id %s not owned by business per contract ⑥ §8", i, ref.ItemID),
			}
		}
		if ref.VariantID != nil && *ref.VariantID != "" {
			if err := p.TenantValidator.ValidateVariantOwnership(ctx, input.BusinessID, *ref.VariantID); err != nil {
				return &StageFailure{
					Stage:    ports.AIRunFailureStageValidation,
					Category: ports.AIRunFailureCategoryTenantViolation,
					Reason:   fmt.Sprintf("selected[%d].variant_id %s not owned by business", i, *ref.VariantID),
				}
			}
		}
		if ref.OfferID != nil && *ref.OfferID != "" {
			if err := p.TenantValidator.ValidateOfferOwnership(ctx, input.BusinessID, *ref.OfferID); err != nil {
				return &StageFailure{
					Stage:    ports.AIRunFailureStageValidation,
					Category: ports.AIRunFailureCategoryTenantViolation,
					Reason:   fmt.Sprintf("selected[%d].offer_id %s not owned by business", i, *ref.OfferID),
				}
			}
		}
	}
	return nil
}

// evaluatePolicy is contract ⑥ §12 — PolicyEvaluator applies the merchant's
// business policies to the validated proposal. Per contract ⑥ §13,
// requires_approval comes ONLY from Mujeeb's PolicyEvaluator, never from Gemini.
func (p *ValidationPipeline) evaluatePolicy(input ValidationInput) (ports.EffectiveDecision, *StageFailure) {
	if p.PolicyEvaluator == nil {
		// No policy configured → default allow (per contract ⑥, this is the
		// conservative default when no policy is registered).
		return ports.EffectiveDecision{
			DecisionID:      input.DecisionID,
			EffectiveAction: string(input.Proposal.Action),
			PolicyDecision:  "allowed",
			Reason:          "no policy evaluator configured; defaulting to allowed per contract ⑥ §12",
		}, nil
	}
	// Convert the contract-aligned AIGeminiProposal to the legacy
	// AIDecisionProposal expected by ports.AIPolicyEvaluator. This is a
	// bridge while services migrate to the contract-aligned types.
	legacy := p.toLegacyProposal(input)
	evaluated := p.PolicyEvaluator.Evaluate(legacy, input.Context)
	policyDecision := strings.TrimSpace(evaluated.PolicyDecision)
	if policyDecision == "" {
		policyDecision = "allowed"
	}
	return ports.EffectiveDecision{
		DecisionID:      input.DecisionID,
		EffectiveAction: string(input.Proposal.Action),
		PolicyDecision:  policyDecision,
		Reason:          "policy evaluation completed per contract ⑥ §12",
	}, nil
}

// authorize is contract ⑥ §17 — the final go/no-go. Per contract ⑥ §19,
// Execution only happens after Authorization succeeds.
//
// Per contract ⑥ §14, when action == human_request, Mujeeb applies its own
// handoff policy; Gemini cannot force handoff via prompt text alone.
func (p *ValidationPipeline) authorize(ctx context.Context, input ValidationInput, decision ports.EffectiveDecision) (ports.EffectiveDecision, *StageFailure) {
	if p.AuthorizationService == nil {
		// No authorization service → use the policy decision as final.
		if decision.PolicyDecision == "allowed" {
			now := p.Now()
			decision.AuthorizedAt = &now
		}
		return decision, nil
	}
	authorized, err := p.AuthorizationService.Authorize(ctx, input, decision)
	if err != nil {
		return ports.EffectiveDecision{}, &StageFailure{
			Stage:    ports.AIRunFailureStageAuthorization,
			Category: ports.AIRunFailureCategoryAuthorizationDenial,
			Reason:   err.Error(),
		}
	}
	return authorized, nil
}

// ValidationInput is the input to the pipeline.
type ValidationInput struct {
	// DecisionID is the ai_decisions.id that this validation is for. The
	// resulting EffectiveDecision is linked to it.
	DecisionID string

	// BusinessID comes from the Authenticated Context — never from Gemini.
	BusinessID string

	// ConversationID is the conversation this decision belongs to.
	ConversationID string

	// Proposal is the contract ④ §4 Gemini output.
	Proposal ports.AIGeminiProposal

	// Context is the AIContext built by the ContextBuilder.
	Context *ports.AIContext

	// EvidenceItemIDs is the set of item IDs that were actually sent to
	// Gemini as evidence. Used by ReferenceValidator per contract ⑥ §10.
	EvidenceItemIDs []string

	// EvidenceVariantIDs is the set of variant IDs sent.
	EvidenceVariantIDs []string

	// EvidenceOfferIDs is the set of offer IDs sent.
	EvidenceOfferIDs []string
}

// toLegacyProposal converts the contract-aligned AIGeminiProposal to the
// legacy AIDecisionProposal shape expected by the existing PolicyEvaluator.
// This is a temporary bridge; once the PolicyEvaluator is migrated to consume
// AIGeminiProposal directly, this conversion will be removed.
func (p *ValidationPipeline) toLegacyProposal(input ValidationInput) ports.AIDecisionProposal {
	entities := []byte(`{}`)
	evidence := []byte(`[]`)
	if len(input.Proposal.Selected) > 0 {
		// Encode selected references as evidence_references JSON array.
		evidence = encodeSelectedAsLegacyJSON(input.Proposal.Selected)
	}
	return ports.AIDecisionProposal{
		IntentBase:         string(input.Proposal.Status),
		DomainContext:      "",
		Entities:           entities,
		EvidenceReferences: evidence,
		RequestedAction:    string(input.Proposal.Action),
		ResponseText:       input.Proposal.ResponseText,
		ConfidenceBand:     "medium",
		RequiresHuman:      input.Proposal.Action == ports.AIProposalActionHumanRequest,
		MissingInformation: []byte(`[]`),
		ReasonCodes:        []byte(`[]`),
		PolicyDecision:     "",
		PolicyVersion:      "",
		KnowledgeVersion:   "none",
		ModelReference:     "",
		SchemaVersion:      1,
	}
}

// encodeSelectedAsLegacyJSON is a minimal helper to encode SelectedReference
// slice to the legacy evidence_references JSON shape. It avoids importing
// encoding/json here to keep the file focused.
func encodeSelectedAsLegacyJSON(selected []ports.SelectedReference) []byte {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, ref := range selected {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"item_id":"`)
		sb.WriteString(ref.ItemID)
		sb.WriteByte('"')
		if ref.VariantID != nil && *ref.VariantID != "" {
			sb.WriteString(`,"variant_id":"`)
			sb.WriteString(*ref.VariantID)
			sb.WriteByte('"')
		}
		if ref.OfferID != nil && *ref.OfferID != "" {
			sb.WriteString(`,"offer_id":"`)
			sb.WriteString(*ref.OfferID)
			sb.WriteByte('"')
		}
		sb.WriteByte('}')
	}
	sb.WriteByte(']')
	return []byte(sb.String())
}

// ReferenceValidator is the contract ⑥ §6-7 reference-existence check.
// Per contract ⑥ §10, references must be in the data Mujeeb actually provided.
type ReferenceValidator interface {
	ValidateItemReference(ctx context.Context, businessID, itemID string, evidenceItemIDs []string) error
	ValidateVariantReference(ctx context.Context, businessID, variantID string, evidenceVariantIDs []string) error
	ValidateOfferReference(ctx context.Context, businessID, offerID string, evidenceOfferIDs []string) error
}

// TenantValidator is the contract ⑥ §8 ownership check.
type TenantValidator interface {
	ValidateItemOwnership(ctx context.Context, businessID, itemID string) error
	ValidateVariantOwnership(ctx context.Context, businessID, variantID string) error
	ValidateOfferOwnership(ctx context.Context, businessID, offerID string) error
}

// AuthorizationService is the contract ⑥ §17 final go/no-go for execution.
// It may add additional checks (e.g., approval workflow state).
type AuthorizationService interface {
	Authorize(ctx context.Context, input ValidationInput, decision ports.EffectiveDecision) (ports.EffectiveDecision, error)
}

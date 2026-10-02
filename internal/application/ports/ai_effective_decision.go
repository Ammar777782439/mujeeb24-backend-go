// Package ports contains the application contracts used by Mujeeb AI capabilities.

package ports

// EffectiveDecision is the post-validation/post-authorization application result.
// It is intentionally separate from any provider proposal contract.

import "time"

// ════════════════════════════════════════════════════════════════════════════
// Contract ⑥ §17 — Effective Decision (post-validation, post-authorization)
// ════════════════════════════════════════════════════════════════════════════

// EffectiveDecision is the contract ⑥ §17 outcome after Structural → Reference
// → Tenant → Ownership → Policy → Authorization. It is distinct from AIGeminiProposal.
//
// Per contract ⑥ §22, no new "invalid/rejected/unauthorized" statuses are added
// to the AI Contract; those are internal layer resultss. The EffectiveDecision
// carries the authoritative action and the policy decision Mujeeb reached.
type EffectiveDecision struct {
	// DecisionID is the ai_decisions.id this Effective Decision is attached to.
	DecisionID string

	// EffectiveAction may differ from the AI proposal's action when policy or
	// authorization overrode it. For example, if Gemini proposed "answer" but
	// policy requires approval, EffectiveAction becomes "blocked" until
	// approval arrives, then transitions to the original action.
	//
	// Allowed values: same as AIProposalAction plus "no_action" and "blocked".
	EffectiveAction string

	// PolicyDecision is the PolicyEvaluator's verdict per contract ⑥ §12-13.
	//   allowed           — proceed without human approval
	//   requires_approval — handoff to human queue; do not execute yet
	//   denied            — do not execute at all
	PolicyDecision string

	// Reason captures the policy key or authorization reason that produced
	// this Effective Decision. Useful for audit and human review.
	Reason string

	// AuthorizedAt is when the Authorization step completed successfully.
	// Empty if PolicyDecision == "denied".
	AuthorizedAt *time.Time

	// ExecutedAt is when the Executor completed the authorized action.
	// Per contract ⑥ §14, Execution Result is tracked separately in
	// ai_runs.status (EXECUTING → COMPLETED/FAILED), not here.
	ExecutedAt *time.Time
}


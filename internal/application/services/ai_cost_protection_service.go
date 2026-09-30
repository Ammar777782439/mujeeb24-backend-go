package services

import (
	"context"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// AICostProtectionService implements ports.AICostProtectionChecker.
//
// Per AIUsageTokenTelemetry.md §19-21:
// - When budget_status == EXCEEDED, no new Auto AI Execution starts.
// - Cost Protection is NOT a round limit — it's a budget limit.
// - Human replies, dashboard, customer data, leads, orders, and channel
//   reception continue to work normally.
//
// P0-2 + P0-3 fix: the checker now also enforces:
//   - Platform-level AI Runtime Kill Switch (Contract §81): when the
//     Platform Admin has set AdminState=DISABLED via platformAIDisable,
//     ALL Auto AI Execution is blocked. Human replies, customers, leads,
//     orders, channel reception continue to work normally.
//   - Merchant AI Reply Entitlement (Contract §33): when the active
//     subscription's AIRepliesRemaining == 0, no new Auto AI Execution.
//     This is the commercial entitlement check — distinct from the
//     cost budget check (which is the internal-budget guard).
//
// Order of checks (fail-fast):
//  1. Platform kill switch (kills ALL AI for ALL businesses)
//  2. Active subscription exists (else allow — entitlement not yet checked)
//  3. AI Reply entitlement (block when remaining = 0)
//  4. Cost budget (block when EXCEEDED)
//
// Each check has its own reason code so the webhook caller can log a
// precise reason and the platform audit trail distinguishes them.
type AICostProtectionService struct {
	Subscriptions      ports.SubscriptionRepository
	AIUsage            ports.AIUsageRepository
	PlatformOperations ports.PlatformOperationsPort
}

// IsAIExecutionAllowed returns true if all of:
//   - AI Runtime is ENABLED at the platform level (Contract §81)
//   - The business has an active subscription with AIRepliesRemaining > 0
//     (Contract §33 — merchant AI Reply entitlement)
//   - The subscription's budget_status != EXCEEDED (Contract §19)
//
// Returns (false, reason) when blocked. The reason is a machine-readable
// code (e.g., "ai_runtime_disabled", "ai_replies_exhausted",
// "ai_cost_budget_exceeded") so the caller can log it precisely and
// the platform audit trail can distinguish the cause.
//
// When any of the underlying repositories is nil or returns an error,
// the checker fails OPEN (returns true). This preserves operability
// during outages — but the platform admin should ensure the
// dependencies are wired so the checks actually run.
func (s *AICostProtectionService) IsAIExecutionAllowed(ctx context.Context, businessID string) (bool, string) {
	if s == nil {
		return true, ""
	}

	// Check 1: Platform-level AI Runtime Kill Switch (P0-2).
	// Per Contract §81: when AdminState=DISABLED, ALL Auto AI Execution
	// is blocked. This is the master switch the platform admin toggles
	// via platformAIDisable / platformAIEnable.
	if s.PlatformOperations != nil {
		state, err := s.PlatformOperations.GetRuntimeState(ctx)
		if err == nil && state.AdminState == ports.ProviderAdminDisabled {
			return false, "ai_runtime_disabled"
		}
		// On error: fail-open (operability). The platform admin can
		// monitor for the missing state via the AI Overview endpoint.
	}

	if s.Subscriptions == nil || s.AIUsage == nil {
		return true, "" // not wired → allow (fail-open for operability)
	}

	// Find the business's active subscription.
	page, err := s.Subscriptions.List(ctx, ports.SubscriptionListFilter{
		BusinessID: businessID,
		Status:     "ACTIVE",
		Limit:      1,
	})
	if err != nil || len(page.Items) == 0 {
		return true, "" // no active subscription → allow (entitlement check handles this separately)
	}
	sub := page.Items[0]
	agg, err := s.AIUsage.GetSubscriptionAIUsage(ctx, sub.ID)
	if err != nil {
		return true, "" // can't check → allow (fail-open)
	}

	// Check 2: Merchant AI Reply Entitlement (P0-3).
	// Per Contract §33: AI Replies is the commercial metric. When the
	// subscription's AIRepliesRemaining == 0, no new Auto AI Execution
	// starts. This is distinct from the cost budget — a subscription can
	// have budget remaining but replies exhausted (e.g., a cheap model
	// hit the reply cap without burning the budget).
	if agg.AIReplyLimit > 0 && agg.AIRepliesRemaining <= 0 {
		return false, "ai_replies_exhausted"
	}

	// Check 3: Cost budget (original check).
	if strings.EqualFold(agg.BudgetStatus, "EXCEEDED") {
		return false, "ai_cost_budget_exceeded"
	}
	return true, ""
}

var _ AICostProtectionChecker = (*AICostProtectionService)(nil)

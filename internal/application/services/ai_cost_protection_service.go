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
// The checker queries the business's active subscription's AI usage aggregate
// and returns allowed=false when budget_status == "EXCEEDED".
type AICostProtectionService struct {
	Subscriptions ports.SubscriptionRepository
	AIUsage       ports.AIUsageRepository
}

// IsAIExecutionAllowed returns true if the business's active subscription
// has budget_status != EXCEEDED. Returns false + reason when blocked.
//
// If the business has no active subscription, AI is allowed (the entitlement
// check would block it separately — cost protection is about the internal
// budget, not the entitlement).
func (s *AICostProtectionService) IsAIExecutionAllowed(ctx context.Context, businessID string) (bool, string) {
	if s == nil || s.Subscriptions == nil || s.AIUsage == nil {
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
	if strings.EqualFold(agg.BudgetStatus, "EXCEEDED") {
		return false, "ai_cost_budget_exceeded"
	}
	return true, ""
}

var _ AICostProtectionChecker = (*AICostProtectionService)(nil)

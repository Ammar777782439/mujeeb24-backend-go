package services

import (
	"context"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// BusinessAIGuard reads the current persisted merchant policy. No local cache:
// a saved opt-out takes effect before the next provider call.
type BusinessAIGuard struct {
	Policies ports.BusinessManagementRepository
}

func (g *BusinessAIGuard) Check(ctx context.Context, businessID string) error {
	if g == nil || g.Policies == nil || strings.TrimSpace(businessID) == "" {
		return ports.ErrBusinessAIPolicyUnavailable
	}
	policy, err := g.Policies.GetRuntimePolicy(ctx, businessID)
	if err != nil {
		return ports.ErrBusinessAIPolicyUnavailable
	}
	switch policy.AIMode {
	case "assist", "approval", "restricted_auto":
		return nil // Permission to GENERATE only; PolicyEvaluator still authorizes actions.
	case "disabled":
		return ports.ErrBusinessAIDisabled
	default:
		return ports.ErrBusinessAIPolicyUnavailable
	}
}

var _ ports.BusinessAIExecutionGuard = (*BusinessAIGuard)(nil)

package services

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// FailoverCustomerSalesDecisionPort switches only on transient provider errors.
// Never rerun a decision on malformed output, authorization failures or canceled
// requests. The decision is not executed here: Mujeeb validates and executes it.
type FailoverCustomerSalesDecisionPort struct {
	Primary  ports.CustomerSalesDecisionPort
	Fallback ports.CustomerSalesDecisionPort
}

func (p FailoverCustomerSalesDecisionPort) Decide(ctx context.Context, input ports.CustomerSalesDecisionInput) (ports.CustomerSalesDecisionOutput, error) {
	if p.Primary == nil {
		return ports.CustomerSalesDecisionOutput{}, errors.New("primary decision provider not configured")
	}
	if err := ctx.Err(); err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}
	output, err := p.Primary.Decide(ctx, input)
	if err == nil || p.Fallback == nil || !shouldFailoverAI(ctx, err) {
		return output, err
	}
	// Both providers see the same canonical request and offered capabilities.
	// The caller remains responsible for one validation/execution path.
	replacement, secondaryErr := p.Fallback.Decide(ctx, input)
	if secondaryErr != nil {
		return ports.CustomerSalesDecisionOutput{}, fmt.Errorf("primary model unavailable (%v), LiteLLM fallback failed: %w", err, secondaryErr)
	}
	return replacement, nil
}

func shouldFailoverAI(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var status interface{ Retryable() bool }
	if errors.As(err, &status) {
		return status.Retryable()
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

var _ ports.CustomerSalesDecisionPort = FailoverCustomerSalesDecisionPort{}

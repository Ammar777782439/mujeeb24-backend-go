package services

import (
	"context"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type stubCustomerSalesPolicy struct {
	decision ports.CustomerSalesPolicyDecision
}

func (s stubCustomerSalesPolicy) Evaluate(context.Context, ports.CustomerSalesProposal, *ports.CustomerSalesContext) ports.CustomerSalesPolicyDecision {
	return s.decision
}

func TestPolicyDecisionEmptyRequiresApproval(t *testing.T) {
	pipeline := NewValidationPipeline(nil, nil, stubCustomerSalesPolicy{
		decision: ports.CustomerSalesPolicyDecision{},
	}, nil)

	decision, failure := pipeline.evaluateCustomerSalesPolicy(context.Background(), ValidationInput{
		Proposal: ports.CustomerSalesProposal{Action: ports.CustomerSalesProposalActionAnswer},
	})
	if failure != nil {
		t.Fatalf("unexpected failure: %v", failure)
	}
	if decision.PolicyDecision != "requires_approval" {
		t.Fatalf("PolicyDecision=%q, want requires_approval", decision.PolicyDecision)
	}
}

func TestPolicyDecisionUnknownRequiresApproval(t *testing.T) {
	pipeline := NewValidationPipeline(nil, nil, stubCustomerSalesPolicy{
		decision: ports.CustomerSalesPolicyDecision{Decision: "unexpected"},
	}, nil)

	decision, failure := pipeline.evaluateCustomerSalesPolicy(context.Background(), ValidationInput{
		Proposal: ports.CustomerSalesProposal{Action: ports.CustomerSalesProposalActionAnswer},
	})
	if failure != nil {
		t.Fatalf("unexpected failure: %v", failure)
	}
	if decision.PolicyDecision != "requires_approval" {
		t.Fatalf("PolicyDecision=%q, want requires_approval", decision.PolicyDecision)
	}
}

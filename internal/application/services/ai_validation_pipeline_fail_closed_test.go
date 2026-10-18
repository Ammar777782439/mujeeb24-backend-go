package services

import (
	"context"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type stubCustomerSalesPolicy struct {
	decision ports.CustomerSalesPolicyDecision
}

func (s stubCustomerSalesPolicy) Evaluate(context.Context, ports.CapabilityDecision, *ports.CustomerSalesContext) ports.CustomerSalesPolicyDecision {
	return s.decision
}

func TestPolicyDecisionEmptyRequiresApproval(t *testing.T) {
	pipeline := NewValidationPipeline(nil, nil, stubCustomerSalesPolicy{
		decision: ports.CustomerSalesPolicyDecision{},
	}, nil)

	decision, failure := pipeline.evaluateCustomerSalesPolicy(context.Background(), ValidationInput{
		Decision: ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityRespondWithoutAction},
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
		Decision: ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityRespondWithoutAction},
	})
	if failure != nil {
		t.Fatalf("unexpected failure: %v", failure)
	}
	if decision.PolicyDecision != "requires_approval" {
		t.Fatalf("PolicyDecision=%q, want requires_approval", decision.PolicyDecision)
	}
}

func TestCapabilityCannotBeReplacedWithLegacyAction(t *testing.T) {
	pipeline := &ValidationPipeline{}
	if failure := pipeline.validateStructural(ports.CapabilityDecision{Capability: "human_request", ResponseText: "handoff"}); failure == nil {
		t.Fatal("legacy action is not a capability")
	}
}

type capabilityAuthorizerStub struct{ output ports.EffectiveDecision }

func (s capabilityAuthorizerStub) Authorize(context.Context, ValidationInput, ports.EffectiveDecision) (ports.EffectiveDecision, error) {
	return s.output, nil
}

func TestAuthorizationCannotReplaceCapabilityOrBypassPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, policy, outputPolicy string
		outputCapability           ports.CustomerSalesCapability
		reject                     bool
	}{
		{"same choice", "allowed", "allowed", ports.CustomerSalesCapabilityAsk, false},
		{"changed choice", "allowed", "allowed", ports.CustomerSalesCapabilityHandoff, true},
		{"bypass review", "requires_approval", "allowed", ports.CustomerSalesCapabilityAsk, true},
		{"bypass denial", "denied", "allowed", ports.CustomerSalesCapabilityAsk, true},
		{"empty policy", "allowed", "", ports.CustomerSalesCapabilityAsk, true},
		{"unknown policy", "allowed", "unexpected", ports.CustomerSalesCapabilityAsk, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pipeline := NewValidationPipeline(nil, nil, nil, capabilityAuthorizerStub{output: ports.EffectiveDecision{Capability: tc.outputCapability, PolicyDecision: tc.outputPolicy}})
			authorized, failure := pipeline.authorize(context.Background(), ValidationInput{Decision: ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityAsk}}, ports.EffectiveDecision{Capability: ports.CustomerSalesCapabilityAsk, PolicyDecision: tc.policy})
			if (failure != nil) != tc.reject {
				t.Fatalf("authorized=%+v failure=%v", authorized, failure)
			}
			if !tc.reject && (authorized.Capability != ports.CustomerSalesCapabilityAsk || authorized.AuthorizedAt == nil) {
				t.Fatal("lost authorized capability or timestamp")
			}
		})
	}
}

func TestStructuralValidationRejectsUnexecutedHumanHandoffPromises(t *testing.T) {
	pipeline := &ValidationPipeline{}
	for _, tc := range []struct {
		name       string
		capability ports.CustomerSalesCapability
		text       string
		reject     bool
	}{
		{"answer promises transfer", ports.CustomerSalesCapabilityRespondWithoutAction, "سأحوّلك إلى موظف خدمة العملاء", true},
		{"answer says transfer complete", ports.CustomerSalesCapabilityRespondWithoutAction, "تم تحويل طلبك للفريق المختص.", true},
		{"ask falsely promises followup", ports.CustomerSalesCapabilityAsk, "فريقنا سيتواصل معك قريباً. ما رقم الطلب؟", true},
		{"neutral process explanation", ports.CustomerSalesCapabilityRespondWithoutAction, "يمكنك طلب المساعدة من موظف من خلال لوحة التحكم.", false},
		{"genuine human handoff capability", ports.CustomerSalesCapabilityHandoff, "سأحوّلك إلى موظف خدمة العملاء", false},
		{"ordinary clarification", ports.CustomerSalesCapabilityAsk, "هل تريد الشرح بالتفصيل؟", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := pipeline.validateStructural(ports.CapabilityDecision{Capability: tc.capability, ResponseText: tc.text})
			if tc.reject {
				if failure == nil || failure.Stage != ports.AIRunFailureStageValidation ||
					failure.Category != ports.AIRunFailureCategoryInvalidAIOutput ||
					failure.Reason != "unexecuted_handoff_claim_in_response_text" {
					t.Fatalf("unsafe customer claim bypassed validation: %+v", failure)
				}
			} else if failure != nil {
				t.Fatalf("safe decision rejected: %+v", failure)
			}
		})
	}
}

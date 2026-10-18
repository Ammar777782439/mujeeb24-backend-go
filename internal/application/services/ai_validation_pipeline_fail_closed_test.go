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

func TestStructuralValidationChecksCapabilitiesNotNaturalLanguageIntent(t *testing.T) {
	pipeline := &ValidationPipeline{}
	for _, tc := range []struct {
		name       string
		capability ports.CustomerSalesCapability
		text       string
	}{
		{"model chose informational reply", ports.CustomerSalesCapabilityRespondWithoutAction, "سيتم تحويلك إلى موظف خدمة العملاء"},
		{"model chose clarification", ports.CustomerSalesCapabilityAsk, "فريقنا سيتواصل معك. ما رقم الطلب؟"},
		{"model chose actual human handoff", ports.CustomerSalesCapabilityHandoff, "سأحوّلك إلى الموظف المختص"},
		{"ordinary informational reply", ports.CustomerSalesCapabilityRespondWithoutAction, "نقدم خدمات متنوعة"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decision := ports.CapabilityDecision{Capability: tc.capability, ResponseText: tc.text}
			if failure := pipeline.validateStructural(decision); failure != nil {
				t.Fatalf("Go must validate the capability structure without classifying reply wording: %v", failure)
			}
			if decision.Capability != tc.capability {
				t.Fatal("Go may not infer or silently replace the model-selected capability")
			}
		})
	}
}

func TestStructuralValidationStillRejectsMissingResponseText(t *testing.T) {
	pipeline := &ValidationPipeline{}
	for _, capability := range []ports.CustomerSalesCapability{
		ports.CustomerSalesCapabilityRespondWithoutAction,
		ports.CustomerSalesCapabilityAsk,
		ports.CustomerSalesCapabilityHandoff,
	} {
		if failure := pipeline.validateStructural(ports.CapabilityDecision{Capability: capability}); failure == nil {
			t.Fatalf("missing customer text accepted for %q", capability)
		}
	}
}

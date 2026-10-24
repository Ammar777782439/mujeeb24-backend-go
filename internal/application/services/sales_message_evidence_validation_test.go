package services

import (
	"context"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestSalesReferenceValidationAllowsOnlyCurrentOrExposedInboundCustomer(t *testing.T) {
	p := &ValidationPipeline{ReferenceValidator: allowAllReferenceValidator{}}
	salesContext := &ports.CustomerSalesContext{
		Sales: &ports.SalesWorkflowContext{Enabled: true, AllowInteractions: true},
		RecentMessages: []ports.CustomerSalesRecentMessageEvidence{
			{Reference: "prior-inbound", Direction: "inbound", Origin: "customer", Text: "اشتراك"},
			{Reference: "outbound", Direction: "outbound", Origin: "ai", Text: "تم"},
			{Reference: "another-tenant", Direction: "inbound", Origin: "customer", Text: ""},
		},
	}
	for _, tc := range []struct {
		ref   string
		allow bool
	}{
		{"provider-current", true},
		{"prior-inbound", true},
		{"outbound", false},
		{"another-tenant", false},
		{"fake", false},
		{"", false},
	} {
		decision := ports.CapabilityDecision{
			Capability: ports.CustomerSalesCapabilityRecordLead,
			Sales:      &ports.SalesWorkflowProposal{EvidenceMessageIDs: []string{tc.ref}},
		}
		failure := p.validateReferences(context.Background(), ValidationInput{
			SourceMessageReference: "provider-current", BusinessID: "business-1",
			ConversationID: "conversation-1", Context: salesContext, Decision: decision,
		})
		if (failure == nil) != tc.allow {
			t.Fatalf("ref=%q allowed=%t failure=%v", tc.ref, tc.allow, failure)
		}
	}
}

package postgres

import (
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"testing"
)

func TestHandoffCustomerMessageRespectsAutoReplyPolicy(t *testing.T) {
	tests := []struct {
		name   string
		policy ports.BusinessRuntimePolicyRecord
		want   string
	}{
		{"enabled", ports.BusinessRuntimePolicyRecord{AIMode: "restricted_auto", AllowAutoReply: true}, "allowed"},
		{"reply disabled", ports.BusinessRuntimePolicyRecord{AIMode: "restricted_auto", AllowAutoReply: false}, "requires_approval"},
		{"review required", ports.BusinessRuntimePolicyRecord{AIMode: "restricted_auto", AllowAutoReply: true, DefaultHumanReview: true}, "requires_approval"},
		{"approval mode", ports.BusinessRuntimePolicyRecord{AIMode: "approval", AllowAutoReply: true}, "requires_approval"},
		{"disabled", ports.BusinessRuntimePolicyRecord{AIMode: "disabled", AllowAutoReply: true}, "denied"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := evaluatePolicyAgainstCapability(tc.policy, ports.CustomerSalesCapabilityHandoff); got != tc.want {
				t.Fatalf("handoff policy=%s want=%s", got, tc.want)
			}
		})
	}
}

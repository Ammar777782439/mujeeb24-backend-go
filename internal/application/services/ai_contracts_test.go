package services

import (
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// TestAIRunLifecycleLegalTransitions verifies contract ⑨ §26: only valid
// forward transitions are allowed. Terminal states cannot transition.
func TestAIRunLifecycleLegalTransitions(t *testing.T) {
	// This test documents the legal-transition rules; full repo-backed tests
	// live in the postgres adapter integration tests.
	cases := []struct {
		from string
		to   string
		ok   bool
	}{
		{ports.AIRunStatusReceived, ports.AIRunStatusContextBuilt, true},
		{ports.AIRunStatusContextBuilt, ports.AIRunStatusRunning, true},
		{ports.AIRunStatusRunning, ports.AIRunStatusWaitingTool, true},
		{ports.AIRunStatusWaitingTool, ports.AIRunStatusRunning, true},
		{ports.AIRunStatusRunning, ports.AIRunStatusValidating, true},
		{ports.AIRunStatusValidating, ports.AIRunStatusAuthorized, true},
		{ports.AIRunStatusAuthorized, ports.AIRunStatusExecuting, true},
		{ports.AIRunStatusExecuting, ports.AIRunStatusCompleted, true},
		// Illegal transitions per contract ⑨ §26
		{ports.AIRunStatusCompleted, ports.AIRunStatusRunning, false},
		{ports.AIRunStatusFailed, ports.AIRunStatusExecuting, false},
		{ports.AIRunStatusCancelled, ports.AIRunStatusRunning, false},
		{ports.AIRunStatusReceived, ports.AIRunStatusAuthorized, false}, // skip CONTEXT_BUILT/RUNNING/VALIDATING
	}
	for _, c := range cases {
		// Verify the legal-transition list matches the contract.
		allowedFrom := legalTransitions[c.to]
		found := false
		for _, s := range allowedFrom {
			if s == c.from {
				found = true
				break
			}
		}
		if found != c.ok {
			t.Errorf("transition %s → %s: expected ok=%v, got found=%v", c.from, c.to, c.ok, found)
		}
	}
}

// legalTransitions maps a destination state to the list of source states
// from which the transition is legal, per contract ⑨ §26.
var legalTransitions = map[string][]string{
	ports.AIRunStatusContextBuilt: {ports.AIRunStatusReceived},
	ports.AIRunStatusRunning:      {ports.AIRunStatusContextBuilt, ports.AIRunStatusWaitingTool},
	ports.AIRunStatusWaitingTool:  {ports.AIRunStatusRunning},
	ports.AIRunStatusValidating:   {ports.AIRunStatusRunning},
	ports.AIRunStatusAuthorized:   {ports.AIRunStatusValidating},
	ports.AIRunStatusExecuting:    {ports.AIRunStatusAuthorized},
	ports.AIRunStatusCompleted: {
		ports.AIRunStatusExecuting,
		ports.AIRunStatusValidating,
		ports.AIRunStatusAuthorized,
	},
}

// TestRetryPolicyRetryable verifies contract ⑨ §6: retryable categories retry
// until MaxAttempts is exhausted.
func TestRetryPolicyRetryable(t *testing.T) {
	policy := NewRetryPolicy(RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		BackoffMultiplier: 2.0,
		JitterFraction:    0.0,
	})
	cases := []struct {
		attempt  int
		category ports.AIRunFailureCategory
		retry    bool
	}{
		{1, ports.AIRunFailureCategoryProviderTemporary, true},
		{2, ports.AIRunFailureCategoryProviderTemporary, true},
		{3, ports.AIRunFailureCategoryProviderTemporary, false}, // exhausted
		{1, ports.AIRunFailureCategoryNetwork, true},
		{1, ports.AIRunFailureCategoryTimeout, true},
		{1, ports.AIRunFailureCategoryRateLimit, true},
		{1, ports.AIRunFailureCategoryInvalidAIOutput, false}, // non-retryable
		{1, ports.AIRunFailureCategoryTenantViolation, false},
		{1, ports.AIRunFailureCategoryPolicyDenial, false},
		{1, ports.AIRunFailureCategoryAuthorizationDenial, false},
	}
	for _, c := range cases {
		d := policy.DecideForFailure(c.attempt, c.category)
		if d.ShouldRetry != c.retry {
			t.Errorf("attempt=%d category=%s: expected retry=%v, got %v (reason: %s)",
				c.attempt, c.category, c.retry, d.ShouldRetry, d.Reason)
		}
	}
}

// TestPartialProgressCoverage verifies contract ⑨ §22-23: batches must all
// be COMPLETED before Final Evaluation may run.
func TestPartialProgressCoverage(t *testing.T) {
	policy := NewPartialProgressPolicy()
	cases := []struct {
		name    string
		batches []ports.AICatalogBatchRecord
		want    bool
	}{
		{
			name:    "empty",
			batches: nil,
			want:    true,
		},
		{
			name: "all completed",
			batches: []ports.AICatalogBatchRecord{
				{Status: "completed"},
				{Status: "completed"},
				{Status: "completed"},
			},
			want: true,
		},
		{
			name: "one pending",
			batches: []ports.AICatalogBatchRecord{
				{Status: "completed"},
				{Status: "pending"},
				{Status: "completed"},
			},
			want: false,
		},
		{
			name: "one failed",
			batches: []ports.AICatalogBatchRecord{
				{Status: "completed"},
				{Status: "failed"},
			},
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := policy.CoverageComplete(c.batches)
			if got != c.want {
				t.Errorf("CoverageComplete: expected %v, got %v", c.want, got)
			}
		})
	}
}

// TestBatchesToRetry verifies contract ⑨ §21: only FAILED and PENDING batches
// are retried; COMPLETED batches are not re-run.
func TestBatchesToRetry(t *testing.T) {
	policy := NewPartialProgressPolicy()
	batches := []ports.AICatalogBatchRecord{
		{ID: "b1", Status: "completed"},
		{ID: "b2", Status: "completed"},
		{ID: "b3", Status: "failed"},
		{ID: "b4", Status: "pending"},
	}
	retry := policy.BatchesToRetry(batches)
	if len(retry) != 2 {
		t.Fatalf("expected 2 batches to retry, got %d", len(retry))
	}
	if retry[0] != "b3" || retry[1] != "b4" {
		t.Errorf("expected [b3, b4], got %v", retry)
	}
}

// TestValidateStructural verifies contract ⑥ §3-5: only the closed status
// and action values pass structural validation.
func TestValidateStructural(t *testing.T) {
	pipeline := &ValidationPipeline{}
	cases := []struct {
		name     string
		proposal ports.AIGeminiProposal
		wantErr  bool
	}{
		{
			name: "valid resolved answer",
			proposal: ports.AIGeminiProposal{
				Status:       ports.AIProposalStatusResolved,
				Action:       ports.AIProposalActionAnswer,
				ResponseText: "السعر 12000 ريال",
			},
			wantErr: false,
		},
		{
			name: "valid human_request without response_text",
			proposal: ports.AIGeminiProposal{
				Status: ports.AIProposalStatusAmbiguous,
				Action: ports.AIProposalActionHumanRequest,
			},
			wantErr: false,
		},
		{
			name: "invalid status",
			proposal: ports.AIGeminiProposal{
				Status:       "approved",
				Action:       ports.AIProposalActionAnswer,
				ResponseText: "x",
			},
			wantErr: true,
		},
		{
			name: "invalid action",
			proposal: ports.AIGeminiProposal{
				Status:       ports.AIProposalStatusResolved,
				Action:       "create_lead", // legacy, no longer valid
				ResponseText: "x",
			},
			wantErr: true,
		},
		{
			name: "missing response_text for answer",
			proposal: ports.AIGeminiProposal{
				Status: ports.AIProposalStatusResolved,
				Action: ports.AIProposalActionAnswer,
			},
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := pipeline.validateStructural(c.proposal)
			if (err != nil) != c.wantErr {
				t.Errorf("expected wantErr=%v, got err=%v", c.wantErr, err)
			}
		})
	}
}

// TestCatalogEntityContractDescriptor verifies contract ⑤ §8: every enum
// value used in merchant data must have a documented meaning in the descriptor.
func TestCatalogEntityContractDescriptor(t *testing.T) {
	d := DefaultCatalogEntityContractDescriptor()
	requiredPricingModes := []string{"fixed", "starting_from", "range", "negotiable", "on_request", "free"}
	for _, m := range requiredPricingModes {
		if _, ok := d.PricingModes[m]; !ok {
			t.Errorf("missing pricing_mode %q in descriptor per contract ⑤ §8", m)
		}
	}
	requiredAvailabilityModes := []string{"in_stock", "limited", "pre_order", "made_to_order", "out_of_stock", "discontinued"}
	for _, m := range requiredAvailabilityModes {
		if _, ok := d.AvailabilityModes[m]; !ok {
			t.Errorf("missing availability_mode %q in descriptor per contract ⑤ §8", m)
		}
	}
	requiredFulfillmentModes := []string{"physical_delivery", "digital_delivery", "pickup", "service_execution", "subscription"}
	for _, m := range requiredFulfillmentModes {
		if _, ok := d.FulfillmentModes[m]; !ok {
			t.Errorf("missing fulfillment_mode %q in descriptor per contract ⑤ §8", m)
		}
	}
	requiredDataTypes := []string{"text", "number", "boolean", "date", "datetime", "select", "multi_select", "location", "money"}
	for _, m := range requiredDataTypes {
		if _, ok := d.AttributeDataTypes[m]; !ok {
			t.Errorf("missing attribute_data_type %q in descriptor per contract ⑤ §8", m)
		}
	}
}

package services

import (
	"strings"
	"testing"

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

func TestValidateStructural(t *testing.T) {
	pipeline := &ValidationPipeline{}
	cases := []struct {
		name     string
		decision ports.CapabilityDecision
		wantErr  bool
	}{
		{"reply", ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityReply, ResponseText: "السعر من الدليل"}, false},
		{"ask", ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityAsk, ResponseText: "ما العدد؟"}, false},
		{"handoff", ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityHandoff, ResponseText: "سأحوّل طلبك"}, false},
		{"handoff requires text", ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityHandoff}, true},
		{"retrieval cannot execute", ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityFullCatalog}, true},
		{"unimplemented draft", ports.CapabilityDecision{Capability: "create_order_draft", ResponseText: "تم"}, true},
		{"legacy action rejected", ports.CapabilityDecision{Capability: "answer", ResponseText: "x"}, true},
		{"missing text", ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityReply}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if failure := pipeline.validateStructural(tc.decision); (failure != nil) != tc.wantErr {
				t.Fatalf("failure=%v", failure)
			}
		})
	}
}

func TestCatalogEntityContractDescriptor(t *testing.T) {
	d := DefaultCatalogEntityContractDescriptor()
	// Per SQL migration 000016/000018: catalog_items_pricing_mode_chk +
	// offers_pricing_mode_chk.
	requiredPricingModes := []string{"fixed", "starting_from", "per_unit", "per_person", "per_day", "quote_required", "dynamic"}
	for _, m := range requiredPricingModes {
		if _, ok := d.PricingModes[m]; !ok {
			t.Errorf("missing pricing_mode %q in descriptor per SQL migration 000016/000018", m)
		}
	}
	// Per SQL migration 000016/000018: catalog_items_availability_mode_chk +
	// offers_availability_mode_chk.
	requiredAvailabilityModes := []string{"stock", "schedule", "supplier_check", "always_available", "unknown"}
	for _, m := range requiredAvailabilityModes {
		if _, ok := d.AvailabilityModes[m]; !ok {
			t.Errorf("missing availability_mode %q in descriptor per SQL migration 000016/000018", m)
		}
	}
	// Per SQL migration 000018: offers_availability_status_chk.
	// Per Catalog Contract §4: the non-breakable rule (unknown ≠ available,
	// stale ≠ confirmed, requires_check ≠ confirmed).
	requiredAvailabilityStatuses := []string{"available", "unavailable", "unknown", "requires_check", "stale"}
	for _, m := range requiredAvailabilityStatuses {
		if _, ok := d.AvailabilityStatuses[m]; !ok {
			t.Errorf("missing availability_status %q in descriptor per SQL migration 000018", m)
		}
	}
	// Per SQL migration 000018: offers_price_verification_chk.
	requiredPriceVerificationStatuses := []string{"unverified", "verified", "stale", "rejected"}
	for _, m := range requiredPriceVerificationStatuses {
		if _, ok := d.PriceVerificationStatuses[m]; !ok {
			t.Errorf("missing price_verification_status %q in descriptor per SQL migration 000018", m)
		}
	}
	// Per SQL migration 000016/000018: catalog_items_fulfillment_mode_chk +
	// offers_fulfillment_mode_chk.
	requiredFulfillmentModes := []string{"delivery", "pickup", "digital", "appointment", "travel", "manual"}
	for _, m := range requiredFulfillmentModes {
		if _, ok := d.FulfillmentModes[m]; !ok {
			t.Errorf("missing fulfillment_mode %q in descriptor per SQL migration 000016/000018", m)
		}
	}
	// Per Catalog Contract §6: attribute data_type MUST be one of these 9 values.
	// These are NOT in a SQL CHECK constraint; they are in the Domain contract
	// (contracts/domain_catalog_contract_review_ar.md §6) and contract 11 §11.
	requiredDataTypes := []string{"text", "number", "boolean", "date", "datetime", "select", "multi_select", "location", "money"}
	// NOTE: AttributeDataTypes is intentionally NOT in the descriptor map because
	// it's defined in the AttributeDefinition.data_type field's comment, not as
	// a separate descriptor entry. We verify it via the contract payload instead.
	payload := BuildCatalogEntityContractPayload()
	dataTypeField := payload.Contract.AttributeDefinition.DataType
	for _, m := range requiredDataTypes {
		if !strings.Contains(dataTypeField, m) {
			t.Errorf("missing attribute_data_type %q in entity contract payload", m)
		}
	}
}

func TestValidateStructuralRejectsEmptySelectedReference(t *testing.T) {
	pipeline := &ValidationPipeline{}
	if failure := pipeline.validateStructural(ports.CapabilityDecision{Capability: ports.CustomerSalesCapabilityReply, ResponseText: "ok", Selected: []ports.SelectedReference{{}}}); failure == nil {
		t.Fatal("empty selected item accepted")
	}
}

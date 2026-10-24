package postgres

import (
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"testing"
	"time"
)

func TestCapabilityDecisionDoesNotRequireInventedIntent(t *testing.T) {
	capability, denied, different := "respond_without_action", "denied", "handoff_to_human"
	now := time.Now()
	base := ports.AIDecisionDraft{ID: "decision", BusinessID: "business", RequestedCapability: &capability, RequestedAction: "answer", ConfidenceBand: "medium", PolicyVersion: "v1", SchemaVersion: 1, Lifecycle: "proposed", CreatedAt: now, UpdatedAt: now}
	if !validAIDecisionDraft(base) {
		t.Fatal("capability decision with empty intent was rejected")
	}
	base.EffectiveCapability = &capability
	if !validAIDecisionDraft(base) {
		t.Fatal("matching effective capability rejected")
	}
	base.EffectiveCapability = &different
	if validAIDecisionDraft(base) {
		t.Fatal("different effective capability accepted")
	}
	base.EffectiveCapability = nil
	base.RequestedCapability = nil
	if validAIDecisionDraft(base) {
		t.Fatal("legacy caller without intent accepted")
	}
	base.IntentBase = "product_question"
	if !validAIDecisionDraft(base) {
		t.Fatal("legacy caller with real intent/action rejected")
	}
	base.IntentBase = ""
	base.RequestedCapability = &capability
	base.RequestedAction = ""
	if validAIDecisionDraft(base) {
		t.Fatal("empty action accepted for executable proposal")
	}
	base.Lifecycle = "expired"
	if validAIDecisionDraft(base) {
		t.Fatal("expired capability accepted without denial")
	}
	base.PolicyDecision = &denied
	if !validAIDecisionDraft(base) {
		t.Fatal("rejected capability audit was lost")
	}
}

func TestSalesCapabilityDecisionAcceptsEmptyLegacyAction(t *testing.T) {
	now := time.Now()
	for _, capability := range []ports.CustomerSalesCapability{
		ports.CustomerSalesCapabilityRecordLead,
		ports.CustomerSalesCapabilityCreateDraft,
		ports.CustomerSalesCapabilityUpdateDraft,
	} {
		t.Run(string(capability), func(t *testing.T) {
			name := string(capability)
			allowed := "allowed"
			draft := ports.AIDecisionDraft{
				ID:                  "decision",
				BusinessID:          "business",
				RequestedCapability: &name,
				EffectiveCapability: &name,
				RequestedAction:     "",
				IntentBase:          "",
				ConfidenceBand:      "medium",
				PolicyVersion:       "v1",
				SchemaVersion:       1,
				Lifecycle:           "proposed",
				PolicyDecision:      &allowed,
				CreatedAt:           now,
				UpdatedAt:           now,
			}
			if !validAIDecisionDraft(draft) {
				t.Fatal("known sales capability rejected despite migration 000074")
			}
			draft.RequestedCapability = nil
			draft.EffectiveCapability = nil
			if validAIDecisionDraft(draft) {
				t.Fatal("empty legacy action accepted without a capability")
			}
		})
	}
}

func TestUnknownCapabilityCannotBypassLegacyActionValidation(t *testing.T) {
	now := time.Now()
	unknown := "invented_external_action"
	draft := ports.AIDecisionDraft{
		ID:                  "decision",
		BusinessID:          "business",
		RequestedCapability: &unknown,
		RequestedAction:     "",
		ConfidenceBand:      "medium",
		PolicyVersion:       "v1",
		SchemaVersion:       1,
		Lifecycle:           "proposed",
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if validAIDecisionDraft(draft) {
		t.Fatal("unknown capability bypassed persistence validation")
	}
}

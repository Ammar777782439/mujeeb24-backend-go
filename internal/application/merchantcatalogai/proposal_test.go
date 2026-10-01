package merchantcatalogai

import "testing"

func TestProposalNormalizeNonResolvedMutation(t *testing.T) {
	proposal := Proposal{
		SchemaVersion: 1,
		Status:        StatusNeedsMoreData,
		Operation:     OperationUpdate,
		ResponseText:  "أكمل بيانات المنتج.",
		Update: &UpdateOperation{
			ItemID: "00000000-0000-0000-0000-000000000001",
		},
	}

	normalized := proposal.Normalize()

	if normalized.Status != StatusNeedsMoreData {
		t.Fatalf("status = %q, want %q", normalized.Status, StatusNeedsMoreData)
	}
	if normalized.Operation != OperationAskMerchant {
		t.Fatalf("operation = %q, want %q", normalized.Operation, OperationAskMerchant)
	}
	if normalized.Update != nil || normalized.Create != nil || normalized.Delete != nil {
		t.Fatal("non-resolved mutation payload must be cleared")
	}
	if err := normalized.Validate(); err != nil {
		t.Fatalf("normalized proposal must validate: %v", err)
	}
}

func TestProposalNormalizeResolvedEmptyUpdate(t *testing.T) {
	proposal := Proposal{
		SchemaVersion: 1,
		Status:        StatusResolved,
		Operation:     OperationUpdate,
		ResponseText:  "تم التحديث.",
		Update: &UpdateOperation{
			ItemID: "00000000-0000-0000-0000-000000000001",
		},
	}

	normalized := proposal.Normalize()

	if normalized.Status != StatusNeedsMoreData {
		t.Fatalf("status = %q, want %q", normalized.Status, StatusNeedsMoreData)
	}
	if normalized.Operation != OperationAskMerchant {
		t.Fatalf("operation = %q, want %q", normalized.Operation, OperationAskMerchant)
	}
	if normalized.Update != nil {
		t.Fatal("empty resolved update must be cleared")
	}
	if err := normalized.Validate(); err != nil {
		t.Fatalf("normalized proposal must validate: %v", err)
	}
}


func TestReadOnlyCapabilityRegistryRejectsUnknownMutationReference(t *testing.T) {
	registry := &ReadOnlyCapabilityRegistry{
		evidenceReferences: map[string]struct{}{
			"item-1":  {},
			"offer-1": {},
		},
	}

	proposal := Proposal{
		SchemaVersion: 1,
		Status:        StatusResolved,
		Operation:     OperationUpdate,
		ResponseText:  "أعددت اقتراح التعديل.",
		EvidenceReferences: []string{"item-1"},
		Update: &UpdateOperation{
			ItemID: "item-1",
			ExistingOffers: []OfferUpdate{{ID: "offer-unknown"}},
		},
	}

	if err := registry.ValidateProposalReferences(proposal); err == nil {
		t.Fatal("expected unknown offer reference to be rejected")
	}
}

func TestReadOnlyCapabilityRegistryAcceptsEvidenceBackedUpdate(t *testing.T) {
	registry := &ReadOnlyCapabilityRegistry{
		evidenceReferences: map[string]struct{}{
			"item-1":  {},
			"offer-1": {},
		},
	}

	proposal := Proposal{
		SchemaVersion:      1,
		Status:             StatusResolved,
		Operation:          OperationUpdate,
		ResponseText:       "أعددت اقتراح تعديل السعر.",
		EvidenceReferences: []string{"item-1", "offer-1"},
		Update: &UpdateOperation{
			ItemID:          "item-1",
			ExistingOffers:  []OfferUpdate{{ID: "offer-1"}},
		},
	}

	if err := registry.ValidateProposalReferences(proposal); err != nil {
		t.Fatalf("expected evidence-backed update to validate: %v", err)
	}
}

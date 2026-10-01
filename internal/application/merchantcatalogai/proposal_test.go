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

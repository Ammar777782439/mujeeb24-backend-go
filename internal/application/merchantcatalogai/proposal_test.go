package merchantcatalogai

import "testing"

func TestProposalNormalizeNonResolvedMutation(t *testing.T) {
	proposal := Proposal{
		SchemaVersion: ProposalSchemaVersion,
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


func TestProposalValidateCreateResolvesNewVariantOffersByRef(t *testing.T) {
	proposal := Proposal{
		SchemaVersion: ProposalSchemaVersion,
		Status:        StatusResolved,
		Operation:     OperationCreate,
		ResponseText:  "أعددت اقتراح إضافة المنتج.",
		Create: &ItemCreate{
			Name:                  "ساعة",
			ItemType:              "physical_good",
			PricingMode:            "fixed",
			AvailabilityMode:      "always_available",
			FulfillmentMode:       "delivery",
			RequiresConfirmation: false,
			Variants: []VariantCreate{
				{Ref: "variant-black", Name: "أسود"},
				{Ref: "variant-yellow", Name: "أصفر"},
				{Ref: "variant-red", Name: "أحمر"},
			},
			Offers: []OfferCreate{
				{Name: DefaultOfferName, VariantRef: stringPtr("variant-black"), PricingMode: "fixed", Amount: stringPtr("20000"), AvailabilityMode: "always_available", AvailabilityStatus: "available", FulfillmentMode: "delivery", Status: "active"},
				{Name: DefaultOfferName, VariantRef: stringPtr("variant-yellow"), PricingMode: "fixed", Amount: stringPtr("22000"), AvailabilityMode: "always_available", AvailabilityStatus: "available", FulfillmentMode: "delivery", Status: "active"},
				{Name: DefaultOfferName, VariantRef: stringPtr("variant-red"), PricingMode: "fixed", Amount: stringPtr("25000"), AvailabilityMode: "always_available", AvailabilityStatus: "available", FulfillmentMode: "delivery", Status: "active"},
			},
		},
	}
	if err := proposal.Validate(); err != nil {
		t.Fatalf("expected proposal to validate: %v", err)
	}
}

func TestProposalValidateCreateRejectsDatabaseVariantID(t *testing.T) {
	proposal := Proposal{
		SchemaVersion: ProposalSchemaVersion,
		Status:        StatusResolved,
		Operation:     OperationCreate,
		ResponseText:  "أعددت اقتراح إضافة المنتج.",
		Create: &ItemCreate{
			Name:                  "ساعة",
			ItemType:              "physical_good",
			PricingMode:            "fixed",
			AvailabilityMode:      "always_available",
			FulfillmentMode:       "delivery",
			RequiresConfirmation: false,
			Variants:               []VariantCreate{{Ref: "variant-red", Name: "أحمر"}},
			Offers: []OfferCreate{{
				VariantID:          stringPtr("00000000-0000-0000-0000-000000000001"),
				Name:               DefaultOfferName,
				PricingMode:        "fixed",
				Amount:             stringPtr("25000"),
				AvailabilityMode:   "always_available",
				AvailabilityStatus: "available",
				FulfillmentMode:    "delivery",
				Status:             "active",
			}},
		},
	}
	if err := proposal.Validate(); err == nil {
		t.Fatal("expected create offer with database variant_id to be rejected")
	}
}

func TestProposalValidateRejectsUnknownVariantRef(t *testing.T) {
	proposal := Proposal{
		SchemaVersion: ProposalSchemaVersion,
		Status:        StatusResolved,
		Operation:     OperationCreate,
		ResponseText:  "أعددت اقتراح إضافة المنتج.",
		Create: &ItemCreate{
			Name:                  "ساعة",
			ItemType:              "physical_good",
			PricingMode:            "fixed",
			AvailabilityMode:      "always_available",
			FulfillmentMode:       "delivery",
			RequiresConfirmation: false,
			Variants:               []VariantCreate{{Ref: "variant-red", Name: "أحمر"}},
			Offers: []OfferCreate{{
				Name:               DefaultOfferName,
				VariantRef:         stringPtr("variant-blue"),
				PricingMode:        "fixed",
				Amount:             stringPtr("25000"),
				AvailabilityMode:   "always_available",
				AvailabilityStatus: "available",
				FulfillmentMode:    "delivery",
				Status:             "active",
			}},
		},
	}
	if err := proposal.Validate(); err == nil {
		t.Fatal("expected unknown variant_ref to be rejected")
	}
}

func stringPtr(value string) *string { return &value }

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


func TestProposalValidateCreateOfferProvenance(t *testing.T) {
	base := func(offer OfferCreate) Proposal {
		return Proposal{
			SchemaVersion: ProposalSchemaVersion,
			Status: StatusResolved,
			Operation: OperationCreate,
			ResponseText: "أعددت الاقتراح.",
			Create: &ItemCreate{
				Name: "ساعة",
				ItemType: "physical_good",
				PricingMode: "fixed",
				AvailabilityMode: "always_available",
				FulfillmentMode: "delivery",
				Offers: []OfferCreate{offer},
			},
		}
	}

	tests := []struct {
		name string
		offer OfferCreate
		wantErr bool
	}{
		{
			name: "merchant stated concrete price",
			offer: OfferCreate{
				Name: DefaultOfferName, NameSource: OfferNameSourceSystemDefault,
				PricingMode: "fixed", Amount: stringPtr("22000"), PriceSource: OfferPriceSourceMerchantStated,
				AvailabilityMode: "always_available", AvailabilityStatus: "available", FulfillmentMode: "delivery", Status: "active",
			},
		},
		{
			name: "merchant stated price cannot be quote required",
			offer: OfferCreate{
				Name: DefaultOfferName, NameSource: OfferNameSourceSystemDefault,
				PricingMode: "quote_required", Amount: nil, PriceSource: OfferPriceSourceMerchantStated,
				AvailabilityMode: "always_available", AvailabilityStatus: "available", FulfillmentMode: "delivery", Status: "active",
			},
			wantErr: true,
		},
		{
			name: "default offer name cannot contain variant",
			offer: OfferCreate{
				Name: "سعر البيع - أحمر", NameSource: OfferNameSourceSystemDefault,
				PricingMode: "fixed", Amount: stringPtr("25000"), PriceSource: OfferPriceSourceMerchantStated,
				AvailabilityMode: "always_available", AvailabilityStatus: "available", FulfillmentMode: "delivery", Status: "active",
			},
			wantErr: true,
		},
		{
			name: "not stated price stays empty",
			offer: OfferCreate{
				Name: DefaultOfferName, NameSource: OfferNameSourceSystemDefault,
				PricingMode: "quote_required", Amount: nil, PriceSource: OfferPriceSourceNotStated,
				AvailabilityMode: "always_available", AvailabilityStatus: "available", FulfillmentMode: "delivery", Status: "active",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := base(tt.offer).Validate()
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}


func TestProposalValidateAllowsDynamicAttributesWithoutSchema(t *testing.T) {
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
			Attributes: map[string]any{
				"water_resistant": true,
				"warranty_period": map[string]any{"value": 1, "unit": "year"},
				"custom_value":    []any{"A", 12, false},
			},
			Offers: []OfferCreate{{
				Name:               DefaultOfferName,
				NameSource:         OfferNameSourceSystemDefault,
				PricingMode:        "fixed",
				Amount:             stringPtr("20000"),
				PriceSource:        OfferPriceSourceMerchantStated,
				Currency:            stringPtr("YER"),
				AvailabilityMode:   "always_available",
				AvailabilityStatus: "available",
				FulfillmentMode:    "delivery",
				Status:             "active",
			}},
		},
	}
	if err := proposal.Validate(); err != nil {
		t.Fatalf("dynamic attributes without schema must validate: %v", err)
	}
}

func TestProposalValidateRejectsNonEnglishAttributeKey(t *testing.T) {
	proposal := Proposal{
		SchemaVersion: ProposalSchemaVersion,
		Status:        StatusResolved,
		Operation:     OperationCreate,
		ResponseText:  "أعددت الاقتراح.",
		Create: &ItemCreate{
			Name:                  "ساعة",
			ItemType:              "physical_good",
			PricingMode:            "fixed",
			AvailabilityMode:      "always_available",
			FulfillmentMode:       "delivery",
			Attributes:             map[string]any{"مقاوم_للماء": true},
			Offers: []OfferCreate{{
				Name:               DefaultOfferName,
				NameSource:         OfferNameSourceSystemDefault,
				PricingMode:        "fixed",
				Amount:             stringPtr("20000"),
				PriceSource:        OfferPriceSourceMerchantStated,
				Currency:            stringPtr("YER"),
				AvailabilityMode:   "always_available",
				AvailabilityStatus: "available",
				FulfillmentMode:    "delivery",
				Status:             "active",
			}},
		},
	}
	if err := proposal.Validate(); err == nil {
		t.Fatal("expected non-English attribute key to be rejected")
	}
}

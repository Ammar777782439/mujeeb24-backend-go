package ports

import "testing"

func TestCatalogAIEvidenceSetContainsSelection(t *testing.T) {
	variantA := "variant-a"
	variantB := "variant-b"
	offerA := "offer-a"

	evidence := NewCatalogAIEvidenceSet()
	evidence.AddBundle(CatalogAIProjectionBundle{
		Item: CatalogItemRecord{ID: "item-a"},
		Variants: []VariantRecord{
			{ID: variantA, CatalogItemID: "item-a"},
		},
		Offers: []OfferRecord{
			{ID: offerA, CatalogItemID: "item-a", VariantID: &variantA},
		},
	})

	if !evidence.ContainsSelection(SelectedReference{ItemID: "item-a", VariantID: &variantA, OfferID: &offerA}) {
		t.Fatal("expected exact item/variant/offer tuple to be valid")
	}
	if evidence.ContainsSelection(SelectedReference{ItemID: "item-missing"}) {
		t.Fatal("invented item must not be valid evidence")
	}
	if evidence.ContainsSelection(SelectedReference{ItemID: "item-a", VariantID: &variantB}) {
		t.Fatal("variant not exposed under item must be rejected")
	}
	if evidence.ContainsSelection(SelectedReference{ItemID: "item-a", VariantID: &variantB, OfferID: &offerA}) {
		t.Fatal("offer/variant mismatch must be rejected")
	}
}

func TestCatalogAIEvidenceSetDoesNotTrustOfferFromAnotherItem(t *testing.T) {
	offer := "offer-b"
	evidence := NewCatalogAIEvidenceSet()
	evidence.AddBundle(CatalogAIProjectionBundle{
		Item: CatalogItemRecord{ID: "item-a"},
		Offers: []OfferRecord{
			{ID: offer, CatalogItemID: "item-b"},
		},
	})
	if evidence.ContainsSelection(SelectedReference{ItemID: "item-a", OfferID: &offer}) {
		t.Fatal("offer belonging to another item must never enter item evidence")
	}
}

func TestCatalogAIEvidenceSetMerge(t *testing.T) {
	v := "variant-a"
	first := NewCatalogAIEvidenceSet()
	first.AddBundle(CatalogAIProjectionBundle{
		Item: CatalogItemRecord{ID: "item-a"},
	})
	second := NewCatalogAIEvidenceSet()
	second.AddBundle(CatalogAIProjectionBundle{
		Item: CatalogItemRecord{ID: "item-a"},
		Variants: []VariantRecord{{ID: v, CatalogItemID: "item-a"}},
	})
	first.Merge(second)
	if !first.ContainsSelection(SelectedReference{ItemID: "item-a", VariantID: &v}) {
		t.Fatal("merged evidence must retain relational children")
	}
}

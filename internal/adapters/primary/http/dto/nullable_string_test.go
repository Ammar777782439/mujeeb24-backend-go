package dto

import (
	"encoding/json"
	"testing"
)

func TestOptionalNullableStringTracksPatchPresence(t *testing.T) {
	var omitted UpdateCatalogItemRequest
	if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil {
		t.Fatal(err)
	}
	if omitted.ShortDescription.Present || omitted.LongDescription.Present {
		t.Fatal("omitted descriptions must remain absent")
	}

	var cleared UpdateCatalogItemRequest
	if err := json.Unmarshal([]byte(`{"short_description":null}`), &cleared); err != nil {
		t.Fatal(err)
	}
	if !cleared.ShortDescription.Present || cleared.ShortDescription.Value != nil {
		t.Fatalf("explicit null was not preserved: %#v", cleared.ShortDescription)
	}

	var updated UpdateCatalogItemRequest
	if err := json.Unmarshal([]byte(`{"long_description":"تفاصيل جديدة"}`), &updated); err != nil {
		t.Fatal(err)
	}
	if !updated.LongDescription.Present || updated.LongDescription.Value == nil || *updated.LongDescription.Value != "تفاصيل جديدة" {
		t.Fatalf("string update was not preserved: %#v", updated.LongDescription)
	}
}

func TestUpdateOfferRequestTracksNullablePatchPresence(t *testing.T) {
	var request UpdateOfferRequest
	if err := json.Unmarshal([]byte(`{"variant_id":null,"amount":null,"currency":null,"pricing_unit":null,"validity_from":null,"validity_until":"2026-12-01T10:00:00Z"}`), &request); err != nil {
		t.Fatal(err)
	}
	if !request.VariantID.Present || request.VariantID.Value != nil {
		t.Fatalf("variant_id null was not preserved: %#v", request.VariantID)
	}
	if !request.Amount.Present || request.Amount.Value != nil {
		t.Fatalf("amount null was not preserved: %#v", request.Amount)
	}
	if !request.Currency.Present || request.Currency.Value != nil {
		t.Fatalf("currency null was not preserved: %#v", request.Currency)
	}
	if !request.PricingUnit.Present || request.PricingUnit.Value != nil {
		t.Fatalf("pricing_unit null was not preserved: %#v", request.PricingUnit)
	}
	if !request.ValidityFrom.Present || request.ValidityFrom.Value != nil {
		t.Fatalf("validity_from null was not preserved: %#v", request.ValidityFrom)
	}
	if !request.ValidityUntil.Present || request.ValidityUntil.Value == nil {
		t.Fatalf("validity_until value was not preserved: %#v", request.ValidityUntil)
	}

	var omitted UpdateOfferRequest
	if err := json.Unmarshal([]byte(`{}`), &omitted); err != nil {
		t.Fatal(err)
	}
	if omitted.Amount.Present || omitted.Currency.Present || omitted.VariantID.Present || omitted.ValidityFrom.Present {
		t.Fatal("omitted nullable offer fields must stay absent")
	}
}

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

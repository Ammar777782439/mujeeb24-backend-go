package services

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCompleteCommercialEntityResolvesSchemaMeaningAndVariantInheritance(t *testing.T) {
	id, version := "schema-phone", 3
	catalog := CatalogAICatalog{ID: "catalog-a", Name: "Store devices", Status: "active"}
	schema := CatalogAIAttributeSchema{ID: id, Version: version, Definitions: []CatalogAIAttributeDefinition{
		{ID: "definition-storage", SchemaID: id, AttributeKey: "attr_k71", Label: "سعة التخزين", DataType: "text"},
		{ID: "definition-color", SchemaID: id, AttributeKey: "attr_m92", Label: "اللون", DataType: "text"},
	}}
	item := CatalogAIItem{
		ID: "phone", CatalogID: catalog.ID, AttributeSchemaID: &id, AttributeSchemaVersion: &version,
		Attributes: map[string]any{"attr_k71": "256GB", "attr_m92": "black"},
		Variants: []CatalogAIVariant{
			{ID: "red", CatalogItemID: "phone", Attributes: map[string]any{"attr_m92": "red"}},
			{ID: "black", CatalogItemID: "phone"},
		},
		Offers: []CatalogAIOffer{
			{ID: "red-offer", CatalogItemID: "phone", VariantID: func() *string { v := "red"; return &v }()},
			{ID: "standalone-offer", CatalogItemID: "phone"},
		},
	}
	attachItemSchemaMeaning(&item, &schema)
	projection := CatalogAIProjection{Catalogs: []CatalogAICatalog{catalog}, AttributeSchemas: []CatalogAIAttributeSchema{schema}, Items: []CatalogAIItem{item}}
	attachItemCatalogMeaning(&projection)
	if projection.Items[0].Catalog == nil || projection.Items[0].Catalog.Name != catalog.Name {
		t.Fatal("product missing its embedded catalog meaning")
	}
	if projection.Items[0].Schema == nil || projection.Items[0].Schema.Version != version {
		t.Fatal("product missing its versioned schema")
	}
	if len(projection.Items[0].ItemLevelOffers) != 1 || projection.Items[0].ItemLevelOffers[0].ID != "standalone-offer" {
		t.Fatalf("item-level offer not directly linked: %+v", projection.Items[0].ItemLevelOffers)
	}
	if len(projection.Items[0].Variants[0].Offers) != 1 || projection.Items[0].Variants[0].Offers[0].ID != "red-offer" {
		t.Fatalf("variant offer not directly linked: %+v", projection.Items[0].Variants[0].Offers)
	}
	if len(projection.Items[0].Variants[1].Offers) != 0 {
		t.Fatalf("offer incorrectly leaked between variants: %+v", projection.Items[0].Variants[1].Offers)
	}
	for _, variant := range projection.Items[0].Variants {
		if got := variant.EffectiveAttributes["attr_k71"]; got != "256GB" {
			t.Fatalf("variant lost inherited storage: %v", got)
		}
		if variant.ID == "red" {
			if variant.EffectiveAttributes["attr_m92"] != "red" {
				t.Fatalf("explicit red override lost: %+v", variant.EffectiveAttributes)
			}
			expected := []CatalogAIResolvedAttribute{
				{Key: "attr_k71", Label: "سعة التخزين", DataType: "text", Origin: "item", Value: "256GB"},
				{Key: "attr_m92", Label: "اللون", DataType: "text", Origin: "variant", Value: "red"},
			}
			if !reflect.DeepEqual(variant.ResolvedAttributes, expected) {
				t.Fatalf("incorrect resolved semantics: got %+v, want %+v", variant.ResolvedAttributes, expected)
			}
		}
	}
	encoded, err := json.Marshal(projection.Items[0])
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(encoded) {
		t.Fatal("complete semantic entity failed JSON serialization")
	}
}

func TestJSONCommercialAttributeNumberMaintainsPrecision(t *testing.T) {
	raw := []byte(`{"high_precision":9007199254740993,"price_ratio":0.12345678901234567890123456789}`)
	parsed := parseJSONAttributes(raw)
	if got := parsed["high_precision"].(json.Number).String(); got != "9007199254740993" {
		t.Fatalf("large integer lost precision: %s", got)
	}
	if got := parsed["price_ratio"].(json.Number).String(); got != "0.12345678901234567890123456789" {
		t.Fatalf("decimal lost precision: %s", got)
	}
}

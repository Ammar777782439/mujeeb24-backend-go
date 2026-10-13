package gemini

import (
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

func TestBatchCandidateSchemaRestrictsReferencesToBatch(t *testing.T) {
	a, b := "variant-a", "variant-b"
	cases := []struct {
		name     string
		batch    services.CatalogAIBatchPayload
		expected []string
	}{
		{
			name: "first batch",
			batch: services.CatalogAIBatchPayload{Items: []services.CatalogAIItem{{
				ID: "item-a",
				Variants: []services.CatalogAIVariant{{ID: a}},
				Offers: []services.CatalogAIOffer{{ID: "offer-a", VariantID: &a}},
			}}},
			expected: []string{"item-a", "variant-a", "offer-a"},
		},
		{
			name: "later reduction batch",
			batch: services.CatalogAIBatchPayload{Items: []services.CatalogAIItem{{
				ID: "item-b",
				Variants: []services.CatalogAIVariant{{ID: b}},
				Offers: []services.CatalogAIOffer{{ID: "offer-b", VariantID: &b}},
			}}},
			expected: []string{"item-b", "variant-b", "offer-b"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			props := batchCandidateResponseSchemaForBatch(tc.batch)["properties"].(map[string]any)["candidates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
			for i, name := range []string{"item_id", "variant_ids", "offer_ids"} {
				prop := props[name].(map[string]any)
				if name != "item_id" {
					prop = prop["items"].(map[string]any)
				}
				ids, ok := prop["enum"].([]string)
				if !ok || len(ids) != 1 || ids[0] != tc.expected[i] {
					t.Fatalf("%s allowed references: %#v", name, prop["enum"])
				}
			}
		})
	}
	props := batchCandidateResponseSchema()["properties"].(map[string]any)["candidates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if _, ok := props["item_id"].(map[string]any)["enum"]; ok {
		t.Fatal("scoped schema modified the reusable base contract")
	}
}

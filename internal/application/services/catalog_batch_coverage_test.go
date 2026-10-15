package services

import (
	"context"
	"strings"
	"testing"
)

func TestCatalogBatchCoverageRetainsCommercialFacts(t *testing.T) {
	v1, v2, schemaID := "variant-1", "variant-2", "schema-1"
	schemaVersion := 2
	price := "12000"
	description := "وصف تفصيلي أصلي يجب ألا يُختصر أو يُعدل"
	source := CatalogAIProjection{
		Catalogs: []CatalogAICatalog{{ID: "catalog-1", Name: "Main", Status: "active"}},
		AttributeSchemas: []CatalogAIAttributeSchema{{
			ID: schemaID, Name: "Specs", Version: 2,
			Definitions: []CatalogAIAttributeDefinition{{ID: "attr-1", SchemaID: schemaID, AttributeKey: "ram", Label: "RAM", DataType: "text"}},
		}},
		Items: []CatalogAIItem{{
			ID: "item-1", CatalogID: "catalog-1", Name: "Laptop",
			LongDescription: &description, AttributeSchemaID: &schemaID, AttributeSchemaVersion: &schemaVersion,
			Attributes: map[string]any{"ram": "16GB"},
			Variants: []CatalogAIVariant{
				{ID: v1, CatalogItemID: "item-1", Attributes: map[string]any{"color": "blue"}},
				{ID: v2, CatalogItemID: "item-1", Attributes: map[string]any{"color": "black"}},
			},
			Offers: []CatalogAIOffer{
				{ID: "offer-1", CatalogItemID: "item-1", VariantID: &v1, Amount: &price},
				{ID: "offer-2", CatalogItemID: "item-1", VariantID: &v2, Amount: &price},
			},
		}},
	}
	stub := &catalogTokenEstimatorStub{countFn: func(input BatchEvaluationInput) (int, error) {
		count := 200
		for _, item := range input.Batch.Items {
			count += 100 * (len(item.Variants) + len(item.Offers))
		}
		return count, nil
	}}
	controller := &CatalogBatchController{Evaluator: stub, TokenBudget: 900}
	batches, err := controller.splitIntoBatches(context.Background(), source, CatalogEvaluationInput{AIRunID: "run", BusinessID: "business"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 {
		t.Fatalf("expected one indivisible complete commercial entity, got %d", len(batches))
	}
	if err := validateCatalogBatchCoverage(source, batches); err != nil {
		t.Fatalf("complete commercial entity rejected: %v", err)
	}

	t.Run("changed offer price", func(t *testing.T) {
		changed := append([]CatalogAIBatchPayload(nil), batches...)
		changed[0].Items = append([]CatalogAIItem(nil), batches[0].Items...)
		changed[0].Items[0].Offers = append([]CatalogAIOffer(nil), batches[0].Items[0].Offers...)
		bad := "1"
		changed[0].Items[0].Offers[0].Amount = &bad
		if err := validateCatalogBatchCoverage(source, changed); err == nil {
			t.Fatal("changed price passed coverage")
		}
	})

	t.Run("detached offer from variant", func(t *testing.T) {
		changed := append([]CatalogAIBatchPayload(nil), batches...)
		changed[0].Items = append([]CatalogAIItem(nil), batches[0].Items...)
		changed[0].Items[0].Variants = nil
		if err := validateCatalogBatchCoverage(source, changed); err == nil || !strings.Contains(err.Error(), "lost or changed") {
			t.Fatalf("detached offer passed coverage: %v", err)
		}
	})

	t.Run("dropped schema definitions", func(t *testing.T) {
		changed := append([]CatalogAIBatchPayload(nil), batches...)
		changed[0].AttributeSchemas = nil
		if err := validateCatalogBatchCoverage(source, changed); err == nil {
			t.Fatal("missing schema passed coverage")
		}
	})

	t.Run("truncated description", func(t *testing.T) {
		changed := append([]CatalogAIBatchPayload(nil), batches...)
		changed[0].Items = append([]CatalogAIItem(nil), batches[0].Items...)
		bad := "truncated"
		changed[0].Items[0].LongDescription = &bad
		if err := validateCatalogBatchCoverage(source, changed); err == nil {
			t.Fatal("truncated description passed coverage")
		}
	})

	t.Run("duplicate entity", func(t *testing.T) {
		duplicate := append([]CatalogAIBatchPayload(nil), batches...)
		duplicate = append(duplicate, batches[0])
		if err := validateCatalogBatchCoverage(source, duplicate); err == nil || !strings.Contains(err.Error(), "duplicated or fragmented") {
			t.Fatalf("duplicate entity was accepted: %v", err)
		}
	})

	t.Run("missing linked variant", func(t *testing.T) {
		changed := append([]CatalogAIBatchPayload(nil), batches...)
		changed[0].Items = append([]CatalogAIItem(nil), batches[0].Items...)
		changed[0].Items[0].Variants = changed[0].Items[0].Variants[:1]
		if err := validateCatalogBatchCoverage(source, changed); err == nil {
			t.Fatal("incomplete entity was accepted")
		}
	})

		t.Run("missing offer", func(t *testing.T) {
		changed := append([]CatalogAIBatchPayload(nil), batches...)
		changed[0].Items = append([]CatalogAIItem(nil), batches[0].Items...)
		changed[0].Items[0].Offers = nil
		if err := validateCatalogBatchCoverage(source, changed); err == nil || !strings.Contains(err.Error(), "lost or changed") {
			t.Fatalf("missing offer passed coverage: %v", err)
		}
	})
}

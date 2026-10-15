package services

import (
	"fmt"
	"reflect"
)

// validateCatalogBatchCoverage proves BEFORE provider invocation that each item
// is transported exactly once and unchanged. A complete commercial entity is
// indivisible: partial variants/offers and duplicate fragments are rejected.
func validateCatalogBatchCoverage(source CatalogAIProjection, batches []CatalogAIBatchPayload) error {
	catalogs := make(map[string]CatalogAICatalog, len(source.Catalogs))
	for _, catalog := range source.Catalogs {
		if _, exists := catalogs[catalog.ID]; exists {
			return fmt.Errorf("duplicate source catalog %s", catalog.ID)
		}
		catalogs[catalog.ID] = catalog
	}
	schemas := make(map[string]CatalogAIAttributeSchema, len(source.AttributeSchemas))
	for _, schema := range source.AttributeSchemas {
		if _, exists := schemas[schema.ID]; exists {
			return fmt.Errorf("duplicate source schema %s", schema.ID)
		}
		schemas[schema.ID] = schema
	}
	items := make(map[string]CatalogAIItem, len(source.Items))
	for _, item := range source.Items {
		if _, exists := items[item.ID]; exists {
			return fmt.Errorf("duplicate source catalog item %s", item.ID)
		}
		if err := validateCatalogItemRelationships(item); err != nil {
			return err
		}
		items[item.ID] = item
	}

	seen := make(map[string]bool, len(items))
	for _, batch := range batches {
		batchCatalogs := make(map[string]CatalogAICatalog, len(batch.Catalogs))
		for _, catalog := range batch.Catalogs {
			if _, exists := batchCatalogs[catalog.ID]; exists {
				return fmt.Errorf("batch %d duplicated catalog %s", batch.BatchNumber, catalog.ID)
			}
			batchCatalogs[catalog.ID] = catalog
		}
		batchSchemas := make(map[string]CatalogAIAttributeSchema, len(batch.AttributeSchemas))
		for _, schema := range batch.AttributeSchemas {
			if _, exists := batchSchemas[schema.ID]; exists {
				return fmt.Errorf("batch %d duplicated schema %s", batch.BatchNumber, schema.ID)
			}
			batchSchemas[schema.ID] = schema
		}
		for _, entity := range batch.Items {
			original, ok := items[entity.ID]
			if !ok {
				return fmt.Errorf("batch %d contains unknown item %s", batch.BatchNumber, entity.ID)
			}
			if seen[entity.ID] {
				return fmt.Errorf("catalog item %s was duplicated or fragmented across batches", entity.ID)
			}
			seen[entity.ID] = true
			if !reflect.DeepEqual(original, entity) {
				return fmt.Errorf("batch %d lost or changed commercial fields, variants or offers of item %s", batch.BatchNumber, entity.ID)
			}
			if len(source.Catalogs) > 0 {
				catalog, ok := catalogs[entity.CatalogID]
				if !ok {
					return fmt.Errorf("source lacks catalog %s for item %s", entity.CatalogID, entity.ID)
				}
				if !reflect.DeepEqual(batchCatalogs[entity.CatalogID], catalog) {
					return fmt.Errorf("batch %d lost catalog %s for item %s", batch.BatchNumber, entity.CatalogID, entity.ID)
				}
			}
			if entity.AttributeSchemaID != nil && *entity.AttributeSchemaID != "" {
				schema, ok := schemas[*entity.AttributeSchemaID]
				if !ok {
					return fmt.Errorf("source lacks schema %s for item %s", *entity.AttributeSchemaID, entity.ID)
				}
				if entity.AttributeSchemaVersion == nil || *entity.AttributeSchemaVersion != schema.Version {
					return fmt.Errorf("schema version mismatch for item %s", entity.ID)
				}
				if !reflect.DeepEqual(batchSchemas[schema.ID], schema) {
					return fmt.Errorf("batch %d lost schema definitions %s for item %s", batch.BatchNumber, schema.ID, entity.ID)
				}
			}
		}
	}
	for _, item := range source.Items {
		if !seen[item.ID] {
			return fmt.Errorf("catalog item %s was not covered by any batch", item.ID)
		}
	}
	return nil
}

func validateCatalogItemRelationships(item CatalogAIItem) error {
	variants := make(map[string]bool, len(item.Variants))
	for _, variant := range item.Variants {
		if variant.CatalogItemID != item.ID {
			return fmt.Errorf("variant %s belongs to another item, not %s", variant.ID, item.ID)
		}
		if variants[variant.ID] {
			return fmt.Errorf("duplicate variant %s in item %s", variant.ID, item.ID)
		}
		variants[variant.ID] = true
	}
	offers := make(map[string]bool, len(item.Offers))
	for _, offer := range item.Offers {
		if offer.CatalogItemID != item.ID {
			return fmt.Errorf("offer %s belongs to another item, not %s", offer.ID, item.ID)
		}
		if offers[offer.ID] {
			return fmt.Errorf("duplicate offer %s in item %s", offer.ID, item.ID)
		}
		offers[offer.ID] = true
		if offer.VariantID != nil && *offer.VariantID != "" && !variants[*offer.VariantID] {
			return fmt.Errorf("offer %s references unavailable variant %s in item %s", offer.ID, *offer.VariantID, item.ID)
		}
	}
	return nil
}

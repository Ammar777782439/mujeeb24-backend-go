package services

import (
	"fmt"
	"reflect"
)

// validateCatalogBatchCoverage proves that each source page has been
// transported losslessly into its AI batches BEFORE sending any requests.
// It compares commercial values and schemas, not just item/offer counts.
// Understanding those facts correctly remains the model's responsibility.
func validateCatalogBatchCoverage(source CatalogAIProjection, batches []CatalogAIBatchPayload) error {
	catalogs := make(map[string]CatalogAICatalog, len(source.Catalogs))
	for _, catalog := range source.Catalogs {
		catalogs[catalog.ID] = catalog
	}
	schemas := make(map[string]CatalogAIAttributeSchema, len(source.AttributeSchemas))
	for _, schema := range source.AttributeSchemas {
		schemas[schema.ID] = schema
	}
	items := make(map[string]CatalogAIItem, len(source.Items))
	for _, item := range source.Items {
		if _, exists := items[item.ID]; exists {
			return fmt.Errorf("duplicate source catalog item %s", item.ID)
		}
		items[item.ID] = item
	}

	seenItems := make(map[string]bool, len(items))
	seenVariants := make(map[string]map[string]bool, len(items))
	seenOffers := make(map[string]map[string]bool, len(items))
	for _, batch := range batches {
		batchCatalogs := make(map[string]CatalogAICatalog, len(batch.Catalogs))
		for _, catalog := range batch.Catalogs {
			batchCatalogs[catalog.ID] = catalog
		}
		batchSchemas := make(map[string]CatalogAIAttributeSchema, len(batch.AttributeSchemas))
		for _, schema := range batch.AttributeSchemas {
			batchSchemas[schema.ID] = schema
		}
		for _, fragment := range batch.Items {
			original, ok := items[fragment.ID]
			if !ok {
				return fmt.Errorf("batch %d contains unknown item %s", batch.BatchNumber, fragment.ID)
			}
			parent := fragment
			parent.Variants, parent.Offers = nil, nil
			expected := original
			expected.Variants, expected.Offers = nil, nil
			if !reflect.DeepEqual(parent, expected) {
				return fmt.Errorf("batch %d changed commercial fields of item %s", batch.BatchNumber, fragment.ID)
			}

			if original.CatalogID != "" && len(source.Catalogs) > 0 {
				catalog, exists := catalogs[original.CatalogID]
				if !exists {
					return fmt.Errorf("source lacks catalog %s for item %s", original.CatalogID, fragment.ID)
				}
				if !reflect.DeepEqual(batchCatalogs[original.CatalogID], catalog) {
					return fmt.Errorf("batch %d lost catalog %s for item %s", batch.BatchNumber, original.CatalogID, fragment.ID)
				}
			}
			if original.AttributeSchemaID != nil && *original.AttributeSchemaID != "" {
				schemaID := *original.AttributeSchemaID
				schema, exists := schemas[schemaID]
				if !exists {
					return fmt.Errorf("source lacks schema %s for item %s", schemaID, fragment.ID)
				}
				if !reflect.DeepEqual(batchSchemas[schemaID], schema) {
					return fmt.Errorf("batch %d lost schema definitions %s for item %s", batch.BatchNumber, schemaID, fragment.ID)
				}
			}
			seenItems[fragment.ID] = true
			if seenVariants[fragment.ID] == nil {
				seenVariants[fragment.ID] = make(map[string]bool)
			}
			if seenOffers[fragment.ID] == nil {
				seenOffers[fragment.ID] = make(map[string]bool)
			}
			originalVariants := make(map[string]CatalogAIVariant, len(original.Variants))
			fragmentVariantIDs := make(map[string]bool, len(fragment.Variants))
			for _, v := range original.Variants {
				originalVariants[v.ID] = v
			}
			for _, v := range fragment.Variants {
				expectedVariant, exists := originalVariants[v.ID]
				if !exists || !reflect.DeepEqual(v, expectedVariant) {
					return fmt.Errorf("batch %d lost or changed variant %s of item %s", batch.BatchNumber, v.ID, fragment.ID)
				}
				fragmentVariantIDs[v.ID] = true
				seenVariants[fragment.ID][v.ID] = true
			}
			originalOffers := make(map[string]CatalogAIOffer, len(original.Offers))
			for _, offer := range original.Offers {
				originalOffers[offer.ID] = offer
			}
			for _, offer := range fragment.Offers {
				expectedOffer, exists := originalOffers[offer.ID]
				if !exists || !reflect.DeepEqual(offer, expectedOffer) {
					return fmt.Errorf("batch %d lost or changed offer %s of item %s", batch.BatchNumber, offer.ID, fragment.ID)
				}
				if offer.VariantID != nil && *offer.VariantID != "" {
					// A real variant may not be separated from its offer.
					if _, exists := originalVariants[*offer.VariantID]; exists && !fragmentVariantIDs[*offer.VariantID] {
						return fmt.Errorf("batch %d separated offer %s from variant %s", batch.BatchNumber, offer.ID, *offer.VariantID)
					}
				}
				seenOffers[fragment.ID][offer.ID] = true
			}
		}
	}

	for _, item := range source.Items {
		if !seenItems[item.ID] {
			return fmt.Errorf("catalog item %s was not covered by any batch", item.ID)
		}
		for _, variant := range item.Variants {
			if !seenVariants[item.ID][variant.ID] {
				return fmt.Errorf("catalog item %s lost variant %s in batching", item.ID, variant.ID)
			}
		}
		for _, offer := range item.Offers {
			if !seenOffers[item.ID][offer.ID] {
				return fmt.Errorf("catalog item %s lost offer %s in batching", item.ID, offer.ID)
			}
		}
	}
	return nil
}

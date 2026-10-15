package services

import "sort"

// attachItemSchemaMeaning materializes human-readable semantics without
// changing canonical attributes. Schema definitions remain the source of truth;
// Go does not infer product meaning from keys or merchant sectors.
func attachItemSchemaMeaning(item *CatalogAIItem, schema *CatalogAIAttributeSchema) {
	if item == nil {
		return
	}
	item.Schema = schema
	var definitions []CatalogAIAttributeDefinition
	if schema != nil {
		definitions = schema.Definitions
	}
	item.ResolvedAttributes = resolveCatalogAttributes(item.Attributes, nil, definitions)
	for i := range item.Variants {
		variant := &item.Variants[i]
		variant.EffectiveAttributes = mergeCatalogAttributes(item.Attributes, variant.Attributes)
		variant.ResolvedAttributes = resolveCatalogAttributes(item.Attributes, variant.Attributes, definitions)
	}
}

// An explicit variant attribute overrides a parent attribute. Missing variant
// keys inherit the exact original parent value; no new commercial value is
// created from the key, prompt or model.
func mergeCatalogAttributes(parent, overrides map[string]any) map[string]any {
	if len(parent) == 0 && len(overrides) == 0 {
		return nil
	}
	out := make(map[string]any, len(parent)+len(overrides))
	for key, value := range parent {
		out[key] = value
	}
	for key, value := range overrides {
		out[key] = value
	}
	return out
}

func resolveCatalogAttributes(parent, overrides map[string]any, definitions []CatalogAIAttributeDefinition) []CatalogAIResolvedAttribute {
	merged := mergeCatalogAttributes(parent, overrides)
	if len(merged) == 0 {
		return nil
	}
	defByKey := make(map[string]CatalogAIAttributeDefinition, len(definitions))
	for _, definition := range definitions {
		defByKey[definition.AttributeKey] = definition
	}
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]CatalogAIResolvedAttribute, 0, len(keys))
	for _, key := range keys {
		fact := CatalogAIResolvedAttribute{Key: key, Value: merged[key], Origin: "item"}
		if _, changed := overrides[key]; changed {
			fact.Origin = "variant"
		}
		if definition, known := defByKey[key]; known {
			fact.Label = definition.Label
			fact.DataType = definition.DataType
		} else {
			// Free attributes have no definition. Do not invent a natural-
			// language meaning: preserve only their literal key.
			fact.Label = key
		}
		out = append(out, fact)
	}
	return out
}

func attachItemCatalogMeaning(projection *CatalogAIProjection) {
	if projection == nil {
		return
	}
	byID := make(map[string]CatalogAICatalog, len(projection.Catalogs))
	for _, catalog := range projection.Catalogs {
		byID[catalog.ID] = catalog
	}
	for i := range projection.Items {
		if catalog, exists := byID[projection.Items[i].CatalogID]; exists {
			copyCatalog := catalog
			projection.Items[i].Catalog = &copyCatalog
		}
	}
}

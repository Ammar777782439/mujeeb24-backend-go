package gemini

import (
	"fmt"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"sort"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

func merchantCatalogProposalSchema() map[string]any {
	stringField := func() map[string]any { return map[string]any{"type": "string"} }
	optionalString := func() map[string]any { return map[string]any{"type": "string"} }
	attributeObjectField := func() map[string]any {
		return map[string]any{
			"type": "object",
			"propertyNames": map[string]any{
				"pattern": "^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$",
			},
			"additionalProperties": true,
			"description":          "Dynamic attribute object. Keys are English snake_case; values may be any valid JSON value. Keys do not need to exist in the database.",
		}
	}

	offerNameSource := map[string]any{
		"type": "string",
		"enum": []string{merchantcatalogai.OfferNameSourceSystemDefault, merchantcatalogai.OfferNameSourceMerchantStated},
	}
	offerPriceSource := map[string]any{
		"type": "string",
		"enum": []string{merchantcatalogai.OfferPriceSourceMerchantStated, merchantcatalogai.OfferPriceSourceNotStated},
	}
	offerPricingMode := map[string]any{
		"type": "string",
		"enum": []string{"fixed", "starting_from", "per_unit", "per_person", "per_day", "quote_required", "dynamic"},
	}
	offerName := map[string]any{
		"type":        "string",
		"description": fmt.Sprintf("Commercial offer name. If the merchant did not explicitly provide a distinct commercial label, name_source must be system_default and name must be exactly %q. Never append or derive the variant name, color, option, or attribute to the default offer name.", merchantcatalogai.DefaultOfferName),
	}
	offerAmount := map[string]any{
		"type":        []string{"string", "null"},
		"description": "Exact merchant-supplied amount when price_source is merchant_stated. Null is allowed only when price_source is not_stated or pricing_mode is dynamic/quote_required according to the contract.",
	}
	// Name provenance and price provenance must both hold, not either one.
	nameCases := []map[string]any{
		{"name_source": map[string]any{"enum": []string{merchantcatalogai.OfferNameSourceSystemDefault}}, "name": map[string]any{"enum": []string{merchantcatalogai.DefaultOfferName}}},
		{"name_source": map[string]any{"enum": []string{merchantcatalogai.OfferNameSourceMerchantStated}}},
	}
	priceCases := []map[string]any{
		{"price_source": map[string]any{"enum": []string{merchantcatalogai.OfferPriceSourceMerchantStated}}, "amount": map[string]any{"type": "string"}, "pricing_mode": map[string]any{"enum": []string{"fixed", "starting_from", "per_unit", "per_person", "per_day", "dynamic"}}},
		{"price_source": map[string]any{"enum": []string{merchantcatalogai.OfferPriceSourceNotStated}}, "amount": map[string]any{"type": "null"}, "pricing_mode": map[string]any{"enum": []string{"quote_required", "dynamic"}}},
	}
	offerPricingSemantics := make([]map[string]any, 0, len(nameCases)*len(priceCases))
	for _, nameCase := range nameCases {
		for _, priceCase := range priceCases {
			properties := make(map[string]any)
			for key, value := range nameCase {
				properties[key] = value
			}
			for key, value := range priceCase {
				properties[key] = value
			}
			offerPricingSemantics = append(offerPricingSemantics, map[string]any{"properties": properties})
		}
	}

	missingField := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": stringField(), "display_name": stringField(), "data_type": stringField(), "reason": stringField(),
		},
		"required": []string{"path", "display_name", "data_type", "reason"},
	}
	itemCreate := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": stringField(), "attribute_schema_id": optionalString(), "item_type": stringField(),
			"short_description": optionalString(), "long_description": optionalString(), "pricing_mode": stringField(),
			"availability_mode": stringField(), "fulfillment_mode": stringField(),
			"requires_confirmation": map[string]any{"type": "boolean"},
			"attributes":            attributeObjectField(),
			"variants": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ref":        stringField(),
						"name":       stringField(),
						"attributes": attributeObjectField(),
					},
					"required": []string{"ref", "name"},
				},
				"description": "New variants are not persisted yet. Each variant requires a unique proposal-local ref used by offers to establish relationships before database IDs exist.",
			},
			"offers": map[string]any{
				"type":        "array",
				"minItems":    1,
				"description": "A resolved create proposal must include offer data. Provenance is contractual: merchant_stated means the merchant explicitly supplied the price; system_default means Mujeeb supplied the canonical offer name. Never derive a commercial offer name from a variant.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"variant_ref":         optionalString(),
						"name":                offerName,
						"name_source":         offerNameSource,
						"pricing_mode":        offerPricingMode,
						"amount":              offerAmount,
						"price_source":        offerPriceSource,
						"currency":            stringField(),
						"pricing_unit":        optionalString(),
						"availability_mode":   stringField(),
						"availability_status": stringField(),
						"fulfillment_mode":    stringField(),
						"status":              stringField(),
					},
					"required": []string{"name", "name_source", "pricing_mode", "amount", "price_source", "availability_mode", "availability_status", "fulfillment_mode", "status", "currency"},
					"anyOf":    offerPricingSemantics,
				},
			},
		},
		"required": []string{"name", "item_type", "pricing_mode", "availability_mode", "fulfillment_mode", "requires_confirmation", "offers"},
	}
	update := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"item_id": stringField(),
			"changes": map[string]any{"type": "object", "properties": map[string]any{
				"name": optionalString(), "status": optionalString(), "attributes": attributeObjectField(),
				"item_type":         optionalString(),
				"short_description": optionalString(), "long_description": optionalString(),
				"pricing_mode": optionalString(), "availability_mode": optionalString(),
				"fulfillment_mode":      optionalString(),
				"requires_confirmation": map[string]any{"type": "boolean"},
			}},
			"existing_variants": map[string]any{"type": "array", "items": map[string]any{
				"type": "object", "properties": map[string]any{
					"id": stringField(), "name": optionalString(), "attributes": attributeObjectField(), "status": optionalString(),
				}, "required": []string{"id"},
			}},
			"new_variants": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ref":        stringField(),
						"name":       stringField(),
						"attributes": attributeObjectField(),
					},
					"required": []string{"ref", "name"},
				},
			},
			"existing_offers": map[string]any{"type": "array", "description": "Modify only the offer(s) the merchant identified. When multiple offers could be the target, return ambiguous/ask_merchant rather than changing all offers. Modify existing offers using IDs returned by read tools. To change an existing offer price, set amount here; do not create a new offer to replace its price. Omit unchanged fields.", "items": map[string]any{
				"type": "object", "properties": map[string]any{
					"id": stringField(), "name": optionalString(), "amount": optionalString(),
					"availability_status": optionalString(), "status": optionalString(),
				}, "required": []string{"id"},
			}},
			"new_offers": map[string]any{
				"description": "Create additional offers only when the merchant requests a new offer. Existing offer price changes belong in existing_offers, not here.",
				"type":        "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"variant_id": map[string]any{
							"type":        "string",
							"description": "Existing Variant database ID returned by a catalog read tool. Use this only when the new offer targets an already-existing variant.",
						},
						"variant_ref": map[string]any{
							"type":        "string",
							"description": "Proposal-local ref of a new variant in update.new_variants. Use this before the new variant has a database ID.",
						},
						"name":                offerName,
						"name_source":         offerNameSource,
						"pricing_mode":        offerPricingMode,
						"amount":              offerAmount,
						"price_source":        offerPriceSource,
						"currency":            optionalString(),
						"pricing_unit":        optionalString(),
						"availability_mode":   stringField(),
						"availability_status": stringField(),
						"fulfillment_mode":    stringField(),
						"status":              stringField(),
					},
					"required": []string{"name", "name_source", "pricing_mode", "amount", "price_source", "availability_mode", "availability_status", "fulfillment_mode", "status", "currency"},
					"anyOf":    offerPricingSemantics,
				},
			},
		},
		"required": []string{"item_id", "changes"},
	}
	deletePayload := map[string]any{
		"type":       "object",
		"properties": map[string]any{"id": stringField(), "reason_given": optionalString()},
		"required":   []string{"id"},
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"schema_version":      map[string]any{"type": "integer", "enum": []int{merchantcatalogai.ProposalSchemaVersion}, "description": "Merchant Catalog AI Proposal Contract version."},
			"status":              map[string]any{"type": "string", "enum": []string{"resolved", "ambiguous", "not_found", "needs_more_data"}},
			"operation":           map[string]any{"type": "string", "enum": []string{"create", "update", "delete", "ask_merchant"}},
			"response_text":       map[string]any{"type": "string", "description": "One proposal targets one catalog item. Multi-item batches and attribute-schema mutations are not supported; return needs_more_data/ask_merchant explaining this limitation rather than silently processing a subset. Discounts and promotions have no executable fields in this contract. If requested, return needs_more_data/ask_merchant explaining this limitation; do not silently drop them or store an executable discount in attributes. Preserve every requested change. Describe only changes actually represented in the proposal payload. Do not claim a description or price change unless its corresponding field is present. A proposal is not proof of execution."},
			"evidence_references": map[string]any{"type": "array", "items": stringField()},
			"missing_information": map[string]any{"type": "array", "items": missingField},
			"create": map[string]any{
				"anyOf": []map[string]any{
					itemCreate,
					{"type": "null"},
				},
			},
			"update": map[string]any{
				"anyOf": []map[string]any{
					update,
					{"type": "null"},
				},
			},
			"delete": map[string]any{
				"anyOf": []map[string]any{
					deletePayload,
					{"type": "null"},
				},
			},
		},
		"required": []string{"schema_version", "status", "operation", "response_text", "create", "update", "delete"},
	}
	constrainMerchantCatalogSchemaValues(schema)
	closeMerchantCatalogSchemaObjects(schema)
	return schema
}

// Proposal structures are closed; dynamic attributes keep arbitrary JSON keys.
func closeMerchantCatalogSchemaObjects(schema map[string]any) {
	if schema["type"] == "object" {
		if _, defined := schema["properties"]; defined {
			schema["additionalProperties"] = false
		}
	}
	for _, value := range schema {
		switch child := value.(type) {
		case map[string]any:
			closeMerchantCatalogSchemaObjects(child)
		case []map[string]any:
			for _, entry := range child {
				closeMerchantCatalogSchemaObjects(entry)
			}
		}
	}
}

func constrainMerchantCatalogSchemaValues(schema map[string]any) {
	d := services.DefaultCatalogEntityContractDescriptor()
	enum := func(values map[string]string) map[string]any {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return map[string]any{"type": "string", "enum": keys}
	}
	root := schema["properties"].(map[string]any)
	create := root["create"].(map[string]any)["anyOf"].([]map[string]any)[0]["properties"].(map[string]any)
	update := root["update"].(map[string]any)["anyOf"].([]map[string]any)[0]["properties"].(map[string]any)
	changes := update["changes"].(map[string]any)["properties"].(map[string]any)
	for _, properties := range []map[string]any{create, changes} {
		properties["pricing_mode"] = enum(d.PricingModes)
		properties["availability_mode"] = enum(d.AvailabilityModes)
		properties["fulfillment_mode"] = enum(d.FulfillmentModes)
	}
	changes["status"] = enum(d.ItemStatuses)
	for _, offers := range []map[string]any{create["offers"].(map[string]any), update["new_offers"].(map[string]any)} {
		props := offers["items"].(map[string]any)["properties"].(map[string]any)
		props["availability_mode"] = enum(d.AvailabilityModes)
		props["availability_status"] = enum(d.AvailabilityStatuses)
		props["fulfillment_mode"] = enum(d.FulfillmentModes)
		props["status"] = enum(d.OfferStatuses)
	}
	existing := update["existing_offers"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	existing["availability_status"] = enum(d.AvailabilityStatuses)
	existing["status"] = enum(d.OfferStatuses)
	variants := update["existing_variants"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	variants["status"] = enum(d.VariantStatuses)
}

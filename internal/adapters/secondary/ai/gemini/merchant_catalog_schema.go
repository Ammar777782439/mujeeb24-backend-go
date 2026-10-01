package gemini

import (
	"fmt"

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
			"description": "Dynamic attribute object. Keys are English snake_case; values may be any valid JSON value. Keys do not need to exist in the database.",
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
		"type": "string",
		"description": fmt.Sprintf("Commercial offer name. If the merchant did not explicitly provide a distinct commercial label, name_source must be system_default and name must be exactly %q. Never append or derive the variant name, color, option, or attribute to the default offer name.", merchantcatalogai.DefaultOfferName),
	}
	offerAmount := map[string]any{
		"type": []string{"string", "null"},
		"description": "Exact merchant-supplied amount when price_source is merchant_stated. Null is allowed only when price_source is not_stated or pricing_mode is dynamic/quote_required according to the contract.",
	}
	offerPricingSemantics := []map[string]any{
		{"properties": map[string]any{
			"name_source": map[string]any{"enum": []string{merchantcatalogai.OfferNameSourceSystemDefault}},
			"name": map[string]any{"enum": []string{merchantcatalogai.DefaultOfferName}},
		}},
		{"properties": map[string]any{
			"name_source": map[string]any{"enum": []string{merchantcatalogai.OfferNameSourceMerchantStated}},
		}},
		{"properties": map[string]any{
			"price_source": map[string]any{"enum": []string{merchantcatalogai.OfferPriceSourceMerchantStated}},
			"amount": map[string]any{"type": "string"},
			"pricing_mode": map[string]any{"enum": []string{"fixed", "starting_from", "per_unit", "per_person", "per_day", "dynamic"}},
		}},
		{"properties": map[string]any{
			"price_source": map[string]any{"enum": []string{merchantcatalogai.OfferPriceSourceNotStated}},
			"amount": map[string]any{"type": "null"},
		}},
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
			"attributes": attributeObjectField(),
			"variants": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ref": stringField(),
						"name": stringField(),
						"attributes": attributeObjectField(),
					},
					"required": []string{"ref", "name"},
				},
				"description": "New variants are not persisted yet. Each variant requires a unique proposal-local ref used by offers to establish relationships before database IDs exist.",
			},
			"offers": map[string]any{
				"type": "array",
				"minItems": 1,
				"description": "A resolved create proposal must include offer data. Provenance is contractual: merchant_stated means the merchant explicitly supplied the price; system_default means Mujeeb supplied the canonical offer name. Never derive a commercial offer name from a variant.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"variant_ref": optionalString(),
						"name": offerName,
						"name_source": offerNameSource,
						"pricing_mode": offerPricingMode,
						"amount": offerAmount,
						"price_source": offerPriceSource,
						"currency": stringField(),
						"pricing_unit": optionalString(),
						"availability_mode": stringField(),
						"availability_status": stringField(),
						"fulfillment_mode": stringField(),
						"status": stringField(),
					},
					"required": []string{"name", "name_source", "pricing_mode", "amount", "price_source", "availability_mode", "availability_status", "fulfillment_mode", "status"},
					"anyOf": offerPricingSemantics,
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
						"ref": stringField(),
						"name": stringField(),
						"attributes": attributeObjectField(),
					},
					"required": []string{"ref", "name"},
				},
			},
			"existing_offers": map[string]any{"type": "array", "items": map[string]any{
				"type": "object", "properties": map[string]any{
					"id": stringField(), "name": optionalString(), "amount": optionalString(),
					"availability_status": optionalString(), "status": optionalString(),
				}, "required": []string{"id"},
			}},
			"new_offers": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"variant_id": map[string]any{
							"type": "string",
							"description": "Existing Variant database ID returned by a catalog read tool. Use this only when the new offer targets an already-existing variant.",
						},
						"variant_ref": map[string]any{
							"type": "string",
							"description": "Proposal-local ref of a new variant in update.new_variants. Use this before the new variant has a database ID.",
						},
						"name": offerName,
						"name_source": offerNameSource,
						"pricing_mode": offerPricingMode,
						"amount": offerAmount,
						"price_source": offerPriceSource,
						"currency": optionalString(),
						"pricing_unit": optionalString(),
						"availability_mode": stringField(),
						"availability_status": stringField(),
						"fulfillment_mode": stringField(),
						"status": stringField(),
					},
					"required": []string{"name", "name_source", "pricing_mode", "amount", "price_source", "availability_mode", "availability_status", "fulfillment_mode", "status"},
					"anyOf": offerPricingSemantics,
				},
			},
		},
		"required": []string{"item_id", "changes"},
	}
	deletePayload := map[string]any{
		"type": "object",
		"properties": map[string]any{"item_id": stringField(), "reason_given": optionalString()},
		"required": []string{"item_id"},
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"schema_version": map[string]any{"type": "integer", "enum": []int{merchantcatalogai.ProposalSchemaVersion}, "description": "Merchant Catalog AI Proposal Contract version."},
			"status": map[string]any{"type": "string", "enum": []string{"resolved", "ambiguous", "not_found", "needs_more_data"}},
			"operation": map[string]any{"type": "string", "enum": []string{"create", "update", "delete", "ask_merchant"}},
			"response_text": stringField(),
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
}

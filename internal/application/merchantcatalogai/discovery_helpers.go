package merchantcatalogai

import (
	"encoding/json"
	"errors"
	"strings"
)

func jsonParams(raw []byte) (map[string]any, error) {
	params := map[string]any{}
	if len(raw) == 0 {
		return params, nil
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, errors.New("invalid capability parameters")
	}
	return params, nil
}

func requiredString(params map[string]any, key string) (string, error) {
	value, ok := params[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", errors.New(key + " is required")
	}
	return strings.TrimSpace(value), nil
}

func readLimit(params map[string]any) int {
	limit := 25
	if value, ok := params["limit"].(float64); ok && int(value) > 0 {
		limit = int(value)
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func catalogEvidenceBase(item portsCatalogItemProjection) map[string]any {
	return map[string]any{
		"id":                        item.ID,
		"catalog_id":                item.CatalogID,
		"attribute_schema_id":       item.AttributeSchemaID,
		"attribute_schema_version":  item.AttributeSchemaVersion,
		"item_type":                 item.ItemType,
		"name":                      item.Name,
		"short_description":         item.ShortDescription,
		"long_description":          item.LongDescription,
		"status":                    item.Status,
		"pricing_mode":              item.PricingMode,
		"availability_mode":         item.AvailabilityMode,
		"fulfillment_mode":          item.FulfillmentMode,
		"requires_confirmation":     item.RequiresConfirmation,
		"attributes":                json.RawMessage(item.Attributes),
	}
}

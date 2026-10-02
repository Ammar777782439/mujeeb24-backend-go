package merchantcatalogai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type listItemsCapability struct {
	repository        ports.CatalogRepository
	selectedCatalogID string
}

func (c listItemsCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name:        "merchant_catalog_list_items",
		Description: "Read items from the already selected merchant catalog. This is bounded factual retrieval, not semantic search. The catalog is selected by Mujeeb; the model must not choose it.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{"type": "string"},
				"search": map[string]any{"type": "string", "description": "Deterministic name/text filter; not semantic search."},
				"limit":  map[string]any{"type": "integer"},
				"cursor": map[string]any{"type": "string"},
			},
			"required": []string{},
		},
	}
}

func (c listItemsCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	params, err := jsonParams(rawParams)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}
	if c.selectedCatalogID == "" {
		return ports.AICapabilityResult{}, errors.New("selected merchant catalog is required")
	}

	status, _ := params["status"].(string)
	search, _ := params["search"].(string)
	cursor, _ := params["cursor"].(string)
	page, err := c.repository.ListCatalogItems(ctx, execCtx.BusinessID, c.selectedCatalogID, strings.TrimSpace(search), status, readLimit(params), cursor)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}

	now := time.Now().UTC()
	items := make([]map[string]any, 0, len(page.Items))
	evidence := make([]ports.CustomerSalesCatalogEvidence, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, map[string]any{
			"id":                       item.ID,
			"catalog_id":               item.CatalogID,
			"attribute_schema_id":      item.AttributeSchemaID,
			"attribute_schema_version": item.AttributeSchemaVersion,
			"item_type":                item.ItemType,
			"name":                     item.Name,
			"short_description":        item.ShortDescription,
			"long_description":         item.LongDescription,
			"status":                   item.Status,
			"pricing_mode":             item.PricingMode,
			"availability_mode":        item.AvailabilityMode,
			"fulfillment_mode":         item.FulfillmentMode,
			"requires_confirmation":    item.RequiresConfirmation,
			"attributes":               json.RawMessage(item.Attributes),
		})
		evidence = append(evidence, ports.CustomerSalesCatalogEvidence{
			Reference:            item.ID,
			CatalogReference:     item.CatalogID,
			ItemType:             item.ItemType,
			Name:                 item.Name,
			Status:               item.Status,
			Attributes:           append([]byte(nil), item.Attributes...),
			ShortDescription:     item.ShortDescription,
			LongDescription:      item.LongDescription,
			PricingMode:          item.PricingMode,
			AvailabilityMode:     item.AvailabilityMode,
			FulfillmentMode:      item.FulfillmentMode,
			RequiresConfirmation: item.RequiresConfirmation,
			EvidenceState:        "verified",
			RetrievedAt:          now,
		})
	}

	return ports.AICapabilityResult{
		Data:            items,
		CatalogEvidence: evidence,
		HasMore:         page.HasMore,
		NextCursor:      page.NextCursor,
		Operation:       "merchant_catalog_list_items",
	}, nil
}

type getItemCapability struct {
	repository        ports.CatalogRepository
	selectedCatalogID string
}

func (c getItemCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name:        "merchant_catalog_get_item",
		Description: "Read one item by exact item_id in the already selected merchant catalog using tenant-scoped repository access.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"item_id": map[string]any{"type": "string"},
			},
			"required": []string{"item_id"},
		},
	}
}

func (c getItemCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	params, err := jsonParams(rawParams)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}
	if c.selectedCatalogID == "" {
		return ports.AICapabilityResult{}, errors.New("selected merchant catalog is required")
	}

	itemID, err := requiredString(params, "item_id")
	if err != nil {
		return ports.AICapabilityResult{}, err
	}
	item, err := c.repository.GetCatalogItem(ctx, execCtx.BusinessID, c.selectedCatalogID, itemID)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}

	now := time.Now().UTC()
	return ports.AICapabilityResult{
		Data: map[string]any{
			"id":                       item.ID,
			"catalog_id":               item.CatalogID,
			"attribute_schema_id":      item.AttributeSchemaID,
			"attribute_schema_version": item.AttributeSchemaVersion,
			"item_type":                item.ItemType,
			"name":                     item.Name,
			"short_description":        item.ShortDescription,
			"long_description":         item.LongDescription,
			"status":                   item.Status,
			"pricing_mode":             item.PricingMode,
			"availability_mode":        item.AvailabilityMode,
			"fulfillment_mode":         item.FulfillmentMode,
			"requires_confirmation":    item.RequiresConfirmation,
			"attributes":               json.RawMessage(item.Attributes),
		},
		CatalogEvidence: []ports.CustomerSalesCatalogEvidence{{
			Reference:        item.ID,
			CatalogReference: item.CatalogID,
			ItemType:         item.ItemType,
			Name:             item.Name,
			Status:           item.Status,
			Attributes:       append([]byte(nil), item.Attributes...),
			EvidenceState:    "verified",
			RetrievedAt:      now,
		}},
		Operation: "merchant_catalog_get_item",
	}, nil
}

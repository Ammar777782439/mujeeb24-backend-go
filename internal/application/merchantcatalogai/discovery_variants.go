package merchantcatalogai

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type listVariantsCapability struct {
	repository        ports.CatalogRepository
	selectedCatalogID string
}

func (c listVariantsCapability) Definition() MerchantCatalogDiscoveryToolDefinition {
	return MerchantCatalogDiscoveryToolDefinition{
		Name:        "merchant_catalog_list_variants",
		Description: "Read variants for one catalog item.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"item_id": map[string]any{"type": "string"},
				"status":  map[string]any{"type": "string"},
				"limit":   map[string]any{"type": "integer"},
				"cursor":  map[string]any{"type": "string"},
			},
			"required": []string{"item_id"},
		},
	}
}

func (c listVariantsCapability) Execute(ctx context.Context, execCtx MerchantCatalogDiscoveryExecutionContext, rawParams []byte) (MerchantCatalogDiscoveryResult, error) {
	params, err := jsonParams(rawParams)
	if err != nil {
		return MerchantCatalogDiscoveryResult{}, err
	}
	if c.selectedCatalogID == "" {
		return MerchantCatalogDiscoveryResult{}, errors.New("selected merchant catalog is required")
	}

	itemID, err := requiredString(params, "item_id")
	if err != nil {
		return MerchantCatalogDiscoveryResult{}, err
	}
	// The parent item is read and authenticated in the merchant's selected
	// catalog before listing children. Its exact DB-backed ID is evidence for
	// this turn even when the child list is empty. The caller must not need
	// another redundant get_item call to reference that verified parent.
	item, err := c.repository.GetCatalogItem(ctx, execCtx.BusinessID, c.selectedCatalogID, itemID)
	if err != nil {
		return MerchantCatalogDiscoveryResult{}, err
	}
	if item.ID != itemID || item.BusinessID != execCtx.BusinessID || item.CatalogID != c.selectedCatalogID {
		return MerchantCatalogDiscoveryResult{}, errors.New("merchant catalog parent read did not match authorized item scope")
	}

	status, _ := params["status"].(string)
	cursor, _ := params["cursor"].(string)
	page, err := c.repository.ListVariants(ctx, execCtx.BusinessID, itemID, status, readLimit(params), cursor)
	if err != nil {
		return MerchantCatalogDiscoveryResult{}, err
	}

	data := make([]map[string]any, 0, len(page.Items))
	// The parent was independently verified above, so its reference belongs
	// to the same scoped evidence set as its children.
	evidenceReferences := make([]string, 0, len(page.Items)+1)
	evidenceReferences = append(evidenceReferences, item.ID)
	for _, variant := range page.Items {
		data = append(data, map[string]any{
			"id":              variant.ID,
			"catalog_item_id": variant.CatalogItemID,
			"name":            variant.Name,
			"attributes":      json.RawMessage(variant.Attributes),
			"status":          variant.Status,
		})
		evidenceReferences = append(evidenceReferences, variant.ID)
	}

	return MerchantCatalogDiscoveryResult{
		Data:               data,
		EvidenceReferences: evidenceReferences,
		HasMore:            page.HasMore,
		NextCursor:         page.NextCursor,
		Operation:          "merchant_catalog_list_variants",
	}, nil
}

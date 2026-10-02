package merchantcatalogai

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type listVariantsCapability struct {
	repository        ports.CatalogRepository
	selectedCatalogID string
}

func (c listVariantsCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
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

func (c listVariantsCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
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
	if _, err := c.repository.GetCatalogItem(ctx, execCtx.BusinessID, c.selectedCatalogID, itemID); err != nil {
		return ports.AICapabilityResult{}, err
	}

	status, _ := params["status"].(string)
	cursor, _ := params["cursor"].(string)
	page, err := c.repository.ListVariants(ctx, execCtx.BusinessID, itemID, status, readLimit(params), cursor)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}

	now := time.Now().UTC()
	data := make([]map[string]any, 0, len(page.Items))
	evidence := make([]ports.AIVariantEvidence, 0, len(page.Items))
	for _, variant := range page.Items {
		data = append(data, map[string]any{
			"id":              variant.ID,
			"catalog_item_id": variant.CatalogItemID,
			"name":            variant.Name,
			"status":          variant.Status,
			"attributes":      json.RawMessage(variant.Attributes),
		})
		evidence = append(evidence, ports.AIVariantEvidence{
			Reference:            variant.ID,
			CatalogItemReference: variant.CatalogItemID,
			Name:                 variant.Name,
			Status:               variant.Status,
			Attributes:           append([]byte(nil), variant.Attributes...),
			EvidenceState:        "verified",
			RetrievedAt:          now,
		})
	}

	return ports.AICapabilityResult{
		Data:            data,
		VariantEvidence: evidence,
		HasMore:         page.HasMore,
		NextCursor:      page.NextCursor,
		Operation:       "merchant_catalog_list_variants",
	}, nil
}

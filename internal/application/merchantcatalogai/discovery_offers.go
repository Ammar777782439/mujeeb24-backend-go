package merchantcatalogai

import (
	"context"

	"errors"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type listOffersCapability struct {
	repository        ports.CatalogRepository
	selectedCatalogID string
}

func (c listOffersCapability) Definition() MerchantCatalogDiscoveryToolDefinition {
	return MerchantCatalogDiscoveryToolDefinition{
		Name:        "merchant_catalog_list_offers",
		Description: "Read offers for one catalog item, including factual price and availability evidence.",
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

func (c listOffersCapability) Execute(ctx context.Context, execCtx MerchantCatalogDiscoveryExecutionContext, rawParams []byte) (MerchantCatalogDiscoveryResult, error) {
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
	page, err := c.repository.ListOffers(ctx, execCtx.BusinessID, itemID, status, readLimit(params), cursor)
	if err != nil {
		return MerchantCatalogDiscoveryResult{}, err
	}

	data := make([]map[string]any, 0, len(page.Items))
	// The parent was independently verified above, so its reference belongs
	// to the same scoped evidence set as its children.
	evidenceReferences := make([]string, 0, len(page.Items)+1)
	evidenceReferences = append(evidenceReferences, item.ID)
	for _, offer := range page.Items {
		if offer.ID == "" || offer.BusinessID != execCtx.BusinessID || offer.CatalogItemID != item.ID {
			return MerchantCatalogDiscoveryResult{}, errors.New("merchant catalog read returned a child outside its verified parent item")
		}
		variantID := ""
		if offer.VariantID != nil {
			variantID = *offer.VariantID
		}

		data = append(data, map[string]any{
			"id":                        offer.ID,
			"catalog_item_id":           offer.CatalogItemID,
			"variant_id":                variantID,
			"name":                      offer.Name,
			"pricing_mode":              offer.PricingMode,
			"amount":                    offer.Amount,
			"currency":                  offer.Currency,
			"pricing_unit":              offer.PricingUnit,
			"price_source":              offer.PriceSource,
			"price_verification_status": offer.PriceVerificationStatus,
			"price_checked_at":          offer.PriceCheckedAt,
			"availability_mode":         offer.AvailabilityMode,
			"availability_status":       offer.AvailabilityStatus,
			"availability_source":       offer.AvailabilitySource,
			"availability_checked_at":   offer.AvailabilityCheckedAt,
			"availability_valid_until":  offer.AvailabilityValidUntil,
			"availability_evidence_ref": offer.AvailabilityEvidenceRef,
			"fulfillment_mode":          offer.FulfillmentMode,
			"validity_from":             offer.ValidityFrom,
			"validity_until":            offer.ValidityUntil,
			"status":                    offer.Status,
		})
		evidenceReferences = append(evidenceReferences, offer.ID)
	}

	return MerchantCatalogDiscoveryResult{
		Data:               data,
		EvidenceReferences: evidenceReferences,
		HasMore:            page.HasMore,
		NextCursor:         page.NextCursor,
		Operation:          "merchant_catalog_list_offers",
	}, nil
}

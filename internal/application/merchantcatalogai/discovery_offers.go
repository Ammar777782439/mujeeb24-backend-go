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
	if _, err := c.repository.GetCatalogItem(ctx, execCtx.BusinessID, c.selectedCatalogID, itemID); err != nil {
		return MerchantCatalogDiscoveryResult{}, err
	}

	status, _ := params["status"].(string)
	cursor, _ := params["cursor"].(string)
	page, err := c.repository.ListOffers(ctx, execCtx.BusinessID, itemID, status, readLimit(params), cursor)
	if err != nil {
		return MerchantCatalogDiscoveryResult{}, err
	}

	data := make([]map[string]any, 0, len(page.Items))
	evidenceReferences := make([]string, 0, len(page.Items))
	for _, offer := range page.Items {
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
		Data:          data,
		OfferEvidence: evidence,
		HasMore:       page.HasMore,
		NextCursor:    page.NextCursor,
		Operation:     "merchant_catalog_list_offers",
	}, nil
}

func dereferenceString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

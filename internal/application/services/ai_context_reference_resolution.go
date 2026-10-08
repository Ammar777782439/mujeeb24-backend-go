package services

import (
	"context"
	"errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"strings"
)

// Resolve exact references already present in validated conversation state.
// These functions do not select products from customer message text.
func (b AutoReplyContextBuilder) findItemByIDWithinBusiness(ctx context.Context, businessID, itemID string, catalogID *string) (ports.CatalogItemRecord, error) {
	if catalogID != nil && strings.TrimSpace(*catalogID) != "" {
		item, err := b.Catalogs.GetCatalogItem(ctx, businessID, *catalogID, itemID)
		if err != nil {
			return ports.CatalogItemRecord{}, err
		}
		if item.BusinessID != businessID {
			return ports.CatalogItemRecord{}, errors.New("AI context catalog item scope mismatch")
		}
		return item, nil
	}
	catalogs, err := b.Catalogs.ListCatalogs(ctx, businessID, "active", b.maxCatalogs(), "")
	if err != nil {
		return ports.CatalogItemRecord{}, err
	}
	for _, catalog := range catalogs.Items {
		if catalog.BusinessID != businessID {
			continue
		}
		item, err := b.Catalogs.GetCatalogItem(ctx, businessID, catalog.ID, itemID)
		if err == nil {
			return item, nil
		}
		if !isScopedNotFound(err) {
			// GetCatalogItem returns not_found for wrong catalog; continue scanning.
			continue
		}
	}
	return ports.CatalogItemRecord{}, &scopedNotFoundError{msg: "item not found within business"}
}

func (b AutoReplyContextBuilder) findOfferByIDWithinBusiness(ctx context.Context, businessID, offerID string, itemID *string) (ports.CatalogRecord, ports.CatalogItemRecord, ports.OfferRecord, error) {
	if itemID != nil && strings.TrimSpace(*itemID) != "" {
		offers, err := b.Catalogs.ListOffers(ctx, businessID, *itemID, "active", b.maxOffers(), "")
		if err != nil {
			return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, err
		}
		for _, offer := range offers.Items {
			if offer.ID == offerID {
				if offer.BusinessID != businessID {
					return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, errors.New("AI context offer scope mismatch")
				}
				item, err := b.findItemByIDWithinBusiness(ctx, businessID, *itemID, nil)
				if err != nil {
					return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, err
				}
				catalog, err := b.Catalogs.GetCatalog(ctx, businessID, item.CatalogID)
				if err != nil {
					return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, err
				}
				return catalog, item, offer, nil
			}
		}
		return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, &scopedNotFoundError{msg: "offer not found for item"}
	}
	catalogs, err := b.Catalogs.ListCatalogs(ctx, businessID, "active", b.maxCatalogs(), "")
	if err != nil {
		return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, err
	}
	for _, catalog := range catalogs.Items {
		if catalog.BusinessID != businessID {
			continue
		}
		items, err := b.Catalogs.ListCatalogItems(ctx, businessID, catalog.ID, "", "active", b.maxItems()*3, "")
		if err != nil {
			return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, err
		}
		for _, item := range items.Items {
			offers, err := b.Catalogs.ListOffers(ctx, businessID, item.ID, "active", b.maxOffers(), "")
			if err != nil {
				return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, err
			}
			for _, offer := range offers.Items {
				if offer.ID == offerID {
					return catalog, item, offer, nil
				}
			}
		}
	}
	return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, &scopedNotFoundError{msg: "offer not found within business"}
}

type scopedNotFoundError struct{ msg string }

func (e *scopedNotFoundError) Error() string     { return "scoped retrieval not found: " + e.msg }
func (e *scopedNotFoundError) ErrorKind() string { return "not_found" }

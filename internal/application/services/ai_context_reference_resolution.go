package services

import (
	"context"
	"errors"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// Resolve exact references already present in validated conversation state.
// Never scan first-N catalog/offer pages hoping to find a reference, and never
// derive a reference from keywords in the customer's message.
func (b AutoReplyContextBuilder) findItemByIDWithinBusiness(ctx context.Context, businessID, itemID string, catalogID *string) (ports.CatalogItemRecord, error) {
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(itemID) == "" {
		return ports.CatalogItemRecord{}, errors.New("business and catalog item ID are required")
	}
	var item ports.CatalogItemRecord
	var err error
	if catalogID != nil && strings.TrimSpace(*catalogID) != "" {
		item, err = b.Catalogs.GetCatalogItem(ctx, businessID, *catalogID, itemID)
	} else {
		item, err = b.Catalogs.GetCatalogItemByID(ctx, businessID, itemID)
	}
	if err != nil {
		return ports.CatalogItemRecord{}, err
	}
	if item.BusinessID != businessID || item.ID != itemID {
		return ports.CatalogItemRecord{}, errors.New("AI context catalog item scope mismatch")
	}
	if catalogID != nil && strings.TrimSpace(*catalogID) != "" && item.CatalogID != *catalogID {
		return ports.CatalogItemRecord{}, errors.New("AI context item catalog mismatch")
	}
	if item.Status != "active" {
		return ports.CatalogItemRecord{}, &scopedNotFoundError{msg: "focused catalog item is inactive"}
	}
	catalog, err := b.Catalogs.GetCatalog(ctx, businessID, item.CatalogID)
	if err != nil {
		return ports.CatalogItemRecord{}, err
	}
	if catalog.BusinessID != businessID || catalog.ID != item.CatalogID {
		return ports.CatalogItemRecord{}, errors.New("AI context catalog scope mismatch")
	}
	if catalog.Status != "active" {
		return ports.CatalogItemRecord{}, &scopedNotFoundError{msg: "focused catalog is inactive"}
	}
	return item, nil
}

func (b AutoReplyContextBuilder) findOfferByIDWithinBusiness(ctx context.Context, businessID, offerID string, itemID *string) (ports.CatalogRecord, ports.CatalogItemRecord, ports.OfferRecord, error) {
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(offerID) == "" {
		return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, errors.New("business and offer ID are required")
	}
	// The existing tenant-scoped GetOffer is an exact read. A focused offer
	// beyond the first page is not a reason to broaden AI evidence.
	offer, err := b.Catalogs.GetOffer(ctx, businessID, offerID)
	if err != nil {
		return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, err
	}
	if offer.ID != offerID || offer.BusinessID != businessID {
		return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, errors.New("AI context offer scope mismatch")
	}
	if offer.Status != "active" {
		return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, &scopedNotFoundError{msg: "focused offer is inactive"}
	}
	if itemID != nil && strings.TrimSpace(*itemID) != "" && offer.CatalogItemID != *itemID {
		return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, errors.New("focused offer does not belong to referenced item")
	}
	item, err := b.findItemByIDWithinBusiness(ctx, businessID, offer.CatalogItemID, nil)
	if err != nil {
		return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, err
	}
	catalog, err := b.Catalogs.GetCatalog(ctx, businessID, item.CatalogID)
	if err != nil {
		return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, err
	}
	if catalog.BusinessID != businessID || catalog.ID != item.CatalogID || catalog.Status != "active" {
		return ports.CatalogRecord{}, ports.CatalogItemRecord{}, ports.OfferRecord{}, errors.New("AI context offer catalog scope mismatch")
	}
	return catalog, item, offer, nil
}

type scopedNotFoundError struct{ msg string }

func (e *scopedNotFoundError) Error() string     { return "scoped retrieval not found: " + e.msg }
func (e *scopedNotFoundError) ErrorKind() string { return "not_found" }

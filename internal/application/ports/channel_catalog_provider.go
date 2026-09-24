package ports

import (
	"context"
)

// ExternalCatalogProduct represents a product fetched from an external channel catalog (e.g. WhatsApp Business / Meta Commerce).
type ExternalCatalogProduct struct {
	ExternalID   string
	Name         string
	Description  string
	PriceAmount  *float64
	Currency     string
	Availability string // "in_stock" | "out_of_stock"
	ImageURL     string
	RetailerID   string
}

// ChannelCatalogProvider defines the interface for fetching catalogs from external channel providers.
type ChannelCatalogProvider interface {
	FetchChannelCatalog(ctx context.Context, connectionID string) ([]ExternalCatalogProduct, error)
}

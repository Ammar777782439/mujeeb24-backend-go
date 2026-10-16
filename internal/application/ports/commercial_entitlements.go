package ports

import (
	"context"
	"time"
)

// CatalogActivationEntitlement serializes and validates transitions into the
// AI-active product scope. Storage remains unlimited; only active products
// consume the subscription allowance.
type CatalogActivationEntitlement interface {
	EnsureCatalogItemActivation(ctx context.Context, businessID, itemID string, now time.Time) error
}

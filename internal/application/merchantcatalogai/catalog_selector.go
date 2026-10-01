package merchantcatalogai

import (
	"context"
	"errors"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

var ErrCatalogSelectionRequired = errors.New("merchant catalog selection is required")

type DeterministicCatalogSelector struct {
	Repository ports.CatalogRepository
}

func (s DeterministicCatalogSelector) Select(ctx context.Context, in CatalogSelectionInput) (CatalogSelection, error) {
	if s.Repository == nil {
		return CatalogSelection{}, errors.New("catalog selector repository is not configured")
	}
	businessID := strings.TrimSpace(in.BusinessID)
	if businessID == "" {
		return CatalogSelection{}, errors.New("business_id is required")
	}
	if id := strings.TrimSpace(in.ExplicitCatalogID); id != "" {
		catalog, err := s.Repository.GetCatalog(ctx, businessID, id)
		if err != nil {
			return CatalogSelection{}, err
		}
		return CatalogSelection{Catalog: catalog, Reason: "explicit"}, nil
	}
	if id := strings.TrimSpace(in.StickyCatalogID); id != "" {
		catalog, err := s.Repository.GetCatalog(ctx, businessID, id)
		if err == nil {
			return CatalogSelection{Catalog: catalog, Reason: "session_sticky"}, nil
		}
	}
	page, err := s.Repository.ListCatalogs(ctx, businessID, "", 2, "")
	if err != nil {
		return CatalogSelection{}, err
	}
	if len(page.Items) == 1 && !page.HasMore {
		return CatalogSelection{Catalog: page.Items[0], Reason: "single_catalog_auto_select"}, nil
	}
	return CatalogSelection{}, ErrCatalogSelectionRequired
}

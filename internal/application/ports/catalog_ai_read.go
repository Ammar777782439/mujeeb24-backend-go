package ports

import "context"

// CatalogAIManifest is the bounded, whole-catalog map sent to the AI on normal
// turns. It describes the merchant's active catalog shape without serializing
// every item.
type CatalogAIManifest struct {
	TotalActiveItems int                         `json:"total_active_items"`
	Catalogs         []CatalogAIManifestCatalog `json:"catalogs"`
	Schemas          []CatalogAIManifestSchema  `json:"schemas,omitempty"`
}

type CatalogAIManifestCatalog struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description *string  `json:"description,omitempty"`
	ItemCount   int      `json:"item_count"`
	ItemTypes   []string `json:"item_types,omitempty"`
}

type CatalogAIManifestSchema struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Version       int      `json:"version"`
	UsageCount    int      `json:"usage_count"`
	AttributeKeys []string `json:"attribute_keys,omitempty"`
}

// CatalogAIProjectionBundle is one fully-hydrated catalog item.
// All nested records belong to Item and the same business.
type CatalogAIProjectionBundle struct {
	Item            CatalogItemRecord
	Variants        []VariantRecord
	Offers          []OfferRecord
	AttributeSchema *AttributeSchemaRecord
}

type CatalogAIProjectionPage struct {
	Catalogs   []CatalogRecord
	Items      []CatalogAIProjectionBundle
	NextCursor string
	HasMore    bool
}
type CatalogAIProjectionRequest struct {
	BusinessID string
	CatalogID  string
	Limit      int
	Cursor     string
}

// CatalogAIReadRepository is the read-optimized catalog boundary used by AI.
// It is read-only, tenant-scoped and returns structured records only.
type CatalogAIReadRepository interface {
	GetManifest(ctx context.Context, businessID string) (CatalogAIManifest, error)
	ListProjectionPage(ctx context.Context, request CatalogAIProjectionRequest) (CatalogAIProjectionPage, error)
}

// CatalogAIEvidenceSet is the exact relational evidence actually exposed to
// the model. It is deliberately not a flat list of independently valid IDs.
type CatalogAIEvidenceSet struct {
	Items map[string]CatalogAIEvidenceItem
}

type CatalogAIEvidenceItem struct {
	Variants map[string]struct{}
	Offers   map[string]CatalogAIEvidenceOffer
}

type CatalogAIEvidenceOffer struct {
	VariantID string
}

func NewCatalogAIEvidenceSet() CatalogAIEvidenceSet {
	return CatalogAIEvidenceSet{Items: make(map[string]CatalogAIEvidenceItem)}
}

func (e *CatalogAIEvidenceSet) AddBundle(bundle CatalogAIProjectionBundle) {
	if e.Items == nil {
		e.Items = make(map[string]CatalogAIEvidenceItem)
	}
	item := CatalogAIEvidenceItem{
		Variants: make(map[string]struct{}),
		Offers:   make(map[string]CatalogAIEvidenceOffer),
	}
	if existing, ok := e.Items[bundle.Item.ID]; ok {
		item = existing
		if item.Variants == nil {
			item.Variants = make(map[string]struct{})
		}
		if item.Offers == nil {
			item.Offers = make(map[string]CatalogAIEvidenceOffer)
		}
	}
	for _, variant := range bundle.Variants {
		if variant.CatalogItemID == bundle.Item.ID {
			item.Variants[variant.ID] = struct{}{}
		}
	}
	for _, offer := range bundle.Offers {
		if offer.CatalogItemID != bundle.Item.ID {
			continue
		}
		var variantID string
		if offer.VariantID != nil {
			variantID = *offer.VariantID
		}
		item.Offers[offer.ID] = CatalogAIEvidenceOffer{VariantID: variantID}
	}
	e.Items[bundle.Item.ID] = item
}

func (e CatalogAIEvidenceSet) ContainsSelection(ref SelectedReference) bool {
	item, ok := e.Items[ref.ItemID]
	if !ok {
		return false
	}
	var selectedVariant string
	if ref.VariantID != nil {
		selectedVariant = *ref.VariantID
		if selectedVariant != "" {
			if _, ok := item.Variants[selectedVariant]; !ok {
				return false
			}
		}
	}
	if ref.OfferID != nil && *ref.OfferID != "" {
		offer, ok := item.Offers[*ref.OfferID]
		if !ok {
			return false
		}
		if selectedVariant != "" && offer.VariantID != "" && offer.VariantID != selectedVariant {
			return false
		}
	}
	return true
}

func (e *CatalogAIEvidenceSet) Merge(other CatalogAIEvidenceSet) {
	if e.Items == nil {
		e.Items = make(map[string]CatalogAIEvidenceItem)
	}
	for itemID, otherItem := range other.Items {
		item := e.Items[itemID]
		if item.Variants == nil {
			item.Variants = make(map[string]struct{})
		}
		if item.Offers == nil {
			item.Offers = make(map[string]CatalogAIEvidenceOffer)
		}
		for variantID := range otherItem.Variants {
			item.Variants[variantID] = struct{}{}
		}
		for offerID, offer := range otherItem.Offers {
			item.Offers[offerID] = offer
		}
		e.Items[itemID] = item
	}
}

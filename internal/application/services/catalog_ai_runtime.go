package services

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// ProjectionFromBundles converts the read-optimized repository result into the
// provider-neutral Catalog AI Projection. Schemas are deduplicated.
func ProjectionFromBundles(bundles []ports.CatalogAIProjectionBundle) CatalogAIProjection {
	projection := CatalogAIProjection{}
	schemas := map[string]CatalogAIAttributeSchema{}

	for _, bundle := range bundles {
		item := mapCatalogItemRecordToProjection(bundle.Item)
		item.Variants = make([]CatalogAIVariant, 0, len(bundle.Variants))
		for _, variant := range bundle.Variants {
			item.Variants = append(item.Variants, CatalogAIVariant{
				ID:         variant.ID,
				Name:       variant.Name,
				Attributes: parseJSONAttributes(variant.Attributes),
				Status:     variant.Status,
			})
		}
		item.Offers = make([]CatalogAIOffer, 0, len(bundle.Offers))
		for _, offer := range bundle.Offers {
			item.Offers = append(item.Offers, CatalogAIOffer{
				ID:                      offer.ID,
				VariantID:               offer.VariantID,
				Name:                    offer.Name,
				PricingMode:             offer.PricingMode,
				Amount:                  offer.Amount,
				Currency:                offer.Currency,
				PricingUnit:             offer.PricingUnit,
				PriceSource:             offer.PriceSource,
				PriceVerificationStatus: stringPtrOrNil(offer.PriceVerificationStatus),
				PriceCheckedAt:          formatTimePtr(offer.PriceCheckedAt),
				AvailabilityMode:        stringPtrOrNil(offer.AvailabilityMode),
				AvailabilityStatus:      stringPtrOrNil(offer.AvailabilityStatus),
				AvailabilitySource:      offer.AvailabilitySource,
				AvailabilityCheckedAt:   formatTimePtr(offer.AvailabilityCheckedAt),
				AvailabilityValidUntil:  formatTimePtr(offer.AvailabilityValidUntil),
				AvailabilityEvidenceRef: offer.AvailabilityEvidenceRef,
				FulfillmentMode:         stringPtrOrNil(offer.FulfillmentMode),
				ValidityFrom:            formatTimePtr(offer.ValidityFrom),
				ValidityUntil:           formatTimePtr(offer.ValidityUntil),
				Status:                  offer.Status,
			})
		}
		projection.Items = append(projection.Items, item)

		if bundle.AttributeSchema != nil {
			schemas[bundle.AttributeSchema.ID] = mapAttributeSchemaRecordToProjection(*bundle.AttributeSchema)
		}
	}

	schemaIDs := make([]string, 0, len(schemas))
	for id := range schemas {
		schemaIDs = append(schemaIDs, id)
	}
	sort.Strings(schemaIDs)
	for _, id := range schemaIDs {
		projection.AttributeSchemas = append(projection.AttributeSchemas, schemas[id])
	}
	return projection
}

func EvidenceFromBundles(bundles []ports.CatalogAIProjectionBundle) ports.CatalogAIEvidenceSet {
	evidence := ports.NewCatalogAIEvidenceSet()
	for _, bundle := range bundles {
		evidence.AddBundle(bundle)
	}
	return evidence
}

func EvidenceFromCustomerSalesContext(ctx *ports.CustomerSalesContext) ports.CatalogAIEvidenceSet {
	evidence := ports.NewCatalogAIEvidenceSet()
	if ctx == nil {
		return evidence
	}
	for _, item := range ctx.CatalogEvidence {
		evidence.Items[item.Reference] = ports.CatalogAIEvidenceItem{
			Variants: map[string]struct{}{},
			Offers:   map[string]ports.CatalogAIEvidenceOffer{},
		}
	}
	for _, variant := range ctx.VariantEvidence {
		item, ok := evidence.Items[variant.CatalogItemReference]
		if !ok {
			continue
		}
		item.Variants[variant.Reference] = struct{}{}
		evidence.Items[variant.CatalogItemReference] = item
	}
	for _, offer := range ctx.OfferEvidence {
		item, ok := evidence.Items[offer.CatalogItemReference]
		if !ok {
			continue
		}
		item.Offers[offer.Reference] = ports.CatalogAIEvidenceOffer{VariantID: offer.VariantReference}
		evidence.Items[offer.CatalogItemReference] = item
	}
	return evidence
}

func ParseManifestValidationRules(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}

func mapCatalogItemRecordToProjection(item ports.CatalogItemRecord) CatalogAIItem {
	return CatalogAIItem{
		ID:                     item.ID,
		CatalogID:              item.CatalogID,
		AttributeSchemaID:      item.AttributeSchemaID,
		AttributeSchemaVersion: item.AttributeSchemaVersion,
		ItemType:               item.ItemType,
		Name:                   item.Name,
		ShortDescription:       item.ShortDescription,
		LongDescription:        item.LongDescription,
		Status:                 item.Status,
		PricingMode:            item.PricingMode,
		AvailabilityMode:       item.AvailabilityMode,
		FulfillmentMode:        item.FulfillmentMode,
		RequiresConfirmation:   item.RequiresConfirmation,
		Attributes:             parseJSONAttributes(item.Attributes),
	}
}

func mapAttributeSchemaRecordToProjection(schema ports.AttributeSchemaRecord) CatalogAIAttributeSchema {
	out := CatalogAIAttributeSchema{ID: schema.ID, Name: schema.Name, Version: schema.Version}
	out.Definitions = make([]CatalogAIAttributeDefinition, 0, len(schema.Definitions))
	for _, def := range schema.Definitions {
		out.Definitions = append(out.Definitions, CatalogAIAttributeDefinition{
			ID:              def.ID,
			SchemaID:        schema.ID,
			AttributeKey:    def.Key,
			Label:           def.Label,
			DataType:        def.DataType,
			IsRequired:      def.Required,
			IsSearchable:    def.Searchable,
			ValidationRules: ParseManifestValidationRules(def.ValidationRules),
			DisplayOrder:    def.DisplayOrder,
		})
	}
	return out
}

func parseJSONAttributes(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func stringPtrOrNil(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	v := value
	return &v
}

func formatTimePtr(value *time.Time) *string {
	if value == nil {
		return nil
	}
	v := value.UTC().Format(time.RFC3339Nano)
	return &v
}

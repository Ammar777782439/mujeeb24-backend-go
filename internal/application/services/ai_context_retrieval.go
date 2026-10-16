package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type retrievalMode string

const (
	retrievalBroader          retrievalMode = "broader"
	retrievalScopedOffer      retrievalMode = "scoped_offer"
	retrievalScopedItem       retrievalMode = "scoped_item"
	retrievalScopedCatalog    retrievalMode = "scoped_catalog"
	retrievalScopedVariant    retrievalMode = "scoped_variant"
	retrievalScopedComparison retrievalMode = "scoped_comparison"
)

func resolveRetrievalMode(state *ports.ConversationStateRecord) (retrievalMode, *ports.ConversationFocus, *ports.ConversationComparison) {
	if state == nil {
		return retrievalBroader, nil, nil
	}
	if state.Comparison != nil && len(state.Comparison.IDs) >= 2 {
		return retrievalScopedComparison, nil, state.Comparison
	}
	focus := state.Focus
	if focus == nil || strings.TrimSpace(focus.ID) == "" || strings.TrimSpace(focus.Type) == "" {
		return retrievalBroader, nil, nil
	}
	switch normalizeFocusType(focus.Type) {
	case "offer":
		return retrievalScopedOffer, focus, nil
	case "item":
		return retrievalScopedItem, focus, nil
	case "catalog":
		return retrievalScopedCatalog, focus, nil
	case "variant":
		return retrievalScopedVariant, focus, nil
	default:
		return retrievalBroader, nil, nil
	}
}

func normalizeFocusType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "service", "booking", "property", "trip", "package", "plan":
		return "item"
	default:
		return t
	}
}

func isScopedNotFound(err error) bool {
	if err == nil {
		return false
	}
	var kinded interface{ ErrorKind() string }
	if errors.As(err, &kinded) {
		return kinded.ErrorKind() == "not_found"
	}
	return false
}

func (b AutoReplyContextBuilder) retrieveScoped(ctx context.Context, businessID string, focus *ports.ConversationFocus, _ *ports.ConversationComparison, now time.Time) ([]ports.CustomerSalesCatalogEvidence, []ports.CustomerSalesOfferEvidence, []ports.CustomerSalesVariantEvidence, error) {
	if focus == nil {
		return nil, nil, nil, errors.New("missing focus")
	}
	switch normalizeFocusType(focus.Type) {
	case "offer":
		return b.retrieveScopedOffer(ctx, businessID, focus, now)
	case "item":
		return b.retrieveScopedItem(ctx, businessID, focus, now)
	case "catalog":
		return b.retrieveScopedCatalog(ctx, businessID, focus, now)
	case "variant":
		return b.retrieveScopedVariant(ctx, businessID, focus, now)
	default:
		return nil, nil, nil, errors.New("unsupported focus type")
	}
}

// retrieveScopedOffer always attaches the complete parent entity. An individual
// offer is a selection, never permission to omit sibling commercial facts.
func (b AutoReplyContextBuilder) retrieveScopedOffer(ctx context.Context, businessID string, focus *ports.ConversationFocus, now time.Time) ([]ports.CustomerSalesCatalogEvidence, []ports.CustomerSalesOfferEvidence, []ports.CustomerSalesVariantEvidence, error) {
	catalog, item, offer, err := b.findOfferByIDWithinBusiness(ctx, businessID, focus.ID, focus.ItemID)
	if err != nil {
		return nil, nil, nil, err
	}
	if catalog.BusinessID != businessID || item.BusinessID != businessID || offer.BusinessID != businessID {
		return nil, nil, nil, errors.New("AI context offer scope mismatch")
	}
	items, offers, variants, err := b.retrieveCompleteItemEvidence(ctx, businessID, item, now)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, evidence := range offers {
		if evidence.Reference == offer.ID {
			return items, offers, variants, nil
		}
	}
	return nil, nil, nil, errors.New("focused offer is not part of the complete active commercial entity")
}

func (b AutoReplyContextBuilder) retrieveScopedItem(ctx context.Context, businessID string, focus *ports.ConversationFocus, now time.Time) ([]ports.CustomerSalesCatalogEvidence, []ports.CustomerSalesOfferEvidence, []ports.CustomerSalesVariantEvidence, error) {
	item, err := b.findItemByIDWithinBusiness(ctx, businessID, focus.ID, focus.CatalogID)
	if err != nil {
		return nil, nil, nil, err
	}
	return b.retrieveCompleteItemEvidence(ctx, businessID, item, now)
}

// A catalog focus is not an item. Never expose a bounded prefix as if it
// represented a complete commercial catalog: the model must request
// evaluate_full_catalog using the manifest if item-level truth is required.
func (b AutoReplyContextBuilder) retrieveScopedCatalog(ctx context.Context, businessID string, focus *ports.ConversationFocus, now time.Time) ([]ports.CustomerSalesCatalogEvidence, []ports.CustomerSalesOfferEvidence, []ports.CustomerSalesVariantEvidence, error) {
	catalog, err := b.Catalogs.GetCatalog(ctx, businessID, focus.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	if catalog.BusinessID != businessID {
		return nil, nil, nil, errors.New("AI context catalog scope mismatch")
	}
	return nil, nil, nil, &scopedNotFoundError{msg: "catalog focus requires evaluate_full_catalog; a prefix is not complete evidence"}
}

func (b AutoReplyContextBuilder) retrieveScopedVariant(ctx context.Context, businessID string, focus *ports.ConversationFocus, now time.Time) ([]ports.CustomerSalesCatalogEvidence, []ports.CustomerSalesOfferEvidence, []ports.CustomerSalesVariantEvidence, error) {
	if focus.ItemID == nil || strings.TrimSpace(*focus.ItemID) == "" {
		return nil, nil, nil, &scopedNotFoundError{msg: "variant focus missing item"}
	}
	item, err := b.findItemByIDWithinBusiness(ctx, businessID, *focus.ItemID, focus.CatalogID)
	if err != nil {
		return nil, nil, nil, err
	}
	items, offers, variants, err := b.retrieveCompleteItemEvidence(ctx, businessID, item, now)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, variant := range variants {
		if variant.Reference == focus.ID {
			return items, offers, variants, nil
		}
	}
	return nil, nil, nil, &scopedNotFoundError{msg: "focused variant not in active entity"}
}

func (b AutoReplyContextBuilder) retrieveComparison(ctx context.Context, businessID string, comparison *ports.ConversationComparison, now time.Time) ([]ports.CustomerSalesCatalogEvidence, []ports.CustomerSalesOfferEvidence, []ports.CustomerSalesVariantEvidence, error) {
	if comparison == nil || len(comparison.IDs) < 2 {
		return nil, nil, nil, &scopedNotFoundError{msg: "invalid comparison"}
	}
	var catalogEvidence []ports.CustomerSalesCatalogEvidence
	var offerEvidence []ports.CustomerSalesOfferEvidence
	var variantEvidence []ports.CustomerSalesVariantEvidence
	seenItems := map[string]bool{}
	for _, id := range comparison.IDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		_, item, _, err := b.findOfferByIDWithinBusiness(ctx, businessID, id, nil)
		if err != nil {
			// An exact reference can identify an item or an offer. Only a
			// verified not-found may try the other type; database errors
			// and cross-tenant mismatches must never be masked.
			if !isScopedNotFound(err) {
				return nil, nil, nil, err
			}
			item, err = b.findItemByIDWithinBusiness(ctx, businessID, id, nil)
			if err != nil {
				return nil, nil, nil, err
			}
		}
		if seenItems[item.ID] {
			continue
		}
		seenItems[item.ID] = true
		items, offers, variants, err := b.retrieveCompleteItemEvidence(ctx, businessID, item, now)
		if err != nil {
			return nil, nil, nil, err
		}
		catalogEvidence = append(catalogEvidence, items...)
		offerEvidence = append(offerEvidence, offers...)
		variantEvidence = append(variantEvidence, variants...)
	}
	if len(catalogEvidence) == 0 {
		return nil, nil, nil, &scopedNotFoundError{msg: "comparison yielded no complete evidence"}
	}
	return catalogEvidence, offerEvidence, variantEvidence, nil
}

// retrieveCompleteItemEvidence reads ALL active variants and offers by cursor,
// never the first maxVariants/maxOffers rows. A broken cursor, missing nested
// relationship or cross-tenant record aborts the entire entity.
func (b AutoReplyContextBuilder) retrieveCompleteItemEvidence(ctx context.Context, businessID string, item ports.CatalogItemRecord, now time.Time) ([]ports.CustomerSalesCatalogEvidence, []ports.CustomerSalesOfferEvidence, []ports.CustomerSalesVariantEvidence, error) {
	if item.BusinessID != businessID {
		return nil, nil, nil, errors.New("AI context item scope mismatch")
	}
	const pageLimit = 200
	var variantEvidence []ports.CustomerSalesVariantEvidence
	var offerEvidence []ports.CustomerSalesOfferEvidence
	variantIDs := make(map[string]bool)
	offerIDs := make(map[string]bool)
	cursor := ""
	for {
		page, err := b.Catalogs.ListVariants(ctx, businessID, item.ID, "active", pageLimit, cursor)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, variant := range page.Items {
			if variant.BusinessID != businessID || variant.CatalogItemID != item.ID || variantIDs[variant.ID] {
				return nil, nil, nil, errors.New("incomplete or duplicate commercial variant evidence")
			}
			variantIDs[variant.ID] = true
			variantEvidence = append(variantEvidence, ports.CustomerSalesVariantEvidence{
				Reference: variant.ID, CatalogItemReference: variant.CatalogItemID, Name: variant.Name, Status: variant.Status,
				Attributes: safeJSONObject(variant.Attributes), EvidenceState: CustomerSalesContextFresh, RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
			})
		}
		if !page.HasMore {
			break
		}
		if page.NextCursor == "" || page.NextCursor == cursor {
			return nil, nil, nil, errors.New("commercial variant cursor did not advance")
		}
		cursor = page.NextCursor
	}
	cursor = ""
	for {
		page, err := b.Catalogs.ListOffers(ctx, businessID, item.ID, "active", pageLimit, cursor)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, offer := range page.Items {
			if offer.BusinessID != businessID || offer.CatalogItemID != item.ID || offerIDs[offer.ID] {
				return nil, nil, nil, errors.New("incomplete or duplicate commercial offer evidence")
			}
			if offer.VariantID != nil && *offer.VariantID != "" && !variantIDs[*offer.VariantID] {
				return nil, nil, nil, errors.New("active offer references missing or inactive commercial variant")
			}
			offerIDs[offer.ID] = true
			offerEvidence = append(offerEvidence, toOfferEvidence(offer, now))
		}
		if !page.HasMore {
			break
		}
		if page.NextCursor == "" || page.NextCursor == cursor {
			return nil, nil, nil, errors.New("commercial offer cursor did not advance")
		}
		cursor = page.NextCursor
	}
	return []ports.CustomerSalesCatalogEvidence{catalogItemEvidence(item, now)}, offerEvidence, variantEvidence, nil
}

// toOfferEvidence projects a repository offer into AI evidence with a
// single field mapping, so scoped, item-based, and comparison paths cannot
// drift apart.
func toOfferEvidence(offer ports.OfferRecord, now time.Time) ports.CustomerSalesOfferEvidence {
	return ports.CustomerSalesOfferEvidence{
		Reference:               offer.ID,
		CatalogItemReference:    offer.CatalogItemID,
		VariantReference:        stringValue(offer.VariantID),
		Name:                    offer.Name,
		PricingMode:             offer.PricingMode,
		Amount:                  stringValue(offer.Amount),
		Currency:                stringValue(offer.Currency),
		PricingUnit:             stringValue(offer.PricingUnit),
		PriceSource:             stringValue(offer.PriceSource),
		PriceVerificationStatus: offer.PriceVerificationStatus,
		PriceCheckedAt:          offer.PriceCheckedAt,
		AvailabilityMode:        offer.AvailabilityMode,
		AvailabilityStatus:      offer.AvailabilityStatus,
		AvailabilitySource:      stringValue(offer.AvailabilitySource),
		AvailabilityCheckedAt:   offer.AvailabilityCheckedAt,
		AvailabilityValidUntil:  offer.AvailabilityValidUntil,
		AvailabilityEvidenceRef: stringValue(offer.AvailabilityEvidenceRef),
		FulfillmentMode:         offer.FulfillmentMode,
		ValidityFrom:            offer.ValidityFrom,
		ValidityUntil:           offer.ValidityUntil,
		Status:                  offer.Status,
		EvidenceState:           offerEvidenceStateForRecord(offer, now),
		RetrievedAt:             now,
		SchemaVersion:           AIEvidenceSchemaVersion,
	}
}

func offerEvidenceStateForRecord(offer ports.OfferRecord, now time.Time) string {
	status := strings.ToLower(strings.TrimSpace(offer.AvailabilityStatus))
	verification := strings.ToLower(strings.TrimSpace(offer.PriceVerificationStatus))
	if status == "unknown" || status == "stale" || status == "requires_check" ||
		verification == "stale" || verification == "unverified" || verification == "rejected" {
		return CustomerSalesContextStale
	}
	if offer.AvailabilityValidUntil != nil && now.After(*offer.AvailabilityValidUntil) {
		return CustomerSalesContextStale
	}
	if offer.ValidityFrom != nil && now.Before(*offer.ValidityFrom) {
		return CustomerSalesContextStale
	}
	if offer.ValidityUntil != nil && now.After(*offer.ValidityUntil) {
		return CustomerSalesContextStale
	}
	return CustomerSalesContextFresh
}

func customerSalesSchemaEvidence(schema ports.AttributeSchemaRecord) ports.CustomerSalesCatalogSchemaEvidence {
	out := ports.CustomerSalesCatalogSchemaEvidence{
		ID:          schema.ID,
		Name:        schema.Name,
		Version:     schema.Version,
		Definitions: make([]ports.CustomerSalesAttributeDefinitionEvidence, 0, len(schema.Definitions)),
	}
	for _, def := range schema.Definitions {
		out.Definitions = append(out.Definitions, ports.CustomerSalesAttributeDefinitionEvidence{
			ID:              def.ID,
			SchemaID:        schema.ID,
			AttributeKey:    def.Key,
			Label:           def.Label,
			DataType:        def.DataType,
			IsRequired:      def.Required,
			ValidationRules: parseJSONAttributes(def.ValidationRules),
			DisplayOrder:    def.DisplayOrder,
		})
	}
	return out
}
func (b AutoReplyContextBuilder) finalizeContext(ctx context.Context, base ports.CustomerSalesContext, input ports.CustomerSalesContextInput, now time.Time) (ports.CustomerSalesContext, error) {
	if b.Sales != nil {
		sales, err := b.Sales.GetContext(ctx, input.BusinessID, input.ConversationID, base.Customer.Reference)
		if err != nil {
			return ports.CustomerSalesContext{}, err
		}
		base.Sales = &sales
	}
	// The manifest describes catalog shape only. Item-level evaluation is
	// delegated to the complete catalog paging/batching path when required.
	if b.CatalogAI != nil && base.CatalogManifest == nil {
		manifest, manifestErr := b.CatalogAI.GetManifest(ctx, input.BusinessID)
		if manifestErr != nil {
			return ports.CustomerSalesContext{}, manifestErr
		}
		base.CatalogManifest = &manifest
	}

	if len(base.CatalogEvidence) > 0 && len(base.CatalogSchemaEvidence) == 0 {
		seenSchemas := make(map[string]struct{})
		for _, item := range base.CatalogEvidence {
			if item.AttributeSchemaReference == nil || strings.TrimSpace(*item.AttributeSchemaReference) == "" {
				continue
			}
			schemaID := strings.TrimSpace(*item.AttributeSchemaReference)
			if _, ok := seenSchemas[schemaID]; ok {
				continue
			}
			schema, schemaErr := b.Catalogs.GetAttributeSchema(ctx, input.BusinessID, schemaID)
			if schemaErr != nil {
				return ports.CustomerSalesContext{}, schemaErr
			}
			base.CatalogSchemaEvidence = append(base.CatalogSchemaEvidence, customerSalesSchemaEvidence(schema))
			seenSchemas[schemaID] = struct{}{}
		}
	}

	if b.Knowledge != nil {
		knowledgeRecords, listErr := b.Knowledge.ListPublished(ctx, input.BusinessID, "", now, b.maxKnowledge()*3)
		if listErr != nil {
			return ports.CustomerSalesContext{}, listErr
		}
		for _, record := range rankKnowledgeRecords(knowledgeRecords, input.Text) {
			if record.BusinessID != input.BusinessID {
				return ports.CustomerSalesContext{}, errors.New("AI context knowledge scope mismatch")
			}
			base.KnowledgeEvidence = append(base.KnowledgeEvidence, ports.CustomerSalesKnowledgeEvidence{
				Reference: record.ID, KnowledgeKey: record.KnowledgeKey, Title: record.Title, Content: record.Content,
				ContentType: record.ContentType, SourceReference: record.SourceReference, Authority: record.Authority,
				EvidenceState: evidenceStateForValidity(now, record.ValidFrom, record.ValidUntil), Version: record.Version, ValidFrom: record.ValidFrom, ValidUntil: record.ValidUntil,
				RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
			})
			if len(base.KnowledgeEvidence) >= b.maxKnowledge() {
				break
			}
		}
	}
	if b.Policies != nil {
		policyRecords, listErr := b.Policies.ListPublished(ctx, input.BusinessID, "", now, b.maxPolicies()*3)
		if listErr != nil {
			return ports.CustomerSalesContext{}, listErr
		}
		for _, record := range rankPolicyRecords(policyRecords, input.Text) {
			if record.BusinessID != input.BusinessID {
				return ports.CustomerSalesContext{}, errors.New("AI context policy scope mismatch")
			}
			base.BusinessPolicyEvidence = append(base.BusinessPolicyEvidence, ports.CustomerSalesBusinessPolicyEvidence{
				Reference: record.ID, PolicyKey: record.PolicyKey, Category: record.Category, Title: record.Title,
				Summary: record.Summary, Rules: safeJSONObject(record.Rules), Authority: record.Authority,
				EvidenceState: evidenceStateForValidity(now, record.ValidFrom, record.ValidUntil), Version: record.Version, ValidFrom: record.ValidFrom, ValidUntil: record.ValidUntil,
				RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
			})
			if len(base.BusinessPolicyEvidence) >= b.maxPolicies() {
				break
			}
		}
		if len(base.BusinessPolicyEvidence) > 0 {
			first := base.BusinessPolicyEvidence[0]
			base.PolicyEvidence = ports.CustomerSalesPolicyEvidence{Reference: first.Reference, Version: "policy-v" + formatInt(first.Version), State: "published", RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion}
		}
	}
	if len(base.CatalogEvidence) > 0 || len(base.OfferEvidence) > 0 || len(base.VariantEvidence) > 0 {
		base.KnowledgeState = CustomerSalesContextPartial
	}
	if len(base.KnowledgeEvidence) > 0 || len(base.BusinessPolicyEvidence) > 0 {
		base.KnowledgeState = CustomerSalesContextGrounded
	}
	if len(base.CatalogEvidence) == 0 {
		base.Freshness = CustomerSalesContextPartial
	}
	for _, offer := range base.OfferEvidence {
		if offer.EvidenceState == CustomerSalesContextStale {
			base.Freshness = CustomerSalesContextStale
			base.KnowledgeState = CustomerSalesContextPartial
			break
		}
	}
	for _, evidence := range base.KnowledgeEvidence {
		if evidence.EvidenceState == CustomerSalesContextStale {
			base.Freshness = CustomerSalesContextStale
			base.KnowledgeState = CustomerSalesContextPartial
			break
		}
	}
	for _, evidence := range base.BusinessPolicyEvidence {
		if evidence.EvidenceState == CustomerSalesContextStale {
			base.Freshness = CustomerSalesContextStale
			base.KnowledgeState = CustomerSalesContextPartial
			break
		}
	}
	return base, nil
}

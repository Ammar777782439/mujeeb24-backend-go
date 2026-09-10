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

func (b AutoReplyContextBuilder) retrieveScoped(ctx context.Context, businessID string, focus *ports.ConversationFocus, _ *ports.ConversationComparison, now time.Time) ([]ports.AICatalogEvidence, []ports.AIOfferEvidence, []ports.AIVariantEvidence, error) {
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

func (b AutoReplyContextBuilder) retrieveScopedOffer(ctx context.Context, businessID string, focus *ports.ConversationFocus, now time.Time) ([]ports.AICatalogEvidence, []ports.AIOfferEvidence, []ports.AIVariantEvidence, error) {
	catalog, item, offer, err := b.findOfferByIDWithinBusiness(ctx, businessID, focus.ID, focus.ItemID)
	if err != nil {
		return nil, nil, nil, err
	}
	if offer.BusinessID != businessID || item.BusinessID != businessID || catalog.BusinessID != businessID {
		return nil, nil, nil, errors.New("AI context offer scope mismatch")
	}
	catalogEvidence := []ports.AICatalogEvidence{{
		Reference: item.ID, CatalogReference: item.CatalogID, ItemType: item.ItemType, Name: item.Name, Status: item.Status,
		Attributes: safeJSONObject(item.Attributes), EvidenceState: AIContextFresh, RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
	}}
	offerEvidence := []ports.AIOfferEvidence{{
		Reference: offer.ID, CatalogItemReference: offer.CatalogItemID, VariantReference: stringValue(offer.VariantID),
		Name: offer.Name, PricingMode: offer.PricingMode, Amount: stringValue(offer.Amount), Currency: stringValue(offer.Currency),
		AvailabilityState: offer.AvailabilityStatus, Status: offer.Status, EvidenceState: offerEvidenceState(offer.AvailabilityStatus),
		RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
	}}
	var variantEvidence []ports.AIVariantEvidence
	if offer.VariantID != nil && strings.TrimSpace(*offer.VariantID) != "" {
		variants, listErr := b.Catalogs.ListVariants(ctx, businessID, item.ID, "active", b.maxVariants(), "")
		if listErr != nil {
			return nil, nil, nil, listErr
		}
		for _, v := range variants.Items {
			if v.ID == *offer.VariantID {
				variantEvidence = append(variantEvidence, ports.AIVariantEvidence{
					Reference: v.ID, CatalogItemReference: v.CatalogItemID, Name: v.Name, Status: v.Status,
					Attributes: safeJSONObject(v.Attributes), EvidenceState: AIContextFresh, RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
				})
				break
			}
		}
	}
	return catalogEvidence, offerEvidence, variantEvidence, nil
}

func (b AutoReplyContextBuilder) retrieveScopedItem(ctx context.Context, businessID string, focus *ports.ConversationFocus, now time.Time) ([]ports.AICatalogEvidence, []ports.AIOfferEvidence, []ports.AIVariantEvidence, error) {
	item, err := b.findItemByIDWithinBusiness(ctx, businessID, focus.ID, focus.CatalogID)
	if err != nil {
		return nil, nil, nil, err
	}
	catalogEvidence := []ports.AICatalogEvidence{{
		Reference: item.ID, CatalogReference: item.CatalogID, ItemType: item.ItemType, Name: item.Name, Status: item.Status,
		Attributes: safeJSONObject(item.Attributes), EvidenceState: AIContextFresh, RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
	}}
	offers, err := b.Catalogs.ListOffers(ctx, businessID, item.ID, "active", b.maxOffers(), "")
	if err != nil {
		return nil, nil, nil, err
	}
	var offerEvidence []ports.AIOfferEvidence
	for _, offer := range offers.Items {
		if offer.BusinessID != businessID || offer.CatalogItemID != item.ID {
			return nil, nil, nil, errors.New("AI context offer scope mismatch")
		}
		offerEvidence = append(offerEvidence, ports.AIOfferEvidence{
			Reference: offer.ID, CatalogItemReference: offer.CatalogItemID, VariantReference: stringValue(offer.VariantID),
			Name: offer.Name, PricingMode: offer.PricingMode, Amount: stringValue(offer.Amount), Currency: stringValue(offer.Currency),
			AvailabilityState: offer.AvailabilityStatus, Status: offer.Status, EvidenceState: offerEvidenceState(offer.AvailabilityStatus),
			RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
		})
	}
	variants, err := b.Catalogs.ListVariants(ctx, businessID, item.ID, "active", b.maxVariants(), "")
	if err != nil {
		return nil, nil, nil, err
	}
	var variantEvidence []ports.AIVariantEvidence
	for _, v := range variants.Items {
		if v.BusinessID != businessID || v.CatalogItemID != item.ID {
			return nil, nil, nil, errors.New("AI context variant scope mismatch")
		}
		variantEvidence = append(variantEvidence, ports.AIVariantEvidence{
			Reference: v.ID, CatalogItemReference: v.CatalogItemID, Name: v.Name, Status: v.Status,
			Attributes: safeJSONObject(v.Attributes), EvidenceState: AIContextFresh, RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
		})
	}
	return catalogEvidence, offerEvidence, variantEvidence, nil
}

func (b AutoReplyContextBuilder) retrieveScopedCatalog(ctx context.Context, businessID string, focus *ports.ConversationFocus, now time.Time) ([]ports.AICatalogEvidence, []ports.AIOfferEvidence, []ports.AIVariantEvidence, error) {
	catalog, err := b.Catalogs.GetCatalog(ctx, businessID, focus.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	if catalog.BusinessID != businessID {
		return nil, nil, nil, errors.New("AI context catalog scope mismatch")
	}
	items, err := b.Catalogs.ListCatalogItems(ctx, businessID, catalog.ID, "", "active", b.maxItems()*3, "")
	if err != nil {
		return nil, nil, nil, err
	}
	var catalogEvidence []ports.AICatalogEvidence
	// Rank only within this catalog.
	for _, item := range rankCatalogItems(items.Items, "") {
		// Empty text means keep order; rank returns all with score 0 sorted by ID.
		// Instead, take first N active items without lexical filter for catalog focus.
		_ = item
		break
	}
	// For catalog focus, include up to maxItems without lexical filtering.
	count := 0
	for _, item := range items.Items {
		if item.BusinessID != businessID || item.CatalogID != catalog.ID {
			return nil, nil, nil, errors.New("AI context catalog item scope mismatch")
		}
		catalogEvidence = append(catalogEvidence, ports.AICatalogEvidence{
			Reference: item.ID, CatalogReference: item.CatalogID, ItemType: item.ItemType, Name: item.Name, Status: item.Status,
			Attributes: safeJSONObject(item.Attributes), EvidenceState: AIContextFresh, RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
		})
		count++
		if count >= b.maxItems() {
			break
		}
	}
	if len(catalogEvidence) == 0 {
		return nil, nil, nil, &scopedNotFoundError{msg: "no active items in catalog"}
	}
	var offerEvidence []ports.AIOfferEvidence
	var variantEvidence []ports.AIVariantEvidence
	for _, item := range catalogEvidence {
		offers, err := b.Catalogs.ListOffers(ctx, businessID, item.Reference, "active", b.maxOffers(), "")
		if err != nil {
			return nil, nil, nil, err
		}
		for _, offer := range offers.Items {
			offerEvidence = append(offerEvidence, ports.AIOfferEvidence{
				Reference: offer.ID, CatalogItemReference: offer.CatalogItemID, VariantReference: stringValue(offer.VariantID),
				Name: offer.Name, PricingMode: offer.PricingMode, Amount: stringValue(offer.Amount), Currency: stringValue(offer.Currency),
				AvailabilityState: offer.AvailabilityStatus, Status: offer.Status, EvidenceState: offerEvidenceState(offer.AvailabilityStatus),
				RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
			})
		}
	}
	_ = variantEvidence
	return catalogEvidence, offerEvidence, nil, nil
}

func (b AutoReplyContextBuilder) retrieveScopedVariant(ctx context.Context, businessID string, focus *ports.ConversationFocus, now time.Time) ([]ports.AICatalogEvidence, []ports.AIOfferEvidence, []ports.AIVariantEvidence, error) {
	if focus.ItemID == nil || strings.TrimSpace(*focus.ItemID) == "" {
		return nil, nil, nil, &scopedNotFoundError{msg: "variant focus missing item"}
	}
	variants, err := b.Catalogs.ListVariants(ctx, businessID, *focus.ItemID, "active", b.maxVariants(), "")
	if err != nil {
		return nil, nil, nil, err
	}
	var matched *ports.VariantRecord
	for i := range variants.Items {
		if variants.Items[i].ID == focus.ID {
			matched = &variants.Items[i]
			break
		}
	}
	if matched == nil {
		return nil, nil, nil, &scopedNotFoundError{msg: "variant not found"}
	}
	item, err := b.findItemByIDWithinBusiness(ctx, businessID, *focus.ItemID, focus.CatalogID)
	if err != nil {
		return nil, nil, nil, err
	}
	catalogEvidence := []ports.AICatalogEvidence{{
		Reference: item.ID, CatalogReference: item.CatalogID, ItemType: item.ItemType, Name: item.Name, Status: item.Status,
		Attributes: safeJSONObject(item.Attributes), EvidenceState: AIContextFresh, RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
	}}
	variantEvidence := []ports.AIVariantEvidence{{
		Reference: matched.ID, CatalogItemReference: matched.CatalogItemID, Name: matched.Name, Status: matched.Status,
		Attributes: safeJSONObject(matched.Attributes), EvidenceState: AIContextFresh, RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
	}}
	offers, err := b.Catalogs.ListOffers(ctx, businessID, item.ID, "active", b.maxOffers(), "")
	if err != nil {
		return nil, nil, nil, err
	}
	var offerEvidence []ports.AIOfferEvidence
	for _, offer := range offers.Items {
		if offer.VariantID != nil && *offer.VariantID == matched.ID {
			offerEvidence = append(offerEvidence, ports.AIOfferEvidence{
				Reference: offer.ID, CatalogItemReference: offer.CatalogItemID, VariantReference: stringValue(offer.VariantID),
				Name: offer.Name, PricingMode: offer.PricingMode, Amount: stringValue(offer.Amount), Currency: stringValue(offer.Currency),
				AvailabilityState: offer.AvailabilityStatus, Status: offer.Status, EvidenceState: offerEvidenceState(offer.AvailabilityStatus),
				RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
			})
		}
	}
	return catalogEvidence, offerEvidence, variantEvidence, nil
}

func (b AutoReplyContextBuilder) retrieveComparison(ctx context.Context, businessID string, comparison *ports.ConversationComparison, now time.Time) ([]ports.AICatalogEvidence, []ports.AIOfferEvidence, []ports.AIVariantEvidence, error) {
	if comparison == nil || len(comparison.IDs) < 2 {
		return nil, nil, nil, &scopedNotFoundError{msg: "invalid comparison"}
	}
	var catalogEvidence []ports.AICatalogEvidence
	var offerEvidence []ports.AIOfferEvidence
	seenItems := map[string]bool{}
	for _, id := range comparison.IDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		catalog, item, offer, err := b.findOfferByIDWithinBusiness(ctx, businessID, id, nil)
		if err != nil {
			// Try as item: include its offers too, otherwise the AI context
			// would hold items without prices during a comparison.
			it, itemErr := b.findItemByIDWithinBusiness(ctx, businessID, id, nil)
			if itemErr != nil {
				return nil, nil, nil, err
			}
			if !seenItems[it.ID] {
				catalogEvidence = append(catalogEvidence, ports.AICatalogEvidence{
					Reference: it.ID, CatalogReference: it.CatalogID, ItemType: it.ItemType, Name: it.Name, Status: it.Status,
					Attributes: safeJSONObject(it.Attributes), EvidenceState: AIContextFresh, RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
				})
				seenItems[it.ID] = true
			}
			itemOffers, offerErr := b.Catalogs.ListOffers(ctx, businessID, it.ID, "active", b.maxOffers(), "")
			if offerErr != nil {
				return nil, nil, nil, offerErr
			}
			for _, itemOffer := range itemOffers.Items {
				if itemOffer.BusinessID != businessID || itemOffer.CatalogItemID != it.ID {
					return nil, nil, nil, errors.New("AI context comparison scope mismatch")
				}
				offerEvidence = append(offerEvidence, toOfferEvidence(itemOffer, now))
			}
			continue
		}
		if catalog.BusinessID != businessID || item.BusinessID != businessID || offer.BusinessID != businessID {
			return nil, nil, nil, errors.New("AI context comparison scope mismatch")
		}
		if !seenItems[item.ID] {
			catalogEvidence = append(catalogEvidence, ports.AICatalogEvidence{
				Reference: item.ID, CatalogReference: item.CatalogID, ItemType: item.ItemType, Name: item.Name, Status: item.Status,
				Attributes: safeJSONObject(item.Attributes), EvidenceState: AIContextFresh, RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
			})
			seenItems[item.ID] = true
		}
		offerEvidence = append(offerEvidence, toOfferEvidence(offer, now))
	}
	if len(catalogEvidence) == 0 {
		return nil, nil, nil, &scopedNotFoundError{msg: "comparison yielded no evidence"}
	}
	return catalogEvidence, offerEvidence, nil, nil
}

// toOfferEvidence projects a repository offer into AI evidence with a
// single field mapping, so scoped, item-based, and comparison paths cannot
// drift apart.
func toOfferEvidence(offer ports.OfferRecord, now time.Time) ports.AIOfferEvidence {
	return ports.AIOfferEvidence{
		Reference: offer.ID, CatalogItemReference: offer.CatalogItemID, VariantReference: stringValue(offer.VariantID),
		Name: offer.Name, PricingMode: offer.PricingMode, Amount: stringValue(offer.Amount), Currency: stringValue(offer.Currency),
		AvailabilityState: offer.AvailabilityStatus, Status: offer.Status, EvidenceState: offerEvidenceState(offer.AvailabilityStatus),
		RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
	}
}

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

func offerEvidenceState(availabilityStatus string) string {
	if strings.EqualFold(strings.TrimSpace(availabilityStatus), "unknown") || strings.EqualFold(strings.TrimSpace(availabilityStatus), "stale") {
		return AIContextStale
	}
	return AIContextFresh
}

type scopedNotFoundError struct{ msg string }

func (e *scopedNotFoundError) Error() string     { return "scoped retrieval not found: " + e.msg }
func (e *scopedNotFoundError) ErrorKind() string { return "not_found" }

// augmentScopedWithCandidates appends bounded alternatives so AI can resolve
// topic switches to a new entity. Focused evidence stays first. Total
// CatalogEvidence never exceeds maxItems. Candidates are siblings from the
// focused catalog plus lexical matches (score>0) from other catalogs.
func (b AutoReplyContextBuilder) augmentScopedWithCandidates(ctx context.Context, businessID, text string, focusedItems []ports.AICatalogEvidence, now time.Time) ([]ports.AICatalogEvidence, []ports.AIOfferEvidence, []ports.AIVariantEvidence, error) {
	exclude := make(map[string]struct{}, len(focusedItems))
	focusedCatalogs := make(map[string]struct{})
	for _, item := range focusedItems {
		exclude[item.Reference] = struct{}{}
		_ = item
		if item.CatalogReference != "" {
			focusedCatalogs[item.CatalogReference] = struct{}{}
		}
	}
	remaining := b.maxItems() - len(focusedItems)
	if remaining <= 0 {
		return nil, nil, nil, nil
	}
	catalogs, err := b.Catalogs.ListCatalogs(ctx, businessID, "active", b.maxCatalogs(), "")
	if err != nil {
		return nil, nil, nil, err
	}
	var candidates []ports.CatalogItemRecord
	// Siblings from focused catalogs first (bounded, no lexical filter).
	for _, catalog := range catalogs.Items {
		if _, ok := focusedCatalogs[catalog.ID]; !ok {
			continue
		}
		items, err := b.Catalogs.ListCatalogItems(ctx, businessID, catalog.ID, "", "active", b.maxItems()*3, "")
		if err != nil {
			return nil, nil, nil, err
		}
		for _, item := range items.Items {
			if _, skip := exclude[item.ID]; skip {
				continue
			}
			candidates = append(candidates, item)
			exclude[item.ID] = struct{}{}
			if len(candidates) >= remaining {
				break
			}
		}
		if len(candidates) >= remaining {
			break
		}
	}
	// Lexical matches from other catalogs (score>0 only).
	if len(candidates) < remaining {
		for _, catalog := range catalogs.Items {
			if _, ok := focusedCatalogs[catalog.ID]; ok {
				continue
			}
			items, err := b.Catalogs.ListCatalogItems(ctx, businessID, catalog.ID, "", "active", b.maxItems()*3, "")
			if err != nil {
				return nil, nil, nil, err
			}
			for _, item := range rankPositive(items.Items, text) {
				if _, skip := exclude[item.ID]; skip {
					continue
				}
				candidates = append(candidates, item)
				exclude[item.ID] = struct{}{}
				if len(candidates) >= remaining {
					break
				}
			}
			if len(candidates) >= remaining {
				break
			}
		}
	}
	var addItems []ports.AICatalogEvidence
	var addOffers []ports.AIOfferEvidence
	var addVariants []ports.AIVariantEvidence
	for _, item := range candidates {
		if item.BusinessID != businessID {
			return nil, nil, nil, errors.New("AI context candidate scope mismatch")
		}
		addItems = append(addItems, ports.AICatalogEvidence{
			Reference: item.ID, CatalogReference: item.CatalogID, ItemType: item.ItemType, Name: item.Name, Status: item.Status,
			Attributes: safeJSONObject(item.Attributes), EvidenceState: AIContextFresh, RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
		})
		offers, err := b.Catalogs.ListOffers(ctx, businessID, item.ID, "active", b.maxOffers(), "")
		if err != nil {
			return nil, nil, nil, err
		}
		for _, offer := range offers.Items {
			if offer.BusinessID != businessID || offer.CatalogItemID != item.ID {
				return nil, nil, nil, errors.New("AI context candidate offer scope mismatch")
			}
			addOffers = append(addOffers, ports.AIOfferEvidence{
				Reference: offer.ID, CatalogItemReference: offer.CatalogItemID, VariantReference: stringValue(offer.VariantID),
				Name: offer.Name, PricingMode: offer.PricingMode, Amount: stringValue(offer.Amount), Currency: stringValue(offer.Currency),
				AvailabilityState: offer.AvailabilityStatus, Status: offer.Status, EvidenceState: offerEvidenceState(offer.AvailabilityStatus),
				RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion,
			})
		}
	}
	return addItems, addOffers, addVariants, nil
}

// rankPositive returns only items with a positive lexical score, preserving rank order.
func rankPositive(items []ports.CatalogItemRecord, text string) []ports.CatalogItemRecord {
	ranked := rankCatalogItems(items, text)
	tokens := tokenize(text)
	if len(tokens) == 0 {
		return nil
	}
	var out []ports.CatalogItemRecord
	for _, item := range ranked {
		searchable := normalizeArabic(strings.ToLower(item.Name + " " + item.ItemType + " " + string(item.Attributes)))
		score := 0
		for _, token := range tokens {
			if strings.Contains(searchable, token) {
				score++
			}
		}
		if score > 0 {
			out = append(out, item)
		}
	}
	return out
}

func (b AutoReplyContextBuilder) finalizeContext(ctx context.Context, base ports.AIContext, input ports.ContextBuildInput, now time.Time) (ports.AIContext, error) {
	if b.Knowledge != nil {
		knowledgeRecords, listErr := b.Knowledge.ListPublished(ctx, input.BusinessID, "", now, b.maxKnowledge()*3)
		if listErr != nil {
			return ports.AIContext{}, listErr
		}
		for _, record := range rankKnowledgeRecords(knowledgeRecords, input.Text) {
			if record.BusinessID != input.BusinessID {
				return ports.AIContext{}, errors.New("AI context knowledge scope mismatch")
			}
			base.KnowledgeEvidence = append(base.KnowledgeEvidence, ports.AIKnowledgeEvidence{
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
			return ports.AIContext{}, listErr
		}
		for _, record := range rankPolicyRecords(policyRecords, input.Text) {
			if record.BusinessID != input.BusinessID {
				return ports.AIContext{}, errors.New("AI context policy scope mismatch")
			}
			base.BusinessPolicyEvidence = append(base.BusinessPolicyEvidence, ports.AIBusinessPolicyEvidence{
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
			base.PolicyEvidence = ports.AIPolicyEvidence{Reference: first.Reference, Version: "policy-v" + formatInt(first.Version), State: "published", RetrievedAt: now, SchemaVersion: AIEvidenceSchemaVersion}
		}
	}
	if len(base.CatalogEvidence) > 0 || len(base.OfferEvidence) > 0 || len(base.VariantEvidence) > 0 {
		base.KnowledgeState = AIContextPartial
	}
	if len(base.KnowledgeEvidence) > 0 || len(base.BusinessPolicyEvidence) > 0 {
		base.KnowledgeState = AIContextGrounded
	}
	if len(base.CatalogEvidence) == 0 {
		base.Freshness = AIContextPartial
	}
	for _, offer := range base.OfferEvidence {
		if offer.EvidenceState == AIContextStale {
			base.Freshness = AIContextStale
			base.KnowledgeState = AIContextPartial
			break
		}
	}
	return base, nil
}

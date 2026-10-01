package merchantcatalogai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type ReadOnlyCapabilityRegistry struct {
	capabilities      map[string]ports.AICapability
	selectedCatalogID string
}

func NewReadOnlyCapabilityRegistry(repository ports.CatalogRepository, selectedCatalogID string) *ReadOnlyCapabilityRegistry {
	r := &ReadOnlyCapabilityRegistry{capabilities: make(map[string]ports.AICapability), selectedCatalogID: strings.TrimSpace(selectedCatalogID)}
	if repository == nil {
		return r
	}
	r.capabilities["merchant_catalog_list_items"] = listItemsCapability{repository: repository, selectedCatalogID: r.selectedCatalogID}
	r.capabilities["merchant_catalog_get_item"] = getItemCapability{repository: repository, selectedCatalogID: r.selectedCatalogID}
	r.capabilities["merchant_catalog_list_variants"] = listVariantsCapability{repository: repository, selectedCatalogID: r.selectedCatalogID}
	r.capabilities["merchant_catalog_list_offers"] = listOffersCapability{repository: repository, selectedCatalogID: r.selectedCatalogID}
	return r
}

func (r *ReadOnlyCapabilityRegistry) Definitions() []ports.AICapabilityDefinition {
	out := make([]ports.AICapabilityDefinition, 0, len(r.capabilities))
	for _, capability := range r.capabilities {
		out = append(out, capability.Definition())
	}
	return out
}

func (r *ReadOnlyCapabilityRegistry) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, name string, rawParams []byte) (ports.AICapabilityResult, error) {
	capability, ok := r.capabilities[name]
	if !ok {
		return ports.AICapabilityResult{}, errors.New("merchant catalog capability is not available: " + name)
	}
	return capability.Execute(ctx, execCtx, rawParams)
}

func (r *ReadOnlyCapabilityRegistry) Get(name string) (ports.AICapability, bool) {
	capability, ok := r.capabilities[name]
	return capability, ok
}

func jsonParams(raw []byte) (map[string]any, error) {
	params := map[string]any{}
	if len(raw) == 0 {
		return params, nil
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, errors.New("invalid capability parameters")
	}
	return params, nil
}

func requiredString(params map[string]any, key string) (string, error) {
	value, ok := params[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return "", errors.New(key + " is required")
	}
	return strings.TrimSpace(value), nil
}

func readLimit(params map[string]any) int {
	limit := 25
	if value, ok := params["limit"].(float64); ok && int(value) > 0 {
		limit = int(value)
	}
	if limit > 100 {
		return 100
	}
	return limit
}

type listItemsCapability struct {
	repository ports.CatalogRepository
	selectedCatalogID string
}

func (c listItemsCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name: "merchant_catalog_list_items",
		Description: "Read items from the already selected merchant catalog. This is bounded factual retrieval, not semantic search. The catalog is selected by Mujeeb; the model must not choose it.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{"type": "string"},
				"limit": map[string]any{"type": "integer"},
				"cursor": map[string]any{"type": "string"},
			},
			"required": []string{},
		},
	}
}

func (c listItemsCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	params, err := jsonParams(rawParams)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}
	catalogID := c.selectedCatalogID
	if catalogID == "" {
		return ports.AICapabilityResult{}, errors.New("selected merchant catalog is required")
	}
	status, _ := params["status"].(string)
	cursor, _ := params["cursor"].(string)
	page, err := c.repository.ListCatalogItems(ctx, execCtx.BusinessID, catalogID, "", status, readLimit(params), cursor)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}
	now := time.Now().UTC()
	items := make([]map[string]any, 0, len(page.Items))
	evidence := make([]ports.AICatalogEvidence, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, map[string]any{
			"id": item.ID,
			"catalog_id": item.CatalogID,
			"item_type": item.ItemType,
			"name": item.Name,
			"status": item.Status,
			"attributes": json.RawMessage(item.Attributes),
		})
		evidence = append(evidence, ports.AICatalogEvidence{
			Reference: item.ID,
			CatalogReference: item.CatalogID,
			ItemType: item.ItemType,
			Name: item.Name,
			Status: item.Status,
			Attributes: append([]byte(nil), item.Attributes...),
			EvidenceState: "verified",
			RetrievedAt: now,
		})
	}
	return ports.AICapabilityResult{
		Data: items,
		CatalogEvidence: evidence,
		HasMore: page.HasMore,
		NextCursor: page.NextCursor,
		Operation: "merchant_catalog_list_items",
	}, nil
}

type getItemCapability struct {
	repository ports.CatalogRepository
	selectedCatalogID string
}

func (c getItemCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name: "merchant_catalog_get_item",
		Description: "Read one item by exact item_id in the already selected merchant catalog using tenant-scoped repository access.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"item_id": map[string]any{"type": "string"},
			},
			"required": []string{"item_id"},
		},
	}
}

func (c getItemCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	params, err := jsonParams(rawParams)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}
	catalogID := c.selectedCatalogID
	if catalogID == "" {
		return ports.AICapabilityResult{}, errors.New("selected merchant catalog is required")
	}
	itemID, err := requiredString(params, "item_id")
	if err != nil {
		return ports.AICapabilityResult{}, err
	}
	item, err := c.repository.GetCatalogItem(ctx, execCtx.BusinessID, catalogID, itemID)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}
	now := time.Now().UTC()
	return ports.AICapabilityResult{
		Data: map[string]any{
			"id": item.ID,
			"catalog_id": item.CatalogID,
			"item_type": item.ItemType,
			"name": item.Name,
			"status": item.Status,
			"attributes": json.RawMessage(item.Attributes),
		},
		CatalogEvidence: []ports.AICatalogEvidence{{
			Reference: item.ID,
			CatalogReference: item.CatalogID,
			ItemType: item.ItemType,
			Name: item.Name,
			Status: item.Status,
			Attributes: append([]byte(nil), item.Attributes...),
			EvidenceState: "verified",
			RetrievedAt: now,
		}},
		Operation: "merchant_catalog_get_item",
	}, nil
}

type listVariantsCapability struct {
	repository ports.CatalogRepository
	selectedCatalogID string
}

func (c listVariantsCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name: "merchant_catalog_list_variants",
		Description: "Read variants for one catalog item.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"item_id": map[string]any{"type": "string"},
				"status": map[string]any{"type": "string"},
				"limit": map[string]any{"type": "integer"},
				"cursor": map[string]any{"type": "string"},
			},
			"required": []string{"item_id"},
		},
	}
}

func (c listVariantsCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	params, err := jsonParams(rawParams)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}
	itemID, err := requiredString(params, "item_id")
	if err != nil {
		return ports.AICapabilityResult{}, err
	}
	item, err := c.repository.GetCatalogItem(ctx, execCtx.BusinessID, c.selectedCatalogID, itemID)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}
	_ = item
	status, _ := params["status"].(string)
	cursor, _ := params["cursor"].(string)
	page, err := c.repository.ListVariants(ctx, execCtx.BusinessID, itemID, status, readLimit(params), cursor)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}
	now := time.Now().UTC()
	data := make([]map[string]any, 0, len(page.Items))
	evidence := make([]ports.AIVariantEvidence, 0, len(page.Items))
	for _, variant := range page.Items {
		data = append(data, map[string]any{
			"id": variant.ID,
			"catalog_item_id": variant.CatalogItemID,
			"name": variant.Name,
			"status": variant.Status,
			"attributes": json.RawMessage(variant.Attributes),
		})
		evidence = append(evidence, ports.AIVariantEvidence{
			Reference: variant.ID,
			CatalogItemReference: variant.CatalogItemID,
			Name: variant.Name,
			Status: variant.Status,
			Attributes: append([]byte(nil), variant.Attributes...),
			EvidenceState: "verified",
			RetrievedAt: now,
		})
	}
	return ports.AICapabilityResult{Data: data, VariantEvidence: evidence, HasMore: page.HasMore, NextCursor: page.NextCursor, Operation: "merchant_catalog_list_variants"}, nil
}

type listOffersCapability struct {
	repository ports.CatalogRepository
	selectedCatalogID string
}

func (c listOffersCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name: "merchant_catalog_list_offers",
		Description: "Read offers for one catalog item, including factual price and availability evidence.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"item_id": map[string]any{"type": "string"},
				"status": map[string]any{"type": "string"},
				"limit": map[string]any{"type": "integer"},
				"cursor": map[string]any{"type": "string"},
			},
			"required": []string{"item_id"},
		},
	}
}

func (c listOffersCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	params, err := jsonParams(rawParams)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}
	itemID, err := requiredString(params, "item_id")
	if err != nil {
		return ports.AICapabilityResult{}, err
	}
	item, err := c.repository.GetCatalogItem(ctx, execCtx.BusinessID, c.selectedCatalogID, itemID)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}
	_ = item
	status, _ := params["status"].(string)
	cursor, _ := params["cursor"].(string)
	page, err := c.repository.ListOffers(ctx, execCtx.BusinessID, itemID, status, readLimit(params), cursor)
	if err != nil {
		return ports.AICapabilityResult{}, err
	}
	now := time.Now().UTC()
	data := make([]map[string]any, 0, len(page.Items))
	evidence := make([]ports.AIOfferEvidence, 0, len(page.Items))
	for _, offer := range page.Items {
		variantID := ""
		if offer.VariantID != nil {
			variantID = *offer.VariantID
		}
		data = append(data, map[string]any{
			"id": offer.ID,
			"catalog_item_id": offer.CatalogItemID,
			"variant_id": variantID,
			"name": offer.Name,
			"pricing_mode": offer.PricingMode,
			"amount": offer.Amount,
			"currency": offer.Currency,
			"availability_status": offer.AvailabilityStatus,
			"status": offer.Status,
		})
		evidence = append(evidence, ports.AIOfferEvidence{
			Reference: offer.ID,
			CatalogItemReference: offer.CatalogItemID,
			VariantReference: variantID,
			Name: offer.Name,
			PricingMode: offer.PricingMode,
			Amount: derefString(offer.Amount),
			Currency: derefString(offer.Currency),
			AvailabilityStatus: offer.AvailabilityStatus,
			Status: offer.Status,
			EvidenceState: "verified",
			RetrievedAt: now,
		})
	}
	return ports.AICapabilityResult{Data: data, OfferEvidence: evidence, HasMore: page.HasMore, NextCursor: page.NextCursor, Operation: "merchant_catalog_list_offers"}, nil
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

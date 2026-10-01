package merchantcatalogai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type ReadOnlyCapabilityRegistry struct {
	capabilities              map[string]ports.AICapability
	selectedCatalogID         string
	evidenceReferences        map[string]struct{}
	attributeSchemaReferences map[string]struct{}
}

func NewReadOnlyCapabilityRegistry(repository ports.CatalogRepository, selectedCatalogID string) *ReadOnlyCapabilityRegistry {
	r := &ReadOnlyCapabilityRegistry{
		capabilities:       make(map[string]ports.AICapability),
		selectedCatalogID:  strings.TrimSpace(selectedCatalogID),
		evidenceReferences:        make(map[string]struct{}),
		attributeSchemaReferences: make(map[string]struct{}),
	}
	if repository == nil {
		return r
	}
	r.capabilities["merchant_catalog_list_items"] = listItemsCapability{repository: repository, selectedCatalogID: r.selectedCatalogID}
	r.capabilities["merchant_catalog_get_item"] = getItemCapability{repository: repository, selectedCatalogID: r.selectedCatalogID}
	r.capabilities["merchant_catalog_list_variants"] = listVariantsCapability{repository: repository, selectedCatalogID: r.selectedCatalogID}
	r.capabilities["merchant_catalog_list_offers"] = listOffersCapability{repository: repository, selectedCatalogID: r.selectedCatalogID}
	r.capabilities["merchant_catalog_list_attribute_schemas"] = listAttributeSchemasCapability{repository: repository}
	log.Printf("[MerchantCatalogAI][DISCOVERY] registry_initialized selected_catalog=%s tools=%d",
		r.selectedCatalogID, len(r.capabilities))
	return r
}

func (r *ReadOnlyCapabilityRegistry) Definitions() []ports.AICapabilityDefinition {
	out := make([]ports.AICapabilityDefinition, 0, len(r.capabilities))
	for _, capability := range r.capabilities {
		out = append(out, capability.Definition())
	}
	log.Printf("[MerchantCatalogAI][DISCOVERY] definitions_ready selected_catalog=%s count=%d",
		r.selectedCatalogID, len(out))
	return out
}

func (r *ReadOnlyCapabilityRegistry) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, name string, rawParams []byte) (ports.AICapabilityResult, error) {
	capability, ok := r.capabilities[name]
	if !ok {
		return ports.AICapabilityResult{}, errors.New("merchant catalog capability is not available: " + name)
	}
	started := time.Now()
	log.Printf("[MerchantCatalogAI][DISCOVERY] START business=%s session=%s tool=%s params_bytes=%d",
		execCtx.BusinessID, execCtx.ConversationID, name, len(rawParams))

	result, err := capability.Execute(ctx, execCtx, rawParams)
	if err != nil {
		log.Printf("[MerchantCatalogAI][DISCOVERY] ERROR business=%s session=%s tool=%s latency_ms=%d err=%v",
			execCtx.BusinessID, execCtx.ConversationID, name, time.Since(started).Milliseconds(), err)
		return ports.AICapabilityResult{}, err
	}

	for _, evidence := range result.CatalogEvidence {
		if ref := strings.TrimSpace(evidence.Reference); ref != "" {
			r.evidenceReferences[ref] = struct{}{}
		}
	}
	for _, evidence := range result.VariantEvidence {
		if ref := strings.TrimSpace(evidence.Reference); ref != "" {
			r.evidenceReferences[ref] = struct{}{}
		}
	}
	for _, evidence := range result.OfferEvidence {
		if ref := strings.TrimSpace(evidence.Reference); ref != "" {
			r.evidenceReferences[ref] = struct{}{}
		}
	}

	if name == "merchant_catalog_list_attribute_schemas" {
		schemaIDs, normalizeErr := normalizeAttributeSchemaReferences(result.Data)
		if normalizeErr != nil {
			log.Printf("[MerchantCatalogAI][DISCOVERY] NORMALIZE_ERROR business=%s session=%s tool=%s latency_ms=%d err=%v",
				execCtx.BusinessID, execCtx.ConversationID, name, time.Since(started).Milliseconds(), normalizeErr)
			return ports.AICapabilityResult{}, normalizeErr
		}
		for _, id := range schemaIDs {
			r.attributeSchemaReferences[id] = struct{}{}
		}
		log.Printf("[MerchantCatalogAI][DISCOVERY] SCHEMAS business=%s session=%s count=%d ids=%v",
			execCtx.BusinessID, execCtx.ConversationID, len(schemaIDs), schemaIDs)
	}

	log.Printf("[MerchantCatalogAI][DISCOVERY] OK business=%s session=%s tool=%s operation=%s latency_ms=%d catalog_refs=%d variant_refs=%d offer_refs=%d",
		execCtx.BusinessID, execCtx.ConversationID, name, result.Operation, time.Since(started).Milliseconds(),
		len(result.CatalogEvidence), len(result.VariantEvidence), len(result.OfferEvidence))
	return result, nil
}

func normalizeAttributeSchemaReferences(data any) ([]string, error) {
	payload, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("attribute schema discovery payload must be object, got %T", data)
	}

	raw, ok := payload["attribute_schemas"]
	if !ok {
		return nil, errors.New("attribute schema discovery payload is missing attribute_schemas")
	}

	schemas, ok := raw.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("attribute_schemas must be []map[string]any, got %T", raw)
	}

	ids := make([]string, 0, len(schemas))
	for index, schema := range schemas {
		id, ok := schema["id"].(string)
		if !ok || strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("attribute_schemas[%d].id is required", index)
		}
		ids = append(ids, strings.TrimSpace(id))
	}
	return ids, nil
}

// ValidateProposalReferences enforces the closed B2B reference boundary.
// Every existing resource referenced by a mutation must have been returned by
// a tenant-scoped read capability during this turn.
func (r *ReadOnlyCapabilityRegistry) ValidateProposalReferences(p Proposal) error {
	if p.Status != StatusResolved || !p.IsMutation() {
		return nil
	}
	known := make(map[string]struct{}, len(r.evidenceReferences))
	for ref := range r.evidenceReferences {
		known[ref] = struct{}{}
	}
	require := func(id string, kind string) error {
		id = strings.TrimSpace(id)
		if id == "" {
			return errors.New(kind + " reference is required")
		}
		if _, ok := known[id]; !ok {
			return errors.New("proposal references a " + kind + " that was not returned by a catalog read tool: " + id)
		}
		return nil
	}
	switch p.Operation {
	case OperationCreate:
		if p.Create != nil && p.Create.AttributeSchemaID != nil {
			schemaID := strings.TrimSpace(*p.Create.AttributeSchemaID)
			if schemaID == "" {
				return errors.New("create attribute_schema_id cannot be empty")
			}
			if _, ok := r.attributeSchemaReferences[schemaID]; !ok {
				return errors.New("create proposal references an attribute schema that was not returned by attribute schema discovery: " + schemaID)
			}
		}
	case OperationUpdate:
		if p.Update == nil {
			return errors.New("update proposal is missing update payload")
		}
		if err := require(p.Update.ItemID, "catalog item"); err != nil {
			return err
		}
		for _, v := range p.Update.ExistingVariants {
			if err := require(v.ID, "variant"); err != nil {
				return err
			}
		}
		for _, o := range p.Update.ExistingOffers {
			if err := require(o.ID, "offer"); err != nil {
				return err
			}
		}
		for _, o := range p.Update.NewOffers {
			if o.VariantID != nil {
				if err := require(*o.VariantID, "variant"); err != nil {
					return err
				}
			}
		}
	case OperationDelete:
		if p.Delete == nil {
			return errors.New("delete proposal is missing delete payload")
		}
		return require(p.Delete.ItemID, "catalog item")
	}
	return nil
}

// listAttributeSchemasCapability exposes tenant-scoped AttributeSchema definitions
// as optional existing evidence. Dynamic attributes do not require a schema. It
// is read-only; this capability never creates or mutates schemas.
type listAttributeSchemasCapability struct {
	repository ports.CatalogRepository
}

func (c listAttributeSchemasCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name: "merchant_catalog_list_attribute_schemas",
		Description: "List existing AttributeSchema versions and definitions for the current business when existing schema evidence is relevant. Dynamic attributes do not require a schema. This is read-only and tenant-scoped.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Optional schema name filter."},
				"version": map[string]any{"type": "integer", "description": "Optional exact schema version."},
			},
		},
	}
}

func (c listAttributeSchemasCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	started := time.Now()
	var params struct {
		Name string `json:"name"`
		Version *int `json:"version"`
	}
	if len(rawParams) > 0 {
		if err := json.Unmarshal(rawParams, &params); err != nil {
			log.Printf("[MerchantCatalogAI][DISCOVERY][SCHEMA] ERROR business=%s session=%s stage=parse_params err=%v",
				execCtx.BusinessID, execCtx.ConversationID, err)
			return ports.AICapabilityResult{}, err
		}
	}
	log.Printf("[MerchantCatalogAI][DISCOVERY][SCHEMA] START business=%s session=%s name_filter=%q version_filter=%v",
		execCtx.BusinessID, execCtx.ConversationID, strings.TrimSpace(params.Name), params.Version)

	page, err := c.repository.ListAttributeSchemas(ctx, execCtx.BusinessID, strings.TrimSpace(params.Name), params.Version, 100, "")
	if err != nil {
		log.Printf("[MerchantCatalogAI][DISCOVERY][SCHEMA] ERROR business=%s session=%s stage=repository latency_ms=%d err=%v",
			execCtx.BusinessID, execCtx.ConversationID, time.Since(started).Milliseconds(), err)
		return ports.AICapabilityResult{}, err
	}
	schemas := make([]map[string]any, 0, len(page.Items))
	definitionCount := 0
	for _, schema := range page.Items {
		definitions := make([]map[string]any, 0, len(schema.Definitions))
		definitionCount += len(schema.Definitions)
		for _, definition := range schema.Definitions {
			var rules any
			if len(definition.ValidationRules) > 0 {
				_ = json.Unmarshal(definition.ValidationRules, &rules)
			}
			definitions = append(definitions, map[string]any{
				"id": definition.ID,
				"key": definition.Key,
				"label": definition.Label,
				"data_type": definition.DataType,
				"required": definition.Required,
				"searchable": definition.Searchable,
				"validation_rules": rules,
				"display_order": definition.DisplayOrder,
			})
		}
		schemas = append(schemas, map[string]any{
			"id": schema.ID,
			"name": schema.Name,
			"version": schema.Version,
			"definitions": definitions,
		})
	}
	log.Printf("[MerchantCatalogAI][DISCOVERY][SCHEMA] OK business=%s session=%s schemas=%d definitions=%d has_more=%t next_cursor_present=%t latency_ms=%d",
		execCtx.BusinessID, execCtx.ConversationID, len(schemas), definitionCount, page.HasMore, strings.TrimSpace(page.NextCursor) != "",
		time.Since(started).Milliseconds())
	return ports.AICapabilityResult{
		Data: map[string]any{"attribute_schemas": schemas, "has_more": page.HasMore, "next_cursor": page.NextCursor},
		HasMore: page.HasMore,
		NextCursor: page.NextCursor,
		Operation: "list_attribute_schemas",
	}, nil
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
				"search": map[string]any{"type": "string", "description": "Deterministic name/text filter; not semantic search."},
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
	search, _ := params["search"].(string)
	cursor, _ := params["cursor"].(string)
	page, err := c.repository.ListCatalogItems(ctx, execCtx.BusinessID, catalogID, strings.TrimSpace(search), status, readLimit(params), cursor)
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
			"attribute_schema_id": item.AttributeSchemaID,
			"attribute_schema_version": item.AttributeSchemaVersion,
			"item_type": item.ItemType,
			"name": item.Name,
			"short_description": item.ShortDescription,
			"long_description": item.LongDescription,
			"status": item.Status,
			"pricing_mode": item.PricingMode,
			"availability_mode": item.AvailabilityMode,
			"fulfillment_mode": item.FulfillmentMode,
			"requires_confirmation": item.RequiresConfirmation,
			"attributes": json.RawMessage(item.Attributes),
		})
		evidence = append(evidence, ports.AICatalogEvidence{
			Reference: item.ID,
			CatalogReference: item.CatalogID,
			ItemType: item.ItemType,
			Name: item.Name,
			Status: item.Status,
			Attributes: append([]byte(nil), item.Attributes...),
			ShortDescription: item.ShortDescription,
			LongDescription: item.LongDescription,
			PricingMode: item.PricingMode,
			AvailabilityMode: item.AvailabilityMode,
			FulfillmentMode: item.FulfillmentMode,
			RequiresConfirmation: item.RequiresConfirmation,
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
			"attribute_schema_id": item.AttributeSchemaID,
			"attribute_schema_version": item.AttributeSchemaVersion,
			"item_type": item.ItemType,
			"name": item.Name,
			"short_description": item.ShortDescription,
			"long_description": item.LongDescription,
			"status": item.Status,
			"pricing_mode": item.PricingMode,
			"availability_mode": item.AvailabilityMode,
			"fulfillment_mode": item.FulfillmentMode,
			"requires_confirmation": item.RequiresConfirmation,
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
			"pricing_unit": offer.PricingUnit,
			"price_source": offer.PriceSource,
			"price_verification_status": offer.PriceVerificationStatus,
			"price_checked_at": offer.PriceCheckedAt,
			"availability_mode": offer.AvailabilityMode,
			"availability_status": offer.AvailabilityStatus,
			"availability_source": offer.AvailabilitySource,
			"availability_checked_at": offer.AvailabilityCheckedAt,
			"availability_valid_until": offer.AvailabilityValidUntil,
			"availability_evidence_ref": offer.AvailabilityEvidenceRef,
			"fulfillment_mode": offer.FulfillmentMode,
			"validity_from": offer.ValidityFrom,
			"validity_until": offer.ValidityUntil,
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

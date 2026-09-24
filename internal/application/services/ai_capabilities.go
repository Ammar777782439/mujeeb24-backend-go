package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

// CapabilityRegistry is a thread-safe implementation of ports.AICapabilityRegistry.
type CapabilityRegistry struct {
	mu           sync.RWMutex
	capabilities map[string]ports.AICapability
}

func NewCapabilityRegistry() *CapabilityRegistry {
	return &CapabilityRegistry{
		capabilities: make(map[string]ports.AICapability),
	}
}

func (r *CapabilityRegistry) Register(capability ports.AICapability) error {
	if capability == nil {
		return errors.New("capability cannot be nil")
	}
	def := capability.Definition()
	name := strings.TrimSpace(def.Name)
	if name == "" {
		return errors.New("capability name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.capabilities[name]; exists {
		return fmt.Errorf("capability %q is already registered", name)
	}
	r.capabilities[name] = capability
	return nil
}

func (r *CapabilityRegistry) Get(name string) (ports.AICapability, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cap, ok := r.capabilities[strings.TrimSpace(name)]
	return cap, ok
}

func (r *CapabilityRegistry) Definitions() []ports.AICapabilityDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]ports.AICapabilityDefinition, 0, len(r.capabilities))
	for _, cap := range r.capabilities {
		defs = append(defs, cap.Definition())
	}
	return defs
}

func (r *CapabilityRegistry) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, name string, rawParams []byte) (ports.AICapabilityResult, error) {
	cap, ok := r.Get(name)
	if !ok {
		return ports.AICapabilityResult{}, fmt.Errorf("unknown capability %q", name)
	}
	return cap.Execute(ctx, execCtx, rawParams)
}

var _ ports.AICapabilityRegistry = (*CapabilityRegistry)(nil)

// IncorporateCapabilityEvidence delegates to ports.IncorporateCapabilityEvidence.
func IncorporateCapabilityEvidence(target *ports.AIContext, result ports.AICapabilityResult) {
	ports.IncorporateCapabilityEvidence(target, result)
}

// CatalogDataCapability is a pure DATA ACCESS capability exposing explicitly
// requested catalog data via application query services. It contains NO search,
// ranking, or reasoning logic.
type CatalogDataCapability struct {
	ListCatalogs       queries.ListCatalogsHandler
	ListCatalogItems   queries.ListCatalogItemsHandler
	GetCatalogItem     queries.GetCatalogItemHandler
	ListOffers         queries.ListOffersHandler
	ListVariants       queries.ListVariantsHandler
	GetAttributeSchema queries.GetAttributeSchemaHandler
	Now                func() time.Time
}

func NewCatalogDataCapability(
	listCatalogs queries.ListCatalogsHandler,
	listCatalogItems queries.ListCatalogItemsHandler,
	getCatalogItem queries.GetCatalogItemHandler,
	listOffers queries.ListOffersHandler,
	listVariants queries.ListVariantsHandler,
	getAttributeSchema queries.GetAttributeSchemaHandler,
) *CatalogDataCapability {
	return &CatalogDataCapability{
		ListCatalogs:       listCatalogs,
		ListCatalogItems:   listCatalogItems,
		GetCatalogItem:     getCatalogItem,
		ListOffers:         listOffers,
		ListVariants:       listVariants,
		GetAttributeSchema: getAttributeSchema,
	}
}

func (c *CatalogDataCapability) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

func (c *CatalogDataCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name: "catalog_data",
		Description: "Retrieve factual merchant catalog data (catalogs, items, offers, variants, schemas) for this business. " +
			"This is a deterministic data access tool, not a search engine or recommendation system.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{
					"type":        "string",
					"enum":        []string{"list_catalogs", "list_catalog_items", "get_catalog_item", "list_offers", "list_variants", "get_attribute_schema"},
					"description": "The explicit catalog data operation to perform.",
				},
				"catalog_id": map[string]any{
					"type":        "string",
					"description": "Catalog ID, required for list_catalog_items and get_catalog_item.",
				},
				"item_id": map[string]any{
					"type":        "string",
					"description": "Catalog item ID, required for get_catalog_item, list_offers, and list_variants.",
				},
				"schema_id": map[string]any{
					"type":        "string",
					"description": "Attribute schema ID, required for get_attribute_schema.",
				},
				"status": map[string]any{
					"type":        "string",
					"description": "Optional status filter (e.g., active).",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of records to return.",
				},
				"cursor": map[string]any{
					"type":        "string",
					"description": "Pagination cursor from previous response next_cursor.",
				},
			},
			"required": []string{"operation"},
		},
	}
}

type catalogDataParams struct {
	Operation string `json:"operation"`
	CatalogID string `json:"catalog_id"`
	ItemID    string `json:"item_id"`
	SchemaID  string `json:"schema_id"`
	Status    string `json:"status"`
	Limit     int    `json:"limit"`
	Cursor    string `json:"cursor"`
}

type CatalogProjection struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Status      string  `json:"status"`
}

type CatalogItemProjection struct {
	ID                     string         `json:"id"`
	CatalogID              string         `json:"catalog_id"`
	AttributeSchemaID      *string        `json:"attribute_schema_id,omitempty"`
	AttributeSchemaVersion *int           `json:"attribute_schema_version,omitempty"`
	ItemType               string         `json:"item_type"`
	Name                   string         `json:"name"`
	Status                 string         `json:"status"`
	Attributes             map[string]any `json:"attributes,omitempty"`
}

type OfferProjection struct {
	ID                 string  `json:"id"`
	CatalogItemID      string  `json:"catalog_item_id"`
	VariantID          *string `json:"variant_id,omitempty"`
	Name               string  `json:"name"`
	PricingMode        string  `json:"pricing_mode"`
	Amount             *string `json:"amount,omitempty"`
	Currency           *string `json:"currency,omitempty"`
	AvailabilityStatus string  `json:"availability_status"`
	Status             string  `json:"status"`
}

type VariantProjection struct {
	ID            string         `json:"id"`
	CatalogItemID string         `json:"catalog_item_id"`
	Name          string         `json:"name"`
	Status        string         `json:"status"`
	Attributes    map[string]any `json:"attributes,omitempty"`
}

type AttributeDefinitionProjection struct {
	ID           string `json:"id"`
	Key          string `json:"key"`
	Label        string `json:"label"`
	DataType     string `json:"data_type"`
	Required     bool   `json:"required"`
	Searchable   bool   `json:"searchable"`
	DisplayOrder int    `json:"display_order"`
}

type AttributeSchemaProjection struct {
	ID          string                          `json:"id"`
	Name        string                          `json:"name"`
	Version     int                             `json:"version"`
	Definitions []AttributeDefinitionProjection `json:"definitions"`
}

func (c *CatalogDataCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	businessID := strings.TrimSpace(execCtx.BusinessID)
	if businessID == "" {
		return ports.AICapabilityResult{}, errors.New("unauthorized: business ID is required in capability execution context")
	}

	var params catalogDataParams
	if len(rawParams) > 0 {
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return ports.AICapabilityResult{}, fmt.Errorf("invalid parameters for catalog_data: %w", err)
		}
	}

	now := c.now()
	actorCtx := commands.ActorContext{BusinessID: commands.BusinessID(businessID)}
	queryMeta := queries.QueryMeta{Actor: actorCtx}

	switch strings.TrimSpace(params.Operation) {
	case "list_catalogs":
		if c.ListCatalogs == nil {
			return ports.AICapabilityResult{}, errors.New("list_catalogs handler is not configured")
		}
		res, err := c.ListCatalogs.Handle(ctx, queries.ListCatalogsQuery{
			Meta:   queryMeta,
			Status: params.Status,
			Limit:  params.Limit,
			Cursor: params.Cursor,
		})
		if err != nil {
			return ports.AICapabilityResult{}, err
		}
		projections := make([]CatalogProjection, 0, len(res.Items))
		for _, item := range res.Items {
			projections = append(projections, CatalogProjection{
				ID:          string(item.ID),
				Name:        item.Name,
				Description: item.Description,
				Status:      item.Status,
			})
		}

		return ports.AICapabilityResult{
			Data: map[string]any{
				"catalogs":    projections,
				"count":       len(projections),
				"has_more":    res.HasMore,
				"next_cursor": res.NextCursor,
			},




		}, nil

	case "list_catalog_items":
		if c.ListCatalogItems == nil {
			return ports.AICapabilityResult{}, errors.New("list_catalog_items handler is not configured")
		}
		catalogID := strings.TrimSpace(params.CatalogID)
		if catalogID == "" {
			return ports.AICapabilityResult{}, errors.New("catalog_id is required for list_catalog_items")
		}
		res, err := c.ListCatalogItems.Handle(ctx, queries.ListCatalogItemsQuery{
			Meta:      queryMeta,
			CatalogID: commands.CatalogID(catalogID),
			Status:    params.Status,
			Limit:     params.Limit,
			Cursor:    params.Cursor,
		})
		if err != nil {
			return ports.AICapabilityResult{}, err
		}
		projections := make([]CatalogItemProjection, 0, len(res.Items))
		evidence := make([]ports.AICatalogEvidence, 0, len(res.Items))
		for _, item := range res.Items {
			var attrs map[string]any
			if len(item.Attributes) > 0 {
				_ = json.Unmarshal(item.Attributes, &attrs)
			}
			var schemaID *string
			if item.AttributeSchemaID != nil {
				str := string(*item.AttributeSchemaID)
				schemaID = &str
			}
			projections = append(projections, CatalogItemProjection{
				ID:                     string(item.ID),
				CatalogID:              string(item.CatalogID),
				AttributeSchemaID:      schemaID,
				AttributeSchemaVersion: item.AttributeSchemaVersion,
				ItemType:               item.ItemType,
				Name:                   item.Name,
				Status:                 item.Status,
				Attributes:             attrs,
			})
			evidence = append(evidence, ports.AICatalogEvidence{
				Reference:        string(item.ID),
				CatalogReference: string(item.CatalogID),
				ItemType:         item.ItemType,
				Name:             item.Name,
				Status:           item.Status,
				Attributes:       safeJSONObject(item.Attributes),
				EvidenceState:    AIContextFresh,
				RetrievedAt:      now,
				SchemaVersion:    AIEvidenceSchemaVersion,
			})
		}

		return ports.AICapabilityResult{
			Data: map[string]any{
				"items":       projections,
				"count":       len(projections),
				"has_more":    res.HasMore,
				"next_cursor": res.NextCursor,
			},
			CatalogEvidence: evidence,




		}, nil

	case "get_catalog_item":
		if c.GetCatalogItem == nil {
			return ports.AICapabilityResult{}, errors.New("get_catalog_item handler is not configured")
		}
		catalogID := strings.TrimSpace(params.CatalogID)
		itemID := strings.TrimSpace(params.ItemID)
		if catalogID == "" || itemID == "" {
			return ports.AICapabilityResult{}, errors.New("catalog_id and item_id are required for get_catalog_item")
		}
		item, err := c.GetCatalogItem.Handle(ctx, queries.GetCatalogItemQuery{
			Meta:      queryMeta,
			CatalogID: commands.CatalogID(catalogID),
			ItemID:    commands.CatalogItemID(itemID),
		})
		if err != nil {
			return ports.AICapabilityResult{}, err
		}
		var attrs map[string]any
		if len(item.Attributes) > 0 {
			_ = json.Unmarshal(item.Attributes, &attrs)
		}
		var schemaID *string
		if item.AttributeSchemaID != nil {
			str := string(*item.AttributeSchemaID)
			schemaID = &str
		}
		projection := CatalogItemProjection{
			ID:                     string(item.ID),
			CatalogID:              string(item.CatalogID),
			AttributeSchemaID:      schemaID,
			AttributeSchemaVersion: item.AttributeSchemaVersion,
			ItemType:               item.ItemType,
			Name:                   item.Name,
			Status:                 item.Status,
			Attributes:             attrs,
		}
		evidence := []ports.AICatalogEvidence{
			{
				Reference:        string(item.ID),
				CatalogReference: string(item.CatalogID),
				ItemType:         item.ItemType,
				Name:             item.Name,
				Status:           item.Status,
				Attributes:       safeJSONObject(item.Attributes),
				EvidenceState:    AIContextFresh,
				RetrievedAt:      now,
				SchemaVersion:    AIEvidenceSchemaVersion,
			},
		}

		return ports.AICapabilityResult{
			Data:            projection,
			CatalogEvidence: evidence,



		}, nil

	case "list_offers":
		if c.ListOffers == nil {
			return ports.AICapabilityResult{}, errors.New("list_offers handler is not configured")
		}
		itemID := strings.TrimSpace(params.ItemID)
		if itemID == "" {
			return ports.AICapabilityResult{}, errors.New("item_id is required for list_offers")
		}
		res, err := c.ListOffers.Handle(ctx, queries.ListOffersQuery{
			Meta:   queryMeta,
			ItemID: commands.CatalogItemID(itemID),
			Status: params.Status,
			Limit:  params.Limit,
			Cursor: params.Cursor,
		})
		if err != nil {
			return ports.AICapabilityResult{}, err
		}
		projections := make([]OfferProjection, 0, len(res.Items))
		evidence := make([]ports.AIOfferEvidence, 0, len(res.Items))
		for _, offer := range res.Items {
			var variantID *string
			if offer.VariantID != "" {
				str := string(offer.VariantID)
				variantID = &str
			}
			projections = append(projections, OfferProjection{
				ID:                 string(offer.ID),
				CatalogItemID:      string(offer.CatalogItemID),
				VariantID:          variantID,
				Name:               offer.Name,
				PricingMode:        offer.PricingMode,
				Amount:             offer.Amount,
				Currency:           offer.Currency,
				AvailabilityStatus: offer.AvailabilityStatus,
				Status:             offer.Status,
			})
			evState := AIContextFresh
			if strings.EqualFold(strings.TrimSpace(offer.AvailabilityStatus), "unknown") || strings.EqualFold(strings.TrimSpace(offer.AvailabilityStatus), "stale") {
				evState = AIContextStale
			}
			evidence = append(evidence, ports.AIOfferEvidence{
				Reference:            string(offer.ID),
				CatalogItemReference: string(offer.CatalogItemID),
				VariantReference:     stringValue(variantID),
				Name:                 offer.Name,
				PricingMode:          offer.PricingMode,
				Amount:               stringValue(offer.Amount),
				Currency:             stringValue(offer.Currency),
				AvailabilityState:    offer.AvailabilityStatus,
				Status:               offer.Status,
				EvidenceState:        evState,
				RetrievedAt:          now,
				SchemaVersion:        AIEvidenceSchemaVersion,
			})
		}

		return ports.AICapabilityResult{
			Data: map[string]any{
				"offers":      projections,
				"count":       len(projections),
				"has_more":    res.HasMore,
				"next_cursor": res.NextCursor,
			},
			OfferEvidence: evidence,




		}, nil

	case "list_variants":
		if c.ListVariants == nil {
			return ports.AICapabilityResult{}, errors.New("list_variants handler is not configured")
		}
		itemID := strings.TrimSpace(params.ItemID)
		if itemID == "" {
			return ports.AICapabilityResult{}, errors.New("item_id is required for list_variants")
		}
		res, err := c.ListVariants.Handle(ctx, queries.ListVariantsQuery{
			Meta:   queryMeta,
			ItemID: commands.CatalogItemID(itemID),
			Status: params.Status,
			Limit:  params.Limit,
			Cursor: params.Cursor,
		})
		if err != nil {
			return ports.AICapabilityResult{}, err
		}
		projections := make([]VariantProjection, 0, len(res.Items))
		evidence := make([]ports.AIVariantEvidence, 0, len(res.Items))
		for _, v := range res.Items {
			var attrs map[string]any
			if len(v.Attributes) > 0 {
				_ = json.Unmarshal(v.Attributes, &attrs)
			}
			projections = append(projections, VariantProjection{
				ID:            string(v.ID),
				CatalogItemID: string(v.CatalogItemID),
				Name:          v.Name,
				Status:        v.Status,
				Attributes:    attrs,
			})
			evidence = append(evidence, ports.AIVariantEvidence{
				Reference:            string(v.ID),
				CatalogItemReference: string(v.CatalogItemID),
				Name:                 v.Name,
				Status:               v.Status,
				Attributes:           safeJSONObject(v.Attributes),
				EvidenceState:        AIContextFresh,
				RetrievedAt:          now,
				SchemaVersion:        AIEvidenceSchemaVersion,
			})
		}

		return ports.AICapabilityResult{
			Data: map[string]any{
				"variants":    projections,
				"count":       len(projections),
				"has_more":    res.HasMore,
				"next_cursor": res.NextCursor,
			},
			VariantEvidence: evidence,




		}, nil

	case "get_attribute_schema":
		if c.GetAttributeSchema == nil {
			return ports.AICapabilityResult{}, errors.New("get_attribute_schema handler is not configured")
		}
		schemaID := strings.TrimSpace(params.SchemaID)
		if schemaID == "" {
			return ports.AICapabilityResult{}, errors.New("schema_id is required for get_attribute_schema")
		}
		schema, err := c.GetAttributeSchema.Handle(ctx, queries.GetAttributeSchemaQuery{
			Meta:     queryMeta,
			SchemaID: commands.AttributeSchemaID(schemaID),
		})
		if err != nil {
			return ports.AICapabilityResult{}, err
		}
		defs := make([]AttributeDefinitionProjection, 0, len(schema.Definitions))
		for _, d := range schema.Definitions {
			defs = append(defs, AttributeDefinitionProjection{
				ID:           string(d.ID),
				Key:          d.Key,
				Label:        d.Label,
				DataType:     d.DataType,
				Required:     d.Required,
				Searchable:   d.Searchable,
				DisplayOrder: d.DisplayOrder,
			})
		}

		return ports.AICapabilityResult{
			Data: AttributeSchemaProjection{
				ID:          string(schema.ID),
				Name:        schema.Name,
				Version:     schema.Version,
				Definitions: defs,
			},



		}, nil

	default:
		return ports.AICapabilityResult{}, fmt.Errorf("unsupported catalog_data operation: %q", params.Operation)
	}
}

var _ ports.AICapability = (*CatalogDataCapability)(nil)

// CatalogAuthoringCapability is an AI capability exposing governed catalog mutation operations.
// All operations strictly validate against domain rules and enforce server-side tenant isolation.
type CatalogAuthoringCapability struct {
	AuthorCatalogItem            commands.AuthorCatalogItemHandler
	CreateCatalog                commands.CreateCatalogHandler
	CreateCatalogItem            commands.CreateCatalogItemHandler
	CreateOffer                  commands.CreateOfferHandler
	CreateVariant                commands.CreateVariantHandler
	CreateAttributeSchemaVersion commands.CreateAttributeSchemaVersionHandler
	Now                          func() time.Time
}

func NewCatalogAuthoringCapability(
	authorCatalogItem commands.AuthorCatalogItemHandler,
	createCatalog commands.CreateCatalogHandler,
	createCatalogItem commands.CreateCatalogItemHandler,
	createOffer commands.CreateOfferHandler,
	createVariant commands.CreateVariantHandler,
	createAttributeSchema commands.CreateAttributeSchemaVersionHandler,
) *CatalogAuthoringCapability {
	return &CatalogAuthoringCapability{
		AuthorCatalogItem:            authorCatalogItem,
		CreateCatalog:                createCatalog,
		CreateCatalogItem:            createCatalogItem,
		CreateOffer:                  createOffer,
		CreateVariant:                createVariant,
		CreateAttributeSchemaVersion: createAttributeSchema,
	}
}

func (c *CatalogAuthoringCapability) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

func (c *CatalogAuthoringCapability) Definition() ports.AICapabilityDefinition {
	return ports.AICapabilityDefinition{
		Name: "catalog_authoring",
		Description: "Execute governed catalog authoring operations (authoring full catalog items with variants and offers, creating catalogs, items, offers, variants, or schemas) for this business. " +
			"This tool mutates merchant catalog data with full server-side domain validation and tenant isolation.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{
					"type": "string",
					"enum": []string{
						"author_catalog_item",
						"create_catalog",
						"create_catalog_item",
						"create_offer",
						"create_variant",
						"create_attribute_schema",
					},
					"description": "The explicit catalog authoring operation to perform.",
				},
				"catalog_id": map[string]any{
					"type":        "string",
					"description": "Catalog ID for item authoring/creation.",
				},
				"item_id": map[string]any{
					"type":        "string",
					"description": "Catalog item ID for offer or variant creation.",
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Name of the entity to create.",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Optional description for catalog.",
				},
				"item_type": map[string]any{
					"type":        "string",
					"description": "Item type for catalog item (e.g. product, service).",
				},
				"attribute_schema_id": map[string]any{
					"type":        "string",
					"description": "Optional attribute schema ID.",
				},
				"attributes": map[string]any{
					"type":        "object",
					"description": "Dynamic key-value attributes (JSON object).",
				},
				"pricing_mode": map[string]any{
					"type":        "string",
					"description": "Pricing mode (e.g. fixed, per_unit, custom).",
				},
				"amount": map[string]any{
					"description": "Price amount (number or string, e.g. '15000' or 15000).",
				},
				"amount_minor": map[string]any{
					"type":        "integer",
					"description": "Price amount in minor currency units (optional alternative to amount).",
				},
				"currency": map[string]any{
					"type":        "string",
					"description": "Currency code (e.g. 'YER', 'SAR', 'USD').",
				},
				"pricing_unit": map[string]any{
					"type":        "string",
					"description": "Optional pricing unit.",
				},
				"variant_id": map[string]any{
					"type":        "string",
					"description": "Optional variant ID to link offer to.",
				},
				"availability_mode": map[string]any{
					"type":        "string",
					"description": "Availability mode (e.g. in_stock).",
				},
				"availability_status": map[string]any{
					"type":        "string",
					"description": "Availability status (e.g. available).",
				},
				"fulfillment_mode": map[string]any{
					"type":        "string",
					"description": "Fulfillment mode (e.g. standard).",
				},
				"status": map[string]any{
					"type":        "string",
					"description": "Status (e.g. draft, active).",
				},
				"requires_confirmation": map[string]any{
					"type":        "boolean",
					"description": "Whether item requires merchant confirmation.",
				},
				"variants": map[string]any{
					"type":        "array",
					"description": "Array of variants for author_catalog_item.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":       map[string]any{"type": "string"},
							"attributes": map[string]any{"type": "object"},
						},
						"required": []string{"name"},
					},
				},
				"offers": map[string]any{
					"type":        "array",
					"description": "Array of offers for author_catalog_item.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":                map[string]any{"type": "string"},
							"pricing_mode":        map[string]any{"type": "string"},
							"amount":              map[string]any{"description": "Price amount (number or string)."},
							"amount_minor":        map[string]any{"type": "integer"},
							"currency":            map[string]any{"type": "string"},
							"pricing_unit":        map[string]any{"type": "string"},
							"variant_name":        map[string]any{"type": "string"},
							"variant_index":       map[string]any{"type": "integer"},
							"availability_status": map[string]any{"type": "string"},
							"fulfillment_mode":    map[string]any{"type": "string"},
							"status":              map[string]any{"type": "string"},
						},
					},
				},
				"definitions": map[string]any{
					"type":        "array",
					"description": "Attribute definitions for create_attribute_schema.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"key":           map[string]any{"type": "string"},
							"label":         map[string]any{"type": "string"},
							"data_type":     map[string]any{"type": "string"},
							"required":      map[string]any{"type": "boolean"},
							"searchable":    map[string]any{"type": "boolean"},
							"display_order": map[string]any{"type": "integer"},
						},
						"required": []string{"key", "label", "data_type"},
					},
				},
			},
			"required": []string{"operation"},
		},
	}
}

type catalogAuthoringParams struct {
	Operation            string               `json:"operation"`
	CatalogID            string               `json:"catalog_id"`
	ItemID               string               `json:"item_id"`
	Name                 string               `json:"name"`
	Description          string               `json:"description"`
	ItemType             string               `json:"item_type"`
	AttributeSchemaID    string               `json:"attribute_schema_id"`
	Attributes           map[string]any       `json:"attributes"`
	PricingMode          string               `json:"pricing_mode"`
	Amount               any                  `json:"amount"`
	AmountMinor          *int64               `json:"amount_minor"`
	Currency             string               `json:"currency"`
	PricingUnit          string               `json:"pricing_unit"`
	VariantID            string               `json:"variant_id"`
	AvailabilityMode     string               `json:"availability_mode"`
	AvailabilityStatus   string               `json:"availability_status"`
	FulfillmentMode      string               `json:"fulfillment_mode"`
	Status               string               `json:"status"`
	RequiresConfirmation bool                 `json:"requires_confirmation"`
	Variants             []authorVariantParam `json:"variants"`
	Offers               []authorOfferParam   `json:"offers"`
	Definitions          []attributeDefParam  `json:"definitions"`
}

type authorVariantParam struct {
	Name       string         `json:"name"`
	Attributes map[string]any `json:"attributes"`
}

type authorOfferParam struct {
	Name               string `json:"name"`
	PricingMode        string `json:"pricing_mode"`
	Amount             any    `json:"amount"`
	AmountMinor        *int64 `json:"amount_minor"`
	Currency           string `json:"currency"`
	PricingUnit        string `json:"pricing_unit"`
	VariantName        string `json:"variant_name"`
	VariantIndex       *int   `json:"variant_index"`
	AvailabilityStatus string `json:"availability_status"`
	FulfillmentMode    string `json:"fulfillment_mode"`
	Status             string `json:"status"`
}

type attributeDefParam struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	DataType     string `json:"data_type"`
	Required     bool   `json:"required"`
	Searchable   bool   `json:"searchable"`
	DisplayOrder int    `json:"display_order"`
}

func parseAmountToMinor(amount any, amountMinor *int64) (*int64, error) {
	if amountMinor != nil {
		return amountMinor, nil
	}
	if amount == nil {
		return nil, nil
	}
	switch v := amount.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, errors.New("amount must be finite")
		}
		scaled := v * 100
		if math.Abs(scaled-math.Round(scaled)) > 1e-7 {
			return nil, errors.New("amount supports at most two decimal places")
		}
		minor := int64(math.Round(scaled))
		return &minor, nil
	case int:
		minor := int64(v) * 100
		return &minor, nil
	case int64:
		minor := v * 100
		return &minor, nil
	case string:
		str := strings.TrimSpace(v)
		if str == "" {
			return nil, nil
		}
		str = strings.ReplaceAll(str, ",", "")
		f, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid amount %q: %w", v, err)
		}
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, errors.New("amount must be finite")
		}
		scaled := f * 100
		if math.Abs(scaled-math.Round(scaled)) > 1e-7 {
			return nil, errors.New("amount supports at most two decimal places")
		}
		minor := int64(math.Round(scaled))
		return &minor, nil
	default:
		return nil, fmt.Errorf("unsupported amount format: %T", amount)
	}
}

func (c *CatalogAuthoringCapability) Execute(ctx context.Context, execCtx ports.AICapabilityExecutionContext, rawParams []byte) (ports.AICapabilityResult, error) {
	businessID := strings.TrimSpace(execCtx.BusinessID)
	if businessID == "" {
		return ports.AICapabilityResult{}, errors.New("unauthorized: business ID is required in capability execution context")
	}

	var params catalogAuthoringParams
	if len(rawParams) > 0 {
		if err := json.Unmarshal(rawParams, &params); err != nil {
			return ports.AICapabilityResult{}, fmt.Errorf("invalid parameters for catalog_authoring: %w", err)
		}
	}

	now := c.now()
	actorCtx := commands.ActorContext{
		BusinessID:  commands.BusinessID(businessID),
		PrincipalID: commands.PrincipalID(execCtx.PrincipalID),
		Role:        execCtx.Role,
		Permissions: append([]string(nil), execCtx.Permissions...),
	}
	cmdMeta := commands.CommandMeta{
		Actor:         actorCtx,
		RequestID:     execCtx.RequestID,
		CorrelationID: execCtx.CorrelationID,
	}

	switch strings.TrimSpace(params.Operation) {
	case "author_catalog_item":
		if c.AuthorCatalogItem == nil {
			return ports.AICapabilityResult{}, errors.New("author_catalog_item handler is not configured")
		}
		catalogID := strings.TrimSpace(params.CatalogID)
		if catalogID == "" {
			return ports.AICapabilityResult{}, errors.New("catalog_id is required for author_catalog_item")
		}
		name := strings.TrimSpace(params.Name)
		if name == "" {
			return ports.AICapabilityResult{}, errors.New("name is required for author_catalog_item")
		}

		var schemaID *commands.AttributeSchemaID
		if strings.TrimSpace(params.AttributeSchemaID) != "" {
			sid := commands.AttributeSchemaID(strings.TrimSpace(params.AttributeSchemaID))
			schemaID = &sid
		}

		variants := make([]commands.AuthorVariantInput, 0, len(params.Variants))
		for _, v := range params.Variants {
			variants = append(variants, commands.AuthorVariantInput{
				Name:       v.Name,
				Attributes: v.Attributes,
			})
		}

		offers := make([]commands.AuthorOfferInput, 0, len(params.Offers))
		for _, o := range params.Offers {
			minor, err := parseAmountToMinor(o.Amount, o.AmountMinor)
			if err != nil {
				return ports.AICapabilityResult{}, fmt.Errorf("invalid offer amount: %w", err)
			}
			var currency *string
			if strings.TrimSpace(o.Currency) != "" {
				c := strings.TrimSpace(o.Currency)
				currency = &c
			}
			var pricingUnit *string
			if strings.TrimSpace(o.PricingUnit) != "" {
				pu := strings.TrimSpace(o.PricingUnit)
				pricingUnit = &pu
			}
			var variantName *string
			if strings.TrimSpace(o.VariantName) != "" {
				vn := strings.TrimSpace(o.VariantName)
				variantName = &vn
			}

			offers = append(offers, commands.AuthorOfferInput{
				Name:               o.Name,
				PricingMode:        o.PricingMode,
				AmountMinor:        minor,
				Currency:           currency,
				PricingUnit:        pricingUnit,
				VariantName:        variantName,
				VariantIndex:       o.VariantIndex,
				AvailabilityStatus: o.AvailabilityStatus,
				FulfillmentMode:    o.FulfillmentMode,
				Status:             o.Status,
			})
		}

		// If top-level amount is provided and offers list is empty, construct a default offer
		if len(offers) == 0 && (params.Amount != nil || params.AmountMinor != nil) {
			minor, err := parseAmountToMinor(params.Amount, params.AmountMinor)
			if err != nil {
				return ports.AICapabilityResult{}, fmt.Errorf("invalid amount: %w", err)
			}
			var currency *string
			if strings.TrimSpace(params.Currency) != "" {
				c := strings.TrimSpace(params.Currency)
				currency = &c
			}
			var pricingUnit *string
			if strings.TrimSpace(params.PricingUnit) != "" {
				pu := strings.TrimSpace(params.PricingUnit)
				pricingUnit = &pu
			}
			offers = append(offers, commands.AuthorOfferInput{
				Name:               name,
				PricingMode:        params.PricingMode,
				AmountMinor:        minor,
				Currency:           currency,
				PricingUnit:        pricingUnit,
				AvailabilityStatus: params.AvailabilityStatus,
				FulfillmentMode:    params.FulfillmentMode,
				Status:             params.Status,
			})
		}

		res, err := c.AuthorCatalogItem.Handle(ctx, commands.AuthorCatalogItemCommand{
			Meta:                 cmdMeta,
			CatalogID:            commands.CatalogID(catalogID),
			AttributeSchemaID:    schemaID,
			ItemType:             params.ItemType,
			Name:                 name,
			PricingMode:          params.PricingMode,
			AvailabilityMode:     params.AvailabilityMode,
			FulfillmentMode:      params.FulfillmentMode,
			RequiresConfirmation: params.RequiresConfirmation,
			Attributes:           params.Attributes,
			Variants:             variants,
			Offers:               offers,
		})
		if err != nil {
			return ports.AICapabilityResult{}, err
		}

		var itemAttrs map[string]any
		if len(res.Item.Attributes) > 0 {
			_ = json.Unmarshal(res.Item.Attributes, &itemAttrs)
		}
		var retSchemaID *string
		if res.Item.AttributeSchemaID != nil {
			str := string(*res.Item.AttributeSchemaID)
			retSchemaID = &str
		}

		itemProjection := CatalogItemProjection{
			ID:                     string(res.Item.ID),
			CatalogID:              string(res.Item.CatalogID),
			AttributeSchemaID:      retSchemaID,
			AttributeSchemaVersion: res.Item.AttributeSchemaVersion,
			ItemType:               res.Item.ItemType,
			Name:                   res.Item.Name,
			Status:                 res.Item.Status,
			Attributes:             itemAttrs,
		}

		catEvidence := []ports.AICatalogEvidence{
			{
				Reference:        string(res.Item.ID),
				CatalogReference: string(res.Item.CatalogID),
				ItemType:         res.Item.ItemType,
				Name:             res.Item.Name,
				Status:           res.Item.Status,
				Attributes:       safeJSONObject(res.Item.Attributes),
				EvidenceState:    AIContextFresh,
				RetrievedAt:      now,
				SchemaVersion:    AIEvidenceSchemaVersion,
			},
		}

		variantProjections := make([]VariantProjection, 0, len(res.Variants))
		variantEvidence := make([]ports.AIVariantEvidence, 0, len(res.Variants))
		for _, v := range res.Variants {
			var vAttrs map[string]any
			if len(v.Attributes) > 0 {
				_ = json.Unmarshal(v.Attributes, &vAttrs)
			}
			variantProjections = append(variantProjections, VariantProjection{
				ID:            string(v.ID),
				CatalogItemID: string(v.CatalogItemID),
				Name:          v.Name,
				Status:        v.Status,
				Attributes:    vAttrs,
			})
			variantEvidence = append(variantEvidence, ports.AIVariantEvidence{
				Reference:            string(v.ID),
				CatalogItemReference: string(v.CatalogItemID),
				Name:                 v.Name,
				Status:               v.Status,
				Attributes:           safeJSONObject(v.Attributes),
				EvidenceState:        AIContextFresh,
				RetrievedAt:          now,
				SchemaVersion:        AIEvidenceSchemaVersion,
			})
		}

		offerProjections := make([]OfferProjection, 0, len(res.Offers))
		offerEvidence := make([]ports.AIOfferEvidence, 0, len(res.Offers))
		for _, o := range res.Offers {
			var vid *string
			if o.VariantID != "" {
				str := string(o.VariantID)
				vid = &str
			}
			offerProjections = append(offerProjections, OfferProjection{
				ID:                 string(o.ID),
				CatalogItemID:      string(o.CatalogItemID),
				VariantID:          vid,
				Name:               o.Name,
				PricingMode:        o.PricingMode,
				Amount:             o.Amount,
				Currency:           o.Currency,
				AvailabilityStatus: o.AvailabilityStatus,
				Status:             o.Status,
			})
			offerEvidence = append(offerEvidence, ports.AIOfferEvidence{
				Reference:            string(o.ID),
				CatalogItemReference: string(o.CatalogItemID),
				VariantReference:     stringValue(vid),
				Name:                 o.Name,
				PricingMode:          o.PricingMode,
				Amount:               stringValue(o.Amount),
				Currency:             stringValue(o.Currency),
				AvailabilityState:    o.AvailabilityStatus,
				Status:               o.Status,
				EvidenceState:        AIContextFresh,
				RetrievedAt:          now,
				SchemaVersion:        AIEvidenceSchemaVersion,
			})
		}

		return ports.AICapabilityResult{
			Data: map[string]any{
				"item":     itemProjection,
				"variants": variantProjections,
				"offers":   offerProjections,
				"status":   res.Status,
			},
			CatalogEvidence: catEvidence,
			VariantEvidence: variantEvidence,
			OfferEvidence:   offerEvidence,


		}, nil

	case "create_catalog":
		if c.CreateCatalog == nil {
			return ports.AICapabilityResult{}, errors.New("create_catalog handler is not configured")
		}
		name := strings.TrimSpace(params.Name)
		if name == "" {
			return ports.AICapabilityResult{}, errors.New("name is required for create_catalog")
		}
		res, err := c.CreateCatalog.Handle(ctx, commands.CreateCatalogCommand{
			Meta:        cmdMeta,
			Name:        name,
			Description: params.Description,
		})
		if err != nil {
			return ports.AICapabilityResult{}, err
		}
		return ports.AICapabilityResult{
			Data: CatalogProjection{
				ID:          string(res.Catalog.ID),
				Name:        res.Catalog.Name,
				Description: res.Catalog.Description,
				Status:      res.Catalog.Status,
			},

		}, nil

	case "create_catalog_item":
		if c.CreateCatalogItem == nil {
			return ports.AICapabilityResult{}, errors.New("create_catalog_item handler is not configured")
		}
		catalogID := strings.TrimSpace(params.CatalogID)
		if catalogID == "" {
			return ports.AICapabilityResult{}, errors.New("catalog_id is required for create_catalog_item")
		}
		name := strings.TrimSpace(params.Name)
		if name == "" {
			return ports.AICapabilityResult{}, errors.New("name is required for create_catalog_item")
		}
		itemType := strings.TrimSpace(params.ItemType)
		if itemType == "" {
			itemType = "product"
		}
		pricingMode := strings.TrimSpace(params.PricingMode)
		if pricingMode == "" {
			pricingMode = "fixed"
		}
		availabilityMode := strings.TrimSpace(params.AvailabilityMode)
		if availabilityMode == "" {
			availabilityMode = "in_stock"
		}
		fulfillmentMode := strings.TrimSpace(params.FulfillmentMode)
		if fulfillmentMode == "" {
			fulfillmentMode = "standard"
		}

		var schemaID *commands.AttributeSchemaID
		if strings.TrimSpace(params.AttributeSchemaID) != "" {
			sid := commands.AttributeSchemaID(strings.TrimSpace(params.AttributeSchemaID))
			schemaID = &sid
		}

		res, err := c.CreateCatalogItem.Handle(ctx, commands.CreateCatalogItemCommand{
			Meta:                 cmdMeta,
			CatalogID:            commands.CatalogID(catalogID),
			AttributeSchemaID:    schemaID,
			ItemType:             itemType,
			Name:                 name,
			PricingMode:          pricingMode,
			AvailabilityMode:     availabilityMode,
			FulfillmentMode:      fulfillmentMode,
			RequiresConfirmation: params.RequiresConfirmation,
			Attributes:           params.Attributes,
		})
		if err != nil {
			return ports.AICapabilityResult{}, err
		}

		var attrs map[string]any
		if len(res.Item.Attributes) > 0 {
			_ = json.Unmarshal(res.Item.Attributes, &attrs)
		}
		var retSchemaID *string
		if res.Item.AttributeSchemaID != nil {
			str := string(*res.Item.AttributeSchemaID)
			retSchemaID = &str
		}
		projection := CatalogItemProjection{
			ID:                     string(res.Item.ID),
			CatalogID:              string(res.Item.CatalogID),
			AttributeSchemaID:      retSchemaID,
			AttributeSchemaVersion: res.Item.AttributeSchemaVersion,
			ItemType:               res.Item.ItemType,
			Name:                   res.Item.Name,
			Status:                 res.Item.Status,
			Attributes:             attrs,
		}
		catEvidence := []ports.AICatalogEvidence{
			{
				Reference:        string(res.Item.ID),
				CatalogReference: string(res.Item.CatalogID),
				ItemType:         res.Item.ItemType,
				Name:             res.Item.Name,
				Status:           res.Item.Status,
				Attributes:       safeJSONObject(res.Item.Attributes),
				EvidenceState:    AIContextFresh,
				RetrievedAt:      now,
				SchemaVersion:    AIEvidenceSchemaVersion,
			},
		}
		return ports.AICapabilityResult{
			Data:            projection,
			CatalogEvidence: catEvidence,

		}, nil

	case "create_offer":
		if c.CreateOffer == nil {
			return ports.AICapabilityResult{}, errors.New("create_offer handler is not configured")
		}
		itemID := strings.TrimSpace(params.ItemID)
		if itemID == "" {
			return ports.AICapabilityResult{}, errors.New("item_id is required for create_offer")
		}
		name := strings.TrimSpace(params.Name)
		if name == "" {
			return ports.AICapabilityResult{}, errors.New("name is required for create_offer")
		}
		pricingMode := strings.TrimSpace(params.PricingMode)
		if pricingMode == "" {
			pricingMode = "fixed"
		}
		minor, err := parseAmountToMinor(params.Amount, params.AmountMinor)
		if err != nil {
			return ports.AICapabilityResult{}, fmt.Errorf("invalid offer amount: %w", err)
		}
		var currency *string
		if strings.TrimSpace(params.Currency) != "" {
			c := strings.TrimSpace(params.Currency)
			currency = &c
		}
		var pricingUnit *string
		if strings.TrimSpace(params.PricingUnit) != "" {
			pu := strings.TrimSpace(params.PricingUnit)
			pricingUnit = &pu
		}
		var variantID *commands.VariantID
		if strings.TrimSpace(params.VariantID) != "" {
			vid := commands.VariantID(strings.TrimSpace(params.VariantID))
			variantID = &vid
		}

		availMode := strings.TrimSpace(params.AvailabilityMode)
		if availMode == "" {
			availMode = "in_stock"
		}
		availStatus := strings.TrimSpace(params.AvailabilityStatus)
		if availStatus == "" {
			availStatus = "available"
		}
		fulfillMode := strings.TrimSpace(params.FulfillmentMode)
		if fulfillMode == "" {
			fulfillMode = "standard"
		}
		status := strings.TrimSpace(params.Status)
		if status == "" {
			status = "active"
		}

		res, err := c.CreateOffer.Handle(ctx, commands.CreateOfferCommand{
			Meta:               cmdMeta,
			CatalogItemID:      commands.CatalogItemID(itemID),
			VariantID:          variantID,
			Name:               name,
			PricingMode:        pricingMode,
			AmountMinor:        minor,
			Currency:           currency,
			PricingUnit:        pricingUnit,
			AvailabilityMode:   availMode,
			AvailabilityStatus: availStatus,
			FulfillmentMode:    fulfillMode,
			Status:             status,
		})
		if err != nil {
			return ports.AICapabilityResult{}, err
		}

		var retVid *string
		if res.Offer.VariantID != "" {
			str := string(res.Offer.VariantID)
			retVid = &str
		}
		projection := OfferProjection{
			ID:                 string(res.Offer.ID),
			CatalogItemID:      string(res.Offer.CatalogItemID),
			VariantID:          retVid,
			Name:               res.Offer.Name,
			PricingMode:        res.Offer.PricingMode,
			Amount:             res.Offer.Amount,
			Currency:           res.Offer.Currency,
			AvailabilityStatus: res.Offer.AvailabilityStatus,
			Status:             res.Offer.Status,
		}
		offerEvidence := []ports.AIOfferEvidence{
			{
				Reference:            string(res.Offer.ID),
				CatalogItemReference: string(res.Offer.CatalogItemID),
				VariantReference:     stringValue(retVid),
				Name:                 res.Offer.Name,
				PricingMode:          res.Offer.PricingMode,
				Amount:               stringValue(res.Offer.Amount),
				Currency:             stringValue(res.Offer.Currency),
				AvailabilityState:    res.Offer.AvailabilityStatus,
				Status:               res.Offer.Status,
				EvidenceState:        AIContextFresh,
				RetrievedAt:          now,
				SchemaVersion:        AIEvidenceSchemaVersion,
			},
		}
		return ports.AICapabilityResult{
			Data:          projection,
			OfferEvidence: offerEvidence,

		}, nil

	case "create_variant":
		if c.CreateVariant == nil {
			return ports.AICapabilityResult{}, errors.New("create_variant handler is not configured")
		}
		itemID := strings.TrimSpace(params.ItemID)
		if itemID == "" {
			return ports.AICapabilityResult{}, errors.New("item_id is required for create_variant")
		}
		name := strings.TrimSpace(params.Name)
		if name == "" {
			return ports.AICapabilityResult{}, errors.New("name is required for create_variant")
		}

		res, err := c.CreateVariant.Handle(ctx, commands.CreateVariantCommand{
			Meta:          cmdMeta,
			CatalogItemID: commands.CatalogItemID(itemID),
			Name:          name,
			Attributes:    params.Attributes,
		})
		if err != nil {
			return ports.AICapabilityResult{}, err
		}

		var attrs map[string]any
		if len(res.Variant.Attributes) > 0 {
			_ = json.Unmarshal(res.Variant.Attributes, &attrs)
		}
		projection := VariantProjection{
			ID:            string(res.Variant.ID),
			CatalogItemID: string(res.Variant.CatalogItemID),
			Name:          res.Variant.Name,
			Status:        res.Variant.Status,
			Attributes:    attrs,
		}
		variantEvidence := []ports.AIVariantEvidence{
			{
				Reference:            string(res.Variant.ID),
				CatalogItemReference: string(res.Variant.CatalogItemID),
				Name:                 res.Variant.Name,
				Status:               res.Variant.Status,
				Attributes:           safeJSONObject(res.Variant.Attributes),
				EvidenceState:        AIContextFresh,
				RetrievedAt:          now,
				SchemaVersion:        AIEvidenceSchemaVersion,
			},
		}
		return ports.AICapabilityResult{
			Data:            projection,
			VariantEvidence: variantEvidence,

		}, nil

	case "create_attribute_schema":
		if c.CreateAttributeSchemaVersion == nil {
			return ports.AICapabilityResult{}, errors.New("create_attribute_schema handler is not configured")
		}
		name := strings.TrimSpace(params.Name)
		if name == "" {
			return ports.AICapabilityResult{}, errors.New("name is required for create_attribute_schema")
		}
		defs := make([]commands.AttributeDefinition, 0, len(params.Definitions))
		for _, d := range params.Definitions {
			defs = append(defs, commands.AttributeDefinition{
				Key:          d.Key,
				Label:        d.Label,
				DataType:     d.DataType,
				Required:     d.Required,
				Searchable:   d.Searchable,
				DisplayOrder: d.DisplayOrder,
			})
		}
		res, err := c.CreateAttributeSchemaVersion.Handle(ctx, commands.CreateAttributeSchemaVersionCommand{
			Meta:        cmdMeta,
			Name:        name,
			Definitions: defs,
		})
		if err != nil {
			return ports.AICapabilityResult{}, err
		}

		defProjections := make([]AttributeDefinitionProjection, 0, len(res.Schema.Definitions))
		for _, d := range res.Schema.Definitions {
			defProjections = append(defProjections, AttributeDefinitionProjection{
				ID:           string(d.ID),
				Key:          d.Key,
				Label:        d.Label,
				DataType:     d.DataType,
				Required:     d.Required,
				Searchable:   d.Searchable,
				DisplayOrder: d.DisplayOrder,
			})
		}
		return ports.AICapabilityResult{
			Data: AttributeSchemaProjection{
				ID:          string(res.Schema.ID),
				Name:        res.Schema.Name,
				Version:     res.Schema.Version,
				Definitions: defProjections,
			},

		}, nil

	default:
		return ports.AICapabilityResult{}, fmt.Errorf("unsupported catalog_authoring operation: %q", params.Operation)
	}
}

var _ ports.AICapability = (*CatalogAuthoringCapability)(nil)

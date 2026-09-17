package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		streamKey := fmt.Sprintf("list_catalogs:status=%s", params.Status)
		return ports.AICapabilityResult{
			Data: map[string]any{
				"catalogs":    projections,
				"count":       len(projections),
				"has_more":    res.HasMore,
				"next_cursor": res.NextCursor,
			},
			HasMore:    res.HasMore,
			NextCursor: res.NextCursor,
			Operation:  "list_catalogs",
			StreamKey:  streamKey,
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
		streamKey := fmt.Sprintf("list_catalog_items:catalog=%s:status=%s", catalogID, params.Status)
		return ports.AICapabilityResult{
			Data: map[string]any{
				"items":       projections,
				"count":       len(projections),
				"has_more":    res.HasMore,
				"next_cursor": res.NextCursor,
			},
			CatalogEvidence: evidence,
			HasMore:         res.HasMore,
			NextCursor:      res.NextCursor,
			Operation:       "list_catalog_items",
			StreamKey:       streamKey,
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
		streamKey := fmt.Sprintf("get_catalog_item:catalog=%s:item=%s", catalogID, itemID)
		return ports.AICapabilityResult{
			Data:            projection,
			CatalogEvidence: evidence,
			HasMore:         false,
			Operation:       "get_catalog_item",
			StreamKey:       streamKey,
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
		streamKey := fmt.Sprintf("list_offers:item=%s:status=%s", itemID, params.Status)
		return ports.AICapabilityResult{
			Data: map[string]any{
				"offers":      projections,
				"count":       len(projections),
				"has_more":    res.HasMore,
				"next_cursor": res.NextCursor,
			},
			OfferEvidence: evidence,
			HasMore:       res.HasMore,
			NextCursor:    res.NextCursor,
			Operation:     "list_offers",
			StreamKey:     streamKey,
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
		streamKey := fmt.Sprintf("list_variants:item=%s:status=%s", itemID, params.Status)
		return ports.AICapabilityResult{
			Data: map[string]any{
				"variants":    projections,
				"count":       len(projections),
				"has_more":    res.HasMore,
				"next_cursor": res.NextCursor,
			},
			VariantEvidence: evidence,
			HasMore:         res.HasMore,
			NextCursor:      res.NextCursor,
			Operation:       "list_variants",
			StreamKey:       streamKey,
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
		streamKey := fmt.Sprintf("get_attribute_schema:schema=%s", schemaID)
		return ports.AICapabilityResult{
			Data: AttributeSchemaProjection{
				ID:          string(schema.ID),
				Name:        schema.Name,
				Version:     schema.Version,
				Definitions: defs,
			},
			HasMore:   false,
			Operation: "get_attribute_schema",
			StreamKey: streamKey,
		}, nil

	default:
		return ports.AICapabilityResult{}, fmt.Errorf("unsupported catalog_data operation: %q", params.Operation)
	}
}

var _ ports.AICapability = (*CatalogDataCapability)(nil)

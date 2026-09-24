package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type mockListCatalogsHandler struct {
	fn func(ctx context.Context, q queries.ListCatalogsQuery) (commands.ListResult[commands.CatalogView], error)
}

func (m mockListCatalogsHandler) Handle(ctx context.Context, q queries.ListCatalogsQuery) (commands.ListResult[commands.CatalogView], error) {
	return m.fn(ctx, q)
}

type mockListCatalogItemsHandler struct {
	fn func(ctx context.Context, q queries.ListCatalogItemsQuery) (commands.ListResult[commands.CatalogItemView], error)
}

func (m mockListCatalogItemsHandler) Handle(ctx context.Context, q queries.ListCatalogItemsQuery) (commands.ListResult[commands.CatalogItemView], error) {
	return m.fn(ctx, q)
}

type mockGetCatalogItemHandler struct {
	fn func(ctx context.Context, q queries.GetCatalogItemQuery) (commands.CatalogItemView, error)
}

func (m mockGetCatalogItemHandler) Handle(ctx context.Context, q queries.GetCatalogItemQuery) (commands.CatalogItemView, error) {
	return m.fn(ctx, q)
}

type mockListOffersHandler struct {
	fn func(ctx context.Context, q queries.ListOffersQuery) (commands.ListResult[commands.OfferView], error)
}

func (m mockListOffersHandler) Handle(ctx context.Context, q queries.ListOffersQuery) (commands.ListResult[commands.OfferView], error) {
	return m.fn(ctx, q)
}

type mockListVariantsHandler struct {
	fn func(ctx context.Context, q queries.ListVariantsQuery) (commands.ListResult[commands.VariantView], error)
}

func (m mockListVariantsHandler) Handle(ctx context.Context, q queries.ListVariantsQuery) (commands.ListResult[commands.VariantView], error) {
	return m.fn(ctx, q)
}

type mockGetAttributeSchemaHandler struct {
	fn func(ctx context.Context, q queries.GetAttributeSchemaQuery) (commands.AttributeSchemaView, error)
}

func (m mockGetAttributeSchemaHandler) Handle(ctx context.Context, q queries.GetAttributeSchemaQuery) (commands.AttributeSchemaView, error) {
	return m.fn(ctx, q)
}

func TestCapabilityRegistry(t *testing.T) {
	registry := NewCapabilityRegistry()

	cap := &CatalogDataCapability{}
	if err := registry.Register(cap); err != nil {
		t.Fatalf("register capability: %v", err)
	}

	// Duplicate registration must fail
	if err := registry.Register(cap); err == nil {
		t.Fatal("expected duplicate registration error, got nil")
	}

	found, ok := registry.Get("catalog_data")
	if !ok || found == nil {
		t.Fatal("expected to find catalog_data capability")
	}

	defs := registry.Definitions()
	if len(defs) != 1 || defs[0].Name != "catalog_data" {
		t.Fatalf("unexpected definitions: %#v", defs)
	}

	// Dispatch unknown capability
	_, err := registry.Execute(context.Background(), ports.AICapabilityExecutionContext{BusinessID: "biz-1"}, "non_existent", nil)
	if err == nil {
		t.Fatal("expected unknown capability error, got nil")
	}
}

func TestCatalogDataCapabilityTenantDefense(t *testing.T) {
	cap := &CatalogDataCapability{}

	// Empty BusinessID must be rejected
	_, err := cap.Execute(context.Background(), ports.AICapabilityExecutionContext{BusinessID: ""}, []byte(`{"operation":"list_catalogs"}`))
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized error for empty BusinessID, got %v", err)
	}
}

func TestCatalogDataCapabilityOperations(t *testing.T) {
	const testBiz = "biz-123"
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	cap := &CatalogDataCapability{
		Now: func() time.Time { return now },
		ListCatalogs: mockListCatalogsHandler{
			fn: func(ctx context.Context, q queries.ListCatalogsQuery) (commands.ListResult[commands.CatalogView], error) {
				if string(q.Meta.Actor.BusinessID) != testBiz {
					t.Fatalf("business ID mismatch: %v", q.Meta.Actor.BusinessID)
				}
				return commands.ListResult[commands.CatalogView]{
					Items: []commands.CatalogView{
						{ID: "cat-1", Name: "Main Catalog", Status: "active"},
					},
				}, nil
			},
		},
		ListCatalogItems: mockListCatalogItemsHandler{
			fn: func(ctx context.Context, q queries.ListCatalogItemsQuery) (commands.ListResult[commands.CatalogItemView], error) {
				if string(q.Meta.Actor.BusinessID) != testBiz || string(q.CatalogID) != "cat-1" {
					t.Fatalf("unexpected list items query: %#v", q)
				}
				return commands.ListResult[commands.CatalogItemView]{
					Items: []commands.CatalogItemView{
						{
							ID:         "item-1",
							CatalogID:  "cat-1",
							ItemType:   "device",
							Name:       "Phone 15",
							Status:     "active",
							Attributes: []byte(`{"color":"black"}`),
						},
					},
				}, nil
			},
		},
		GetCatalogItem: mockGetCatalogItemHandler{
			fn: func(ctx context.Context, q queries.GetCatalogItemQuery) (commands.CatalogItemView, error) {
				if string(q.Meta.Actor.BusinessID) != testBiz || string(q.CatalogID) != "cat-1" || string(q.ItemID) != "item-1" {
					t.Fatalf("unexpected get item query: %#v", q)
				}
				return commands.CatalogItemView{
					ID:         "item-1",
					CatalogID:  "cat-1",
					ItemType:   "device",
					Name:       "Phone 15",
					Status:     "active",
					Attributes: []byte(`{"color":"black"}`),
				}, nil
			},
		},
		ListOffers: mockListOffersHandler{
			fn: func(ctx context.Context, q queries.ListOffersQuery) (commands.ListResult[commands.OfferView], error) {
				if string(q.Meta.Actor.BusinessID) != testBiz || string(q.ItemID) != "item-1" {
					t.Fatalf("unexpected list offers query: %#v", q)
				}
				amount := "250000"
				currency := "YER"
				return commands.ListResult[commands.OfferView]{
					Items: []commands.OfferView{
						{
							ID:                 "offer-1",
							CatalogItemID:      "item-1",
							Name:               "Standard Offer",
							PricingMode:        "fixed",
							Amount:             &amount,
							Currency:           &currency,
							AvailabilityStatus: "available",
							Status:             "active",
						},
					},
				}, nil
			},
		},
		ListVariants: mockListVariantsHandler{
			fn: func(ctx context.Context, q queries.ListVariantsQuery) (commands.ListResult[commands.VariantView], error) {
				if string(q.Meta.Actor.BusinessID) != testBiz || string(q.ItemID) != "item-1" {
					t.Fatalf("unexpected list variants query: %#v", q)
				}
				return commands.ListResult[commands.VariantView]{
					Items: []commands.VariantView{
						{
							ID:            "var-1",
							CatalogItemID: "item-1",
							Name:          "256GB Black",
							Status:        "active",
							Attributes:    []byte(`{"storage":"256GB"}`),
						},
					},
				}, nil
			},
		},
		GetAttributeSchema: mockGetAttributeSchemaHandler{
			fn: func(ctx context.Context, q queries.GetAttributeSchemaQuery) (commands.AttributeSchemaView, error) {
				if string(q.Meta.Actor.BusinessID) != testBiz || string(q.SchemaID) != "schema-1" {
					t.Fatalf("unexpected get schema query: %#v", q)
				}
				return commands.AttributeSchemaView{
					ID:      "schema-1",
					Name:    "Phone Schema",
					Version: 1,
					Definitions: []commands.AttributeDefinitionView{
						{ID: "def-1", Key: "storage", Label: "Storage", DataType: "text", Required: true},
					},
				}, nil
			},
		},
	}

	execCtx := ports.AICapabilityExecutionContext{BusinessID: testBiz, ConversationID: "conv-1"}

	// 1. list_catalogs
	res1, err := cap.Execute(context.Background(), execCtx, []byte(`{"operation":"list_catalogs"}`))
	if err != nil {
		t.Fatalf("list_catalogs: %v", err)
	}
	data1, _ := json.Marshal(res1.Data)
	if !strings.Contains(string(data1), "Main Catalog") {
		t.Fatalf("unexpected list_catalogs result: %s", string(data1))
	}

	// 2. list_catalog_items
	res2, err := cap.Execute(context.Background(), execCtx, []byte(`{"operation":"list_catalog_items","catalog_id":"cat-1"}`))
	if err != nil {
		t.Fatalf("list_catalog_items: %v", err)
	}
	if len(res2.CatalogEvidence) != 1 || res2.CatalogEvidence[0].Reference != "item-1" {
		t.Fatalf("unexpected catalog evidence: %#v", res2.CatalogEvidence)
	}

	// 3. get_catalog_item
	res3, err := cap.Execute(context.Background(), execCtx, []byte(`{"operation":"get_catalog_item","catalog_id":"cat-1","item_id":"item-1"}`))
	if err != nil {
		t.Fatalf("get_catalog_item: %v", err)
	}
	if len(res3.CatalogEvidence) != 1 || res3.CatalogEvidence[0].Name != "Phone 15" {
		t.Fatalf("unexpected get_catalog_item evidence: %#v", res3.CatalogEvidence)
	}

	// 4. list_offers
	res4, err := cap.Execute(context.Background(), execCtx, []byte(`{"operation":"list_offers","item_id":"item-1"}`))
	if err != nil {
		t.Fatalf("list_offers: %v", err)
	}
	if len(res4.OfferEvidence) != 1 || res4.OfferEvidence[0].Reference != "offer-1" || res4.OfferEvidence[0].Amount != "250000" {
		t.Fatalf("unexpected offer evidence: %#v", res4.OfferEvidence)
	}

	// 5. list_variants
	res5, err := cap.Execute(context.Background(), execCtx, []byte(`{"operation":"list_variants","item_id":"item-1"}`))
	if err != nil {
		t.Fatalf("list_variants: %v", err)
	}
	if len(res5.VariantEvidence) != 1 || res5.VariantEvidence[0].Reference != "var-1" {
		t.Fatalf("unexpected variant evidence: %#v", res5.VariantEvidence)
	}

	// 6. get_attribute_schema
	res6, err := cap.Execute(context.Background(), execCtx, []byte(`{"operation":"get_attribute_schema","schema_id":"schema-1"}`))
	if err != nil {
		t.Fatalf("get_attribute_schema: %v", err)
	}
	data6, _ := json.Marshal(res6.Data)
	if !strings.Contains(string(data6), "Phone Schema") {
		t.Fatalf("unexpected get_attribute_schema result: %s", string(data6))
	}

	// 7. unsupported operation
	_, err = cap.Execute(context.Background(), execCtx, []byte(`{"operation":"unknown_op"}`))
	if err == nil {
		t.Fatal("expected error for unsupported operation")
	}
}

func TestIncorporateCapabilityEvidence(t *testing.T) {
	aiCtx := &ports.AIContext{
		CatalogEvidence: []ports.AICatalogEvidence{
			{Reference: "item-1", Name: "Existing Item"},
		},
		OfferEvidence: []ports.AIOfferEvidence{
			{Reference: "offer-1", Name: "Existing Offer"},
		},
	}

	result := ports.AICapabilityResult{
		CatalogEvidence: []ports.AICatalogEvidence{
			{Reference: "item-1", Name: "Duplicate Item"},
			{Reference: "item-2", Name: "New Item"},
		},
		OfferEvidence: []ports.AIOfferEvidence{
			{Reference: "offer-2", Name: "New Offer"},
		},
		VariantEvidence: []ports.AIVariantEvidence{
			{Reference: "var-1", Name: "New Variant"},
		},
	}

	IncorporateCapabilityEvidence(aiCtx, result)

	if len(aiCtx.CatalogEvidence) != 2 {
		t.Fatalf("expected 2 catalog items, got %d", len(aiCtx.CatalogEvidence))
	}
	if aiCtx.CatalogEvidence[1].Reference != "item-2" {
		t.Fatalf("expected item-2 to be appended, got %s", aiCtx.CatalogEvidence[1].Reference)
	}
	if len(aiCtx.OfferEvidence) != 2 {
		t.Fatalf("expected 2 offers, got %d", len(aiCtx.OfferEvidence))
	}
	if len(aiCtx.VariantEvidence) != 1 {
		t.Fatalf("expected 1 variant, got %d", len(aiCtx.VariantEvidence))
	}
}

func TestCatalogDataCapabilityPaginationAndContinuation(t *testing.T) {
	const testBiz = "biz-paged"
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	cap := &CatalogDataCapability{
		Now: func() time.Time { return now },
		ListCatalogItems: mockListCatalogItemsHandler{
			fn: func(ctx context.Context, q queries.ListCatalogItemsQuery) (commands.ListResult[commands.CatalogItemView], error) {
				if q.Cursor == "" {
					return commands.ListResult[commands.CatalogItemView]{
						Items: []commands.CatalogItemView{
							{ID: "item-1", CatalogID: "cat-1", ItemType: "device", Name: "Phone 15", Status: "active"},
						},
						NextCursor: "cursor-page-2",
						HasMore:    true,
					}, nil
				}
				if q.Cursor == "cursor-page-2" {
					return commands.ListResult[commands.CatalogItemView]{
						Items: []commands.CatalogItemView{
							{ID: "item-2", CatalogID: "cat-1", ItemType: "device", Name: "Phone 16", Status: "active"},
						},
						NextCursor: "",
						HasMore:    false,
					}, nil
				}
				return commands.ListResult[commands.CatalogItemView]{}, nil
			},
		},
	}

	execCtx := ports.AICapabilityExecutionContext{BusinessID: testBiz, ConversationID: "conv-1"}

	// Page 1: returns has_more=true and next_cursor
	res1, err := cap.Execute(context.Background(), execCtx, []byte(`{"operation":"list_catalog_items","catalog_id":"cat-1","limit":1}`))
	if err != nil {
		t.Fatalf("page 1 failed: %v", err)
	}
	if !res1.Data.(map[string]any)["has_more"].(bool) || res1.Data.(map[string]any)["next_cursor"].(string) != "cursor-page-2" {
		t.Fatalf("expected HasMore=true and NextCursor='cursor-page-2', got HasMore=%v, NextCursor=%s", res1.Data.(map[string]any)["has_more"].(bool), res1.Data.(map[string]any)["next_cursor"].(string))
	}
	data1, ok := res1.Data.(map[string]any)
	if !ok || data1["has_more"] != true || data1["next_cursor"] != "cursor-page-2" {
		t.Fatalf("expected Data map to contain pagination fields: %#v", data1)
	}

	// Page 2: returns has_more=false
	res2, err := cap.Execute(context.Background(), execCtx, []byte(`{"operation":"list_catalog_items","catalog_id":"cat-1","limit":1,"cursor":"cursor-page-2"}`))
	if err != nil {
		t.Fatalf("page 2 failed: %v", err)
	}
	if res2.Data.(map[string]any)["has_more"].(bool) || res2.Data.(map[string]any)["next_cursor"].(string) != "" {
		t.Fatalf("expected HasMore=false and NextCursor='', got HasMore=%v, NextCursor=%s", res2.Data.(map[string]any)["has_more"].(bool), res2.Data.(map[string]any)["next_cursor"].(string))
	}
	data2, ok := res2.Data.(map[string]any)
	if !ok || data2["has_more"] != false {
		t.Fatalf("expected Data map has_more=false on final page: %#v", data2)
	}
}

func TestCatalogDataCapabilityTenantIsolationNeverOverriddenByAI(t *testing.T) {
	const trustedBiz = "trusted-tenant-123"
	receivedBiz := ""

	cap := &CatalogDataCapability{
		ListCatalogs: mockListCatalogsHandler{
			fn: func(ctx context.Context, q queries.ListCatalogsQuery) (commands.ListResult[commands.CatalogView], error) {
				receivedBiz = string(q.Meta.Actor.BusinessID)
				return commands.ListResult[commands.CatalogView]{
					Items: []commands.CatalogView{{ID: "cat-1", Name: "Trusted Catalog"}},
				}, nil
			},
		},
	}

	execCtx := ports.AICapabilityExecutionContext{BusinessID: trustedBiz, ConversationID: "conv-1"}

	// AI maliciously attempts to inject business_id into parameters
	_, err := cap.Execute(context.Background(), execCtx, []byte(`{"operation":"list_catalogs","business_id":"attacker-tenant-666"}`))
	if err != nil {
		t.Fatalf("execution failed: %v", err)
	}
	if receivedBiz != trustedBiz {
		t.Fatalf("SECURITY VIOLATION: query received business ID %q instead of trusted %q", receivedBiz, trustedBiz)
	}
}

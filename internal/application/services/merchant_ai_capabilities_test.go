package services_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

type mockCatalogRepo struct {
	catalogs []ports.CatalogRecord
	items    []ports.CatalogItemRecord
	offers   []ports.OfferRecord
}

func (m *mockCatalogRepo) ListCatalogs(_ context.Context, businessID, status string, limit int, cursor string) (ports.CatalogPage, error) {
	return ports.CatalogPage{Items: m.catalogs}, nil
}

func (m *mockCatalogRepo) GetCatalog(_ context.Context, businessID, catalogID string) (ports.CatalogRecord, error) {
	for _, c := range m.catalogs {
		if c.ID == catalogID {
			return c, nil
		}
	}
	return ports.CatalogRecord{}, nil
}

func (m *mockCatalogRepo) CreateCatalog(_ context.Context, draft ports.CatalogDraft) (ports.CatalogRecord, error) {
	rec := ports.CatalogRecord{ID: draft.ID, BusinessID: draft.BusinessID, Name: draft.Name, Status: draft.Status}
	m.catalogs = append(m.catalogs, rec)
	return rec, nil
}

func (m *mockCatalogRepo) UpdateCatalog(_ context.Context, patch ports.CatalogPatch) (ports.CatalogRecord, error) {
	return ports.CatalogRecord{}, nil
}

func (m *mockCatalogRepo) ListCatalogItems(_ context.Context, businessID, catalogID, search, status string, limit int, cursor string) (ports.CatalogItemPage, error) {
	return ports.CatalogItemPage{Items: m.items}, nil
}

func (m *mockCatalogRepo) GetCatalogItem(_ context.Context, businessID, catalogID, itemID string) (ports.CatalogItemRecord, error) {
	return ports.CatalogItemRecord{}, nil
}

func (m *mockCatalogRepo) CreateCatalogItem(_ context.Context, draft ports.CatalogItemDraft) (ports.CatalogItemRecord, error) {
	return ports.CatalogItemRecord{}, nil
}

func (m *mockCatalogRepo) UpdateCatalogItem(_ context.Context, patch ports.CatalogItemPatch) (ports.CatalogItemRecord, error) {
	return ports.CatalogItemRecord{}, nil
}

func (m *mockCatalogRepo) ListOffers(_ context.Context, businessID, itemID, status string, limit int, cursor string) (ports.OfferPage, error) {
	return ports.OfferPage{Items: m.offers}, nil
}

func (m *mockCatalogRepo) CreateOffer(_ context.Context, draft ports.OfferDraft) (ports.OfferRecord, error) {
	return ports.OfferRecord{}, nil
}

func (m *mockCatalogRepo) UpdateOffer(_ context.Context, patch ports.OfferPatch) (ports.OfferRecord, error) {
	return ports.OfferRecord{}, nil
}

func (m *mockCatalogRepo) ListVariants(_ context.Context, businessID, itemID, status string, limit int, cursor string) (ports.VariantPage, error) {
	return ports.VariantPage{}, nil
}

func (m *mockCatalogRepo) CreateVariant(_ context.Context, draft ports.VariantDraft) (ports.VariantRecord, error) {
	return ports.VariantRecord{}, nil
}

func (m *mockCatalogRepo) UpdateVariant(_ context.Context, patch ports.VariantPatch) (ports.VariantRecord, error) {
	return ports.VariantRecord{}, nil
}

func (m *mockCatalogRepo) ListAttributeSchemas(_ context.Context, businessID, name string, version *int, limit int, cursor string) (ports.AttributeSchemaPage, error) {
	return ports.AttributeSchemaPage{}, nil
}

func (m *mockCatalogRepo) GetAttributeSchema(_ context.Context, businessID, schemaID string) (ports.AttributeSchemaRecord, error) {
	return ports.AttributeSchemaRecord{}, nil
}

func (m *mockCatalogRepo) NextAttributeSchemaVersion(_ context.Context, businessID, name string) (int, error) {
	return 1, nil
}

func (m *mockCatalogRepo) CreateAttributeSchemaVersion(_ context.Context, draft ports.AttributeSchemaDraft) (ports.AttributeSchemaRecord, error) {
	return ports.AttributeSchemaRecord{}, nil
}

type fakeAuthorHandler struct{}

func (f fakeAuthorHandler) Handle(_ context.Context, cmd commands.AuthorCatalogItemCommand) (commands.AuthorCatalogItemResult, error) {
	return commands.AuthorCatalogItemResult{
		Item: commands.CatalogItemView{
			ID:     "item-123",
			Name:   cmd.Name,
			Status: "active",
		},
		Variants: make([]commands.VariantView, len(cmd.Variants)),
		Offers:   make([]commands.OfferView, len(cmd.Offers)),
	}, nil
}

func TestMerchantAICapabilities_RegistrationAndList(t *testing.T) {
	repo := &mockCatalogRepo{
		catalogs: []ports.CatalogRecord{
			{ID: "cat-1", Name: "الكتالوج الرئيسي", Status: "active"},
		},
	}
	registry := services.NewMerchantAICapabilityRegistry(repo, fakeAuthorHandler{}, nil, nil, nil, nil)

	defs := registry.Definitions()
	if len(defs) < 8 {
		t.Fatalf("expected at least 8 tools, got %d", len(defs))
	}

	execCtx := ports.AICapabilityExecutionContext{BusinessID: "biz-1"}
	res, err := registry.Execute(context.Background(), execCtx, "list_catalogs", []byte(`{}`))
	if err != nil {
		t.Fatalf("Execute list_catalogs error = %v", err)
	}

	dataBytes, _ := json.Marshal(res.Data)
	var data map[string]any
	_ = json.Unmarshal(dataBytes, &data)
	if data["total"].(float64) != 1 {
		t.Errorf("expected 1 catalog, got %v", data["total"])
	}
}

func TestMerchantAICapabilities_AuthorItem(t *testing.T) {
	repo := &mockCatalogRepo{
		catalogs: []ports.CatalogRecord{
			{ID: "cat-1", Name: "الكتالوج الرئيسي", Status: "active"},
		},
	}
	registry := services.NewMerchantAICapabilityRegistry(repo, fakeAuthorHandler{}, nil, nil, nil, nil)

	params := `{
		"name": "عطر كوكو شانيل",
		"pricing_mode": "fixed",
		"availability_mode": "stock",
		"fulfillment_mode": "pickup",
		"offers": [
			{"amount": 15000, "currency": "YER"}
		]
	}`

	execCtx := ports.AICapabilityExecutionContext{BusinessID: "biz-1"}
	res, err := registry.Execute(context.Background(), execCtx, "author_catalog_item", []byte(params))
	if err != nil {
		t.Fatalf("Execute author_catalog_item error = %v", err)
	}

	dataBytes, _ := json.Marshal(res.Data)
	var data map[string]any
	_ = json.Unmarshal(dataBytes, &data)
	if data["success"] != true {
		t.Errorf("expected success true, got %v", data["success"])
	}
	if data["name"] != "عطر كوكو شانيل" {
		t.Errorf("expected name 'عطر كوكو شانيل', got %v", data["name"])
	}
}

type fakeVariantHandler struct{}

func (f fakeVariantHandler) Handle(_ context.Context, cmd commands.CreateVariantCommand) (commands.VariantResult, error) {
	return commands.VariantResult{
		Variant: commands.VariantView{
			ID:     "var-999",
			Name:   cmd.Name,
			Status: "active",
		},
	}, nil
}

type fakeUpdateOfferHandler struct {
	updatedOffers []commands.UpdateOfferCommand
}

func (f *fakeUpdateOfferHandler) Handle(_ context.Context, cmd commands.UpdateOfferCommand) (commands.OfferResult, error) {
	f.updatedOffers = append(f.updatedOffers, cmd)
	return commands.OfferResult{
		Offer: commands.OfferView{
			ID:     cmd.OfferID,
			Status: "active",
		},
	}, nil
}

func TestMerchantAICapabilities_GetItemDetailsAutoResolvesCatalog(t *testing.T) {
	repo := &mockCatalogRepo{
		catalogs: []ports.CatalogRecord{
			{ID: "cat-1", Name: "الكتالوج الرئيسي", Status: "active"},
		},
		items: []ports.CatalogItemRecord{
			{ID: "item-100", CatalogID: "cat-1", Name: "عسل دوعني", ItemType: "food", Status: "active"},
		},
	}
	registry := services.NewMerchantAICapabilityRegistry(repo, fakeAuthorHandler{}, nil, nil, nil, nil)

	execCtx := ports.AICapabilityExecutionContext{BusinessID: "biz-1"}
	// Test calling get_item_details without catalog_id
	res, err := registry.Execute(context.Background(), execCtx, "get_item_details", []byte(`{"item_id": "item-100"}`))
	if err != nil {
		t.Fatalf("get_item_details should not fail when catalog_id is omitted: %v", err)
	}
	dataBytes, _ := json.Marshal(res.Data)
	var data map[string]any
	_ = json.Unmarshal(dataBytes, &data)
	if data["item"] == nil {
		t.Fatalf("expected item data in result")
	}
}

func TestMerchantAICapabilities_CreateVariantWithPrice(t *testing.T) {
	repo := &mockCatalogRepo{
		catalogs: []ports.CatalogRecord{
			{ID: "cat-1", Name: "الكتالوج الرئيسي", Status: "active"},
		},
	}
	registry := services.NewMerchantAICapabilityRegistry(repo, fakeAuthorHandler{}, nil, nil, nil, fakeVariantHandler{})

	execCtx := ports.AICapabilityExecutionContext{BusinessID: "biz-1"}
	params := `{
		"item_id": "item-100",
		"name": "مقاس XXL",
		"price_amount": 4500,
		"currency": "YER"
	}`
	res, err := registry.Execute(context.Background(), execCtx, "create_variant", []byte(params))
	if err != nil {
		t.Fatalf("create_variant error = %v", err)
	}
	dataBytes, _ := json.Marshal(res.Data)
	var data map[string]any
	_ = json.Unmarshal(dataBytes, &data)
	if data["success"] != true {
		t.Errorf("expected success true, got %v", data["success"])
	}
	if data["variant_id"] != "var-999" {
		t.Errorf("expected variant_id var-999, got %v", data["variant_id"])
	}
}

func TestMerchantAICapabilities_UpdateCatalogItemTargetedVariant(t *testing.T) {
	varId1 := "var-1"
	varId2 := "var-2"
	amt1 := "6000000"
	amt2 := "1500000"
	repo := &mockCatalogRepo{
		offers: []ports.OfferRecord{
			{ID: "off-1", VariantID: &varId1, Name: "دبة كبيرة", Amount: &amt1, Status: "active"},
			{ID: "off-2", VariantID: &varId2, Name: "علبة صغيرة", Amount: &amt2, Status: "active"},
		},
	}
	offerHandler := &fakeUpdateOfferHandler{}
	registry := services.NewMerchantAICapabilityRegistry(repo, fakeAuthorHandler{}, nil, nil, offerHandler, nil)

	execCtx := ports.AICapabilityExecutionContext{BusinessID: "biz-1"}
	// Update specifically offer off-2 by offer_id
	params := `{
		"item_id": "item-100",
		"offer_id": "off-2",
		"price_amount": 12000
	}`
	res, err := registry.Execute(context.Background(), execCtx, "update_catalog_item", []byte(params))
	if err != nil {
		t.Fatalf("update_catalog_item error = %v", err)
	}
	dataBytes, _ := json.Marshal(res.Data)
	var data map[string]any
	_ = json.Unmarshal(dataBytes, &data)
	if data["success"] != true {
		t.Errorf("expected success true, got %v", data["success"])
	}
	if len(offerHandler.updatedOffers) != 1 {
		t.Fatalf("expected exactly 1 offer updated, got %d", len(offerHandler.updatedOffers))
	}
	if string(offerHandler.updatedOffers[0].OfferID) != "off-2" {
		t.Errorf("expected off-2 to be updated, got %s", offerHandler.updatedOffers[0].OfferID)
	}
	if *offerHandler.updatedOffers[0].AmountMinor != 1200000 {
		t.Errorf("expected amount minor 1200000, got %d", *offerHandler.updatedOffers[0].AmountMinor)
	}
}

func TestMerchantAICapabilities_ParseFileAutoCreatesDefaultCatalog(t *testing.T) {
	// Empty repo with NO catalogs
	repo := &mockCatalogRepo{}
	registry := services.NewMerchantAICapabilityRegistry(repo, fakeAuthorHandler{}, nil, nil, nil, nil)

	execCtx := ports.AICapabilityExecutionContext{BusinessID: "biz-1"}
	csvContent := "اسم الصنف,السعر\nعسل سدر,5000\nسمن بلدي,7000"
	params, _ := json.Marshal(map[string]any{
		"raw_content": csvContent,
	})

	res, err := registry.Execute(context.Background(), execCtx, "parse_and_import_file", params)
	if err != nil {
		t.Fatalf("parse_and_import_file should auto-create default catalog: %v", err)
	}
	dataBytes, _ := json.Marshal(res.Data)
	var data map[string]any
	_ = json.Unmarshal(dataBytes, &data)
	if data["success"] != true {
		t.Errorf("expected success true, got %v", data["success"])
	}
	if data["imported_count"].(float64) != 2 {
		t.Errorf("expected 2 items imported, got %v", data["imported_count"])
	}
}

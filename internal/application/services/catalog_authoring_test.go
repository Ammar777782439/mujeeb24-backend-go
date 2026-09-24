package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type mockTransactionManager struct {
	withinFn func(ctx context.Context, fn func(context.Context) error) error
}

func (m mockTransactionManager) Within(ctx context.Context, fn func(context.Context) error) error {
	if m.withinFn != nil {
		return m.withinFn(ctx, fn)
	}
	return fn(ctx)
}

type mockCatalogRepository struct {
	ports.CatalogRepository
	createCatalogItemFn func(ctx context.Context, draft ports.CatalogItemDraft) (ports.CatalogItemRecord, error)
	createVariantFn     func(ctx context.Context, draft ports.VariantDraft) (ports.VariantRecord, error)
	createOfferFn       func(ctx context.Context, draft ports.OfferDraft) (ports.OfferRecord, error)
	createCatalogFn     func(ctx context.Context, draft ports.CatalogDraft) (ports.CatalogRecord, error)
	createSchemaFn      func(ctx context.Context, draft ports.AttributeSchemaDraft) (ports.AttributeSchemaRecord, error)
	getSchemaFn         func(ctx context.Context, businessID, schemaID string) (ports.AttributeSchemaRecord, error)
	nextSchemaVerFn     func(ctx context.Context, businessID, name string) (int, error)
}

func (m mockCatalogRepository) CreateCatalogItem(ctx context.Context, draft ports.CatalogItemDraft) (ports.CatalogItemRecord, error) {
	if m.createCatalogItemFn != nil {
		return m.createCatalogItemFn(ctx, draft)
	}
	return ports.CatalogItemRecord{
		ID:              draft.ID,
		BusinessID:      draft.BusinessID,
		CatalogID:       draft.CatalogID,
		Name:            draft.Name,
		ItemType:        draft.ItemType,
		Status:          "draft",
		ResourceVersion: 1,
		CreatedAt:       draft.CreatedAt,
		UpdatedAt:       draft.UpdatedAt,
	}, nil
}

func (m mockCatalogRepository) CreateVariant(ctx context.Context, draft ports.VariantDraft) (ports.VariantRecord, error) {
	if m.createVariantFn != nil {
		return m.createVariantFn(ctx, draft)
	}
	return ports.VariantRecord{
		ID:              draft.ID,
		BusinessID:      draft.BusinessID,
		CatalogItemID:   draft.CatalogItemID,
		Name:            draft.Name,
		Status:          draft.Status,
		ResourceVersion: 1,
		CreatedAt:       draft.CreatedAt,
		UpdatedAt:       draft.UpdatedAt,
	}, nil
}

func (m mockCatalogRepository) CreateOffer(ctx context.Context, draft ports.OfferDraft) (ports.OfferRecord, error) {
	if m.createOfferFn != nil {
		return m.createOfferFn(ctx, draft)
	}
	var amountStr *string
	if draft.AmountMinor != nil {
		s := "15000"
		amountStr = &s
	}
	return ports.OfferRecord{
		ID:                 draft.ID,
		BusinessID:         draft.BusinessID,
		CatalogItemID:      draft.CatalogItemID,
		VariantID:          draft.VariantID,
		Name:               draft.Name,
		PricingMode:        draft.PricingMode,
		Amount:             amountStr,
		Currency:           draft.Currency,
		AvailabilityStatus: draft.AvailabilityStatus,
		Status:             draft.Status,
		ResourceVersion:    1,
		CreatedAt:          draft.CreatedAt,
		UpdatedAt:          draft.UpdatedAt,
	}, nil
}

func (m mockCatalogRepository) CreateCatalog(ctx context.Context, draft ports.CatalogDraft) (ports.CatalogRecord, error) {
	if m.createCatalogFn != nil {
		return m.createCatalogFn(ctx, draft)
	}
	return ports.CatalogRecord{
		ID:              draft.ID,
		BusinessID:      draft.BusinessID,
		Name:            draft.Name,
		Description:     draft.Description,
		Status:          draft.Status,
		ResourceVersion: 1,
		CreatedAt:       draft.CreatedAt,
		UpdatedAt:       draft.UpdatedAt,
	}, nil
}

func (m mockCatalogRepository) CreateAttributeSchemaVersion(ctx context.Context, draft ports.AttributeSchemaDraft) (ports.AttributeSchemaRecord, error) {
	if m.createSchemaFn != nil {
		return m.createSchemaFn(ctx, draft)
	}
	return ports.AttributeSchemaRecord{
		ID:         draft.ID,
		BusinessID: draft.BusinessID,
		Name:       draft.Name,
		Version:    draft.Version,
	}, nil
}

func (m mockCatalogRepository) GetAttributeSchema(ctx context.Context, businessID, schemaID string) (ports.AttributeSchemaRecord, error) {
	if m.getSchemaFn != nil {
		return m.getSchemaFn(ctx, businessID, schemaID)
	}
	return ports.AttributeSchemaRecord{ID: schemaID, BusinessID: businessID, Name: "Test Schema", Version: 1}, nil
}

func (m mockCatalogRepository) NextAttributeSchemaVersion(ctx context.Context, businessID, name string) (int, error) {
	if m.nextSchemaVerFn != nil {
		return m.nextSchemaVerFn(ctx, businessID, name)
	}
	return 1, nil
}

// 1. Test AuthorCatalogItemCommandService Atomic Multi-step Creation
func TestAuthorCatalogItemCommandServiceAtomicSuccess(t *testing.T) {
	const testBiz = "biz-author-1"
	createdItem := false
	createdVariants := 0
	createdOffers := 0

	repo := mockCatalogRepository{
		createCatalogItemFn: func(ctx context.Context, draft ports.CatalogItemDraft) (ports.CatalogItemRecord, error) {
			if draft.BusinessID != testBiz {
				t.Fatalf("expected business %s, got %s", testBiz, draft.BusinessID)
			}
			if draft.Name != "Men's White Shirt" {
				t.Fatalf("unexpected name: %s", draft.Name)
			}
			createdItem = true
			return ports.CatalogItemRecord{
				ID:              draft.ID,
				BusinessID:      draft.BusinessID,
				CatalogID:       draft.CatalogID,
				Name:            draft.Name,
				ItemType:        draft.ItemType,
				Status:          "draft",
				ResourceVersion: 1,
			}, nil
		},
		createVariantFn: func(ctx context.Context, draft ports.VariantDraft) (ports.VariantRecord, error) {
			if draft.BusinessID != testBiz {
				t.Fatalf("expected business %s, got %s", testBiz, draft.BusinessID)
			}
			createdVariants++
			return ports.VariantRecord{
				ID:              draft.ID,
				BusinessID:      draft.BusinessID,
				CatalogItemID:   draft.CatalogItemID,
				Name:            draft.Name,
				Status:          draft.Status,
				ResourceVersion: 1,
			}, nil
		},
		createOfferFn: func(ctx context.Context, draft ports.OfferDraft) (ports.OfferRecord, error) {
			if draft.BusinessID != testBiz {
				t.Fatalf("expected business %s, got %s", testBiz, draft.BusinessID)
			}
			if draft.AmountMinor == nil || *draft.AmountMinor != 1500000 {
				t.Fatalf("unexpected amount minor: %v", draft.AmountMinor)
			}
			if draft.Currency == nil || *draft.Currency != "YER" {
				t.Fatalf("unexpected currency: %v", draft.Currency)
			}
			createdOffers++
			amtStr := "15000"
			return ports.OfferRecord{
				ID:                 draft.ID,
				BusinessID:         draft.BusinessID,
				CatalogItemID:      draft.CatalogItemID,
				Name:               draft.Name,
				PricingMode:        draft.PricingMode,
				Amount:             &amtStr,
				Currency:           draft.Currency,
				AvailabilityStatus: draft.AvailabilityStatus,
				Status:             draft.Status,
				ResourceVersion:    1,
			}, nil
		},
	}

	cmdServices := CatalogCommandServices{
		Repository:   repo,
		Transactions: mockTransactionManager{},
		NewID:        func() string { return "gen-id" },
		Now:          func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC) },
	}
	authorService := AuthorCatalogItemCommandService{CatalogCommandServices: cmdServices}

	amountMinor := int64(1500000)
	currency := "YER"

	res, err := authorService.Handle(context.Background(), commands.AuthorCatalogItemCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{
				BusinessID:  testBiz,
				PrincipalID: "user-1",
				Role:        "owner",
			},
		},
		CatalogID: "cat-1",
		Name:      "Men's White Shirt",
		ItemType:  "product",
		Variants: []commands.AuthorVariantInput{
			{Name: "S", Attributes: map[string]any{"size": "S"}},
			{Name: "M", Attributes: map[string]any{"size": "M"}},
			{Name: "L", Attributes: map[string]any{"size": "L"}},
		},
		Offers: []commands.AuthorOfferInput{
			{
				Name:        "Standard Offer",
				PricingMode: "fixed",
				AmountMinor: &amountMinor,
				Currency:    &currency,
			},
		},
	})

	if err != nil {
		t.Fatalf("author item failed: %v", err)
	}

	if !createdItem || createdVariants != 3 || createdOffers != 1 {
		t.Fatalf("unexpected counts: createdItem=%v, variants=%d, offers=%d", createdItem, createdVariants, createdOffers)
	}
	if res.Item.Name != "Men's White Shirt" {
		t.Fatalf("unexpected item name: %s", res.Item.Name)
	}
	if len(res.Variants) != 3 {
		t.Fatalf("expected 3 variants, got %d", len(res.Variants))
	}
	if len(res.Offers) != 1 {
		t.Fatalf("expected 1 offer, got %d", len(res.Offers))
	}
}

// 2. Test Transaction Rollback on Error (Atomicity)
func TestAuthorCatalogItemCommandServiceTransactionRollback(t *testing.T) {
	txExecuted := false
	repo := mockCatalogRepository{
		createCatalogItemFn: func(ctx context.Context, draft ports.CatalogItemDraft) (ports.CatalogItemRecord, error) {
			return ports.CatalogItemRecord{ID: "item-1"}, nil
		},
		createVariantFn: func(ctx context.Context, draft ports.VariantDraft) (ports.VariantRecord, error) {
			return ports.VariantRecord{}, errors.New("database disk full")
		},
	}

	cmdServices := CatalogCommandServices{
		Repository: repo,
		Transactions: mockTransactionManager{
			withinFn: func(ctx context.Context, fn func(context.Context) error) error {
				txExecuted = true
				return fn(ctx)
			},
		},
	}
	authorService := AuthorCatalogItemCommandService{CatalogCommandServices: cmdServices}

	_, err := authorService.Handle(context.Background(), commands.AuthorCatalogItemCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{
				BusinessID: "biz-1",
				Role:       "manager",
			},
		},
		CatalogID: "cat-1",
		Name:      "Shirt",
		Variants: []commands.AuthorVariantInput{
			{Name: "S"},
		},
	})

	if err == nil {
		t.Fatal("expected transaction error, got nil")
	}
	if !txExecuted {
		t.Fatal("expected transaction manager Within to be executed")
	}
}

// 3. Test Role Authorization Rejection for Viewer / Analyst
func TestAuthorCatalogItemCommandServiceRoleAuthorization(t *testing.T) {
	cmdServices := CatalogCommandServices{
		Repository:   mockCatalogRepository{},
		Transactions: mockTransactionManager{},
	}
	authorService := AuthorCatalogItemCommandService{CatalogCommandServices: cmdServices}

	for _, unauthRole := range []string{"viewer", "analyst"} {
		_, err := authorService.Handle(context.Background(), commands.AuthorCatalogItemCommand{
			Meta: commands.CommandMeta{
				Actor: commands.ActorContext{
					BusinessID: "biz-1",
					Role:       unauthRole,
				},
			},
			CatalogID: "cat-1",
			Name:      "Shirt",
		})
		if err == nil {
			t.Fatalf("expected forbidden error for role %s, got nil", unauthRole)
		}
		var appErr *appErrors.Error
		if !errors.As(err, &appErr) || appErr.Code != appErrors.CodeForbidden {
			t.Fatalf("expected CodeForbidden, got %v", err)
		}
	}
}

// 4. Test Invalid Pricing Rejection
func TestAuthorCatalogItemCommandServiceInvalidPricing(t *testing.T) {
	cmdServices := CatalogCommandServices{
		Repository:   mockCatalogRepository{},
		Transactions: mockTransactionManager{},
	}
	authorService := AuthorCatalogItemCommandService{CatalogCommandServices: cmdServices}

	negativeMinor := int64(-500)
	validMinor := int64(1000)
	emptyCurrency := ""

	// Negative amount
	_, err := authorService.Handle(context.Background(), commands.AuthorCatalogItemCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{BusinessID: "biz-1", Role: "owner"},
		},
		CatalogID: "cat-1",
		Name:      "Shirt",
		Offers: []commands.AuthorOfferInput{
			{
				Name:        "Promo",
				PricingMode: "fixed",
				AmountMinor: &negativeMinor,
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "negative") {
		t.Fatalf("expected negative amount error, got %v", err)
	}

	// Missing currency when amount is specified
	_, err = authorService.Handle(context.Background(), commands.AuthorCatalogItemCommand{
		Meta: commands.CommandMeta{
			Actor: commands.ActorContext{BusinessID: "biz-1", Role: "owner"},
		},
		CatalogID: "cat-1",
		Name:      "Shirt",
		Offers: []commands.AuthorOfferInput{
			{
				Name:        "Promo",
				PricingMode: "fixed",
				AmountMinor: &validMinor,
				Currency:    &emptyCurrency,
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "currency") {
		t.Fatalf("expected missing currency error, got %v", err)
	}
}

// 5. Test CatalogAuthoringCapability Tenant Isolation (Overriding AI parameters)
func TestCatalogAuthoringCapabilityTenantDefense(t *testing.T) {
	const trustedBiz = "trusted-biz-100"
	receivedBiz := ""

	repo := mockCatalogRepository{
		createCatalogFn: func(ctx context.Context, draft ports.CatalogDraft) (ports.CatalogRecord, error) {
			receivedBiz = draft.BusinessID
			return ports.CatalogRecord{ID: "cat-1", BusinessID: draft.BusinessID, Name: draft.Name}, nil
		},
	}

	cmdServices := CatalogCommandServices{
		Repository:   repo,
		Transactions: mockTransactionManager{},
		NewID:        func() string { return "id-1" },
		Now:          func() time.Time { return time.Now().UTC() },
	}

	cap := NewCatalogAuthoringCapability(
		AuthorCatalogItemCommandService{CatalogCommandServices: cmdServices},
		CreateCatalogCommandService{CatalogCommandServices: cmdServices},
		CreateCatalogItemCommandService{CatalogCommandServices: cmdServices},
		CreateOfferCommandService{CatalogCommandServices: cmdServices},
		CreateVariantCommandService{CatalogCommandServices: cmdServices},
		CreateAttributeSchemaVersionCommandService{CatalogCommandServices: cmdServices},
	)

	execCtx := ports.AICapabilityExecutionContext{
		BusinessID:     trustedBiz,
		ConversationID: "conv-1",
		Role:           "owner",
	}

	// AI maliciously attempts to inject business_id into rawParams
	raw := []byte(`{"operation":"create_catalog","name":"Attacker Catalog","business_id":"attacker-tenant-999"}`)
	_, err := cap.Execute(context.Background(), execCtx, raw)
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}
	if receivedBiz != trustedBiz {
		t.Fatalf("SECURITY VIOLATION: repository received business %q instead of trusted %q", receivedBiz, trustedBiz)
	}
}

// 6. Test CatalogAuthoringCapability All Granular Operations
func TestCatalogAuthoringCapabilityAllOperations(t *testing.T) {
	const testBiz = "biz-full-test"
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	repo := mockCatalogRepository{
		createCatalogItemFn: func(ctx context.Context, draft ports.CatalogItemDraft) (ports.CatalogItemRecord, error) {
			return ports.CatalogItemRecord{
				ID:         "item-10",
				BusinessID: draft.BusinessID,
				CatalogID:  draft.CatalogID,
				Name:       draft.Name,
				ItemType:   draft.ItemType,
				Status:     "draft",
			}, nil
		},
		createVariantFn: func(ctx context.Context, draft ports.VariantDraft) (ports.VariantRecord, error) {
			return ports.VariantRecord{
				ID:            "var-10",
				BusinessID:    draft.BusinessID,
				CatalogItemID: draft.CatalogItemID,
				Name:          draft.Name,
				Status:        "active",
			}, nil
		},
		createOfferFn: func(ctx context.Context, draft ports.OfferDraft) (ports.OfferRecord, error) {
			amt := "15000"
			return ports.OfferRecord{
				ID:                 "offer-10",
				BusinessID:         draft.BusinessID,
				CatalogItemID:      draft.CatalogItemID,
				Name:               draft.Name,
				PricingMode:        draft.PricingMode,
				Amount:             &amt,
				Currency:           draft.Currency,
				AvailabilityStatus: draft.AvailabilityStatus,
				Status:             draft.Status,
			}, nil
		},
		createCatalogFn: func(ctx context.Context, draft ports.CatalogDraft) (ports.CatalogRecord, error) {
			return ports.CatalogRecord{
				ID:         "cat-10",
				BusinessID: draft.BusinessID,
				Name:       draft.Name,
				Status:     "draft",
			}, nil
		},
		createSchemaFn: func(ctx context.Context, draft ports.AttributeSchemaDraft) (ports.AttributeSchemaRecord, error) {
			return ports.AttributeSchemaRecord{
				ID:         "schema-10",
				BusinessID: draft.BusinessID,
				Name:       draft.Name,
				Version:    1,
			}, nil
		},
	}

	cmdServices := CatalogCommandServices{
		Repository:   repo,
		Transactions: mockTransactionManager{},
		NewID:        func() string { return "gen-id" },
		Now:          func() time.Time { return now },
	}

	cap := NewCatalogAuthoringCapability(
		AuthorCatalogItemCommandService{CatalogCommandServices: cmdServices},
		CreateCatalogCommandService{CatalogCommandServices: cmdServices},
		CreateCatalogItemCommandService{CatalogCommandServices: cmdServices},
		CreateOfferCommandService{CatalogCommandServices: cmdServices},
		CreateVariantCommandService{CatalogCommandServices: cmdServices},
		CreateAttributeSchemaVersionCommandService{CatalogCommandServices: cmdServices},
	)
	cap.Now = func() time.Time { return now }

	execCtx := ports.AICapabilityExecutionContext{
		BusinessID:     testBiz,
		ConversationID: "conv-1",
		Role:           "owner",
	}

	// 1. author_catalog_item composite
	authorRaw := []byte(`{
		"operation": "author_catalog_item",
		"catalog_id": "cat-10",
		"name": "Men's White Shirt",
		"item_type": "product",
		"amount": "15000",
		"currency": "YER",
		"variants": [
			{"name": "S", "attributes": {"size": "S"}},
			{"name": "M", "attributes": {"size": "M"}}
		]
	}`)
	res1, err := cap.Execute(context.Background(), execCtx, authorRaw)
	if err != nil {
		t.Fatalf("author_catalog_item failed: %v", err)
	}
	if len(res1.CatalogEvidence) != 1 || res1.CatalogEvidence[0].Name != "Men's White Shirt" {
		t.Fatalf("unexpected catalog evidence: %#v", res1.CatalogEvidence)
	}
	if len(res1.VariantEvidence) != 2 {
		t.Fatalf("expected 2 variant evidence records, got %d", len(res1.VariantEvidence))
	}
	if len(res1.OfferEvidence) != 1 || res1.OfferEvidence[0].Currency != "YER" {
		t.Fatalf("unexpected offer evidence: %#v", res1.OfferEvidence)
	}

	// 2. create_catalog
	res2, err := cap.Execute(context.Background(), execCtx, []byte(`{"operation":"create_catalog","name":"Spring 2026"}`))
	if err != nil {
		t.Fatalf("create_catalog failed: %v", err)
	}
	data2, _ := json.Marshal(res2.Data)
	if !strings.Contains(string(data2), "Spring 2026") {
		t.Fatalf("unexpected create_catalog result: %s", string(data2))
	}

	// 3. create_catalog_item
	res3, err := cap.Execute(context.Background(), execCtx, []byte(`{"operation":"create_catalog_item","catalog_id":"cat-10","name":"Item One","item_type":"product"}`))
	if err != nil {
		t.Fatalf("create_catalog_item failed: %v", err)
	}
	if len(res3.CatalogEvidence) != 1 || res3.CatalogEvidence[0].Name != "Item One" {
		t.Fatalf("unexpected catalog item evidence: %#v", res3.CatalogEvidence)
	}

	// 4. create_offer
	res4, err := cap.Execute(context.Background(), execCtx, []byte(`{"operation":"create_offer","item_id":"item-10","name":"Base Offer","pricing_mode":"fixed","amount":"15000","currency":"YER"}`))
	if err != nil {
		t.Fatalf("create_offer failed: %v", err)
	}
	if len(res4.OfferEvidence) != 1 || res4.OfferEvidence[0].Amount != "15000" {
		t.Fatalf("unexpected offer evidence: %#v", res4.OfferEvidence)
	}

	// 5. create_variant
	res5, err := cap.Execute(context.Background(), execCtx, []byte(`{"operation":"create_variant","item_id":"item-10","name":"XL","attributes":{"size":"XL"}}`))
	if err != nil {
		t.Fatalf("create_variant failed: %v", err)
	}
	if len(res5.VariantEvidence) != 1 || res5.VariantEvidence[0].Name != "XL" {
		t.Fatalf("unexpected variant evidence: %#v", res5.VariantEvidence)
	}

	// 6. create_attribute_schema
	res6, err := cap.Execute(context.Background(), execCtx, []byte(`{"operation":"create_attribute_schema","name":"Apparel","definitions":[{"key":"size","label":"Size","data_type":"text","required":true}]}`))
	if err != nil {
		t.Fatalf("create_attribute_schema failed: %v", err)
	}
	data6, _ := json.Marshal(res6.Data)
	if !strings.Contains(string(data6), "Apparel") {
		t.Fatalf("unexpected create_attribute_schema result: %s", string(data6))
	}

	// 7. unsupported operation
	_, err = cap.Execute(context.Background(), execCtx, []byte(`{"operation":"drop_database"}`))
	if err == nil {
		t.Fatal("expected error for unsupported operation")
	}
}

// 7. Test Amount Parsing helper
func TestParseAmountToMinor(t *testing.T) {
	// String integers
	m, err := parseAmountToMinor("15000", nil)
	if err != nil || *m != 1500000 {
		t.Fatalf("expected 1500000, got %v, err=%v", m, err)
	}

	// String with commas
	m, err = parseAmountToMinor("15,000.50", nil)
	if err != nil || *m != 1500050 {
		t.Fatalf("expected 1500050, got %v, err=%v", m, err)
	}

	// Float
	m, err = parseAmountToMinor(float64(250.75), nil)
	if err != nil || *m != 25075 {
		t.Fatalf("expected 25075, got %v, err=%v", m, err)
	}

	// Int
	m, err = parseAmountToMinor(100, nil)
	if err != nil || *m != 10000 {
		t.Fatalf("expected 10000, got %v, err=%v", m, err)
	}

	// Already provided amountMinor
	val := int64(99900)
	m, err = parseAmountToMinor(nil, &val)
	if err != nil || *m != 99900 {
		t.Fatalf("expected 99900, got %v, err=%v", m, err)
	}

	// Invalid string
	_, err = parseAmountToMinor("not-a-number", nil)
	if err == nil {
		t.Fatal("expected error for non-numeric string")
	}

	// Too many decimal places
	_, err = parseAmountToMinor(12.3456, nil)
	if err == nil {
		t.Fatal("expected error for more than 2 decimal places")
	}
}


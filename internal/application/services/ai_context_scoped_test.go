package services

import (
	"context"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type scopedCatalogRepo struct {
	contextCatalogRepository
	itemByCatalog map[string]ports.CatalogItemRecord
	offerByItem   map[string]ports.OfferRecord
}

func (r scopedCatalogRepo) GetCatalogItem(_ context.Context, businessID, catalogID, itemID string) (ports.CatalogItemRecord, error) {
	if it, ok := r.itemByCatalog[catalogID+"/"+itemID]; ok {
		return it, nil
	}
	return ports.CatalogItemRecord{}, errNotFoundStub{}
}

func (r scopedCatalogRepo) GetCatalog(_ context.Context, businessID, catalogID string) (ports.CatalogRecord, error) {
	if catalogID == "cat-1" {
		return ports.CatalogRecord{ID: "cat-1", BusinessID: businessID, Name: "c", Status: "active"}, nil
	}
	return ports.CatalogRecord{}, errNotFoundStub{}
}

func (r scopedCatalogRepo) ListOffers(_ context.Context, _ string, itemID, _ string, _ int, _ string) (ports.OfferPage, error) {
	if o, ok := r.offerByItem[itemID]; ok {
		return ports.OfferPage{Items: []ports.OfferRecord{o}}, nil
	}
	return ports.OfferPage{}, nil
}

func TestScopedOfferRetrieval_BasicOnly(t *testing.T) {
	basicItem := ports.CatalogItemRecord{ID: "item-basic", BusinessID: "b1", CatalogID: "cat-1", Name: "Basic", Status: "active", Attributes: []byte(`{}`)}
	basicOffer := ports.OfferRecord{ID: "offer-basic", BusinessID: "b1", CatalogItemID: "item-basic", Name: "Basic", PricingMode: "fixed", Amount: strPtr("100"), Currency: strPtr("YER"), AvailabilityStatus: "available", Status: "active"}
	repo := scopedCatalogRepo{
		contextCatalogRepository: contextCatalogRepository{
			catalogs: ports.CatalogPage{Items: []ports.CatalogRecord{{ID: "cat-1", BusinessID: "b1", Name: "c", Status: "active"}}},
			items:    map[string]ports.CatalogItemPage{"cat-1": {Items: []ports.CatalogItemRecord{basicItem}}},
			offers:   map[string]ports.OfferPage{"item-basic": {Items: []ports.OfferRecord{basicOffer}}},
		},
		itemByCatalog: map[string]ports.CatalogItemRecord{"cat-1/item-basic": basicItem},
		offerByItem:   map[string]ports.OfferRecord{"item-basic": basicOffer},
	}
	// Patch GetCatalog via wrapper: use real record through ListCatalogs path.
	// Our fake GetCatalog returns empty; override by using catalog repo that returns cat-1.
	repo.contextCatalogRepository.catalogs.Items[0].BusinessID = "b1"
	b := AutoReplyContextBuilder{
		Businesses:    contextBusinessRepository{record: ports.BusinessRecord{ID: "b1"}},
		Conversations: contextConversationRepository{record: ports.ConversationRecord{ID: "c1", BusinessID: "b1", CustomerID: "cust-1"}},
		Customers:     contextCustomerRepository{record: ports.CustomerRecord{ID: "cust-1", BusinessID: "b1"}},
		Catalogs:      repo,
		Messages:      contextMessageRepository{},
	}
	focusItem := "item-basic"
	focusCatalog := "cat-1"
	state := &ports.ConversationStateRecord{
		BusinessID: "b1", ConversationID: "c1",
		Focus: &ports.ConversationFocus{Type: "offer", ID: "offer-basic", ItemID: &focusItem, CatalogID: &focusCatalog},
	}
	ctx, err := b.Build(context.Background(), ports.ContextBuildInput{
		BusinessID: "b1", ConversationID: "c1", Text: "وكم مدتها؟", Channel: "whatsapp",
		ConversationState: state,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(ctx.CatalogEvidence) != 1 || ctx.CatalogEvidence[0].Reference != "item-basic" {
		t.Fatalf("scoped catalog evidence must be basic only, got %#v", ctx.CatalogEvidence)
	}
	if len(ctx.OfferEvidence) != 1 || ctx.OfferEvidence[0].Reference != "offer-basic" {
		t.Fatalf("scoped offer evidence must be basic only, got %#v", ctx.OfferEvidence)
	}
	// Pro must not leak
	for _, o := range ctx.OfferEvidence {
		if o.Reference == "offer-pro" {
			t.Fatal("pro evidence leaked into basic focus")
		}
	}
}

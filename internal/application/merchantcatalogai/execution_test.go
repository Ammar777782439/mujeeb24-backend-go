package merchantcatalogai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type executionRepo struct {
	ports.CatalogRepository
	item        ports.CatalogItemRecord
	creates     int
	updates     int
	variants    int
	failVariant bool
}

func (r *executionRepo) GetCatalogItem(context.Context, string, string, string) (ports.CatalogItemRecord, error) {
	return r.item, nil
}
func (r *executionRepo) UpdateCatalogItem(_ context.Context, p ports.CatalogItemPatch) (ports.CatalogItemRecord, error) {
	r.updates++
	r.item.ResourceVersion++
	if p.Attributes != nil {
		r.item.Attributes = p.Attributes
	}
	return r.item, nil
}
func (r *executionRepo) CreateCatalogItem(_ context.Context, p ports.CatalogItemDraft) (ports.CatalogItemRecord, error) {
	r.creates++
	return ports.CatalogItemRecord{ID: p.ID}, nil
}
func (r *executionRepo) CreateVariant(_ context.Context, p ports.VariantDraft) (ports.VariantRecord, error) {
	if r.failVariant {
		return ports.VariantRecord{}, errors.New("variant write failed")
	}
	r.variants++
	return ports.VariantRecord{ID: p.ID}, nil
}
func (r *executionRepo) CreateOffer(_ context.Context, p ports.OfferDraft) (ports.OfferRecord, error) {
	return ports.OfferRecord{ID: p.ID}, nil
}
func (r *executionRepo) ListVariants(context.Context, string, string, string, int, string) (ports.VariantPage, error) {
	return ports.VariantPage{}, nil
}

type executionMemoryStore struct {
	p           StoredProposal
	completions int
}

func (s *executionMemoryStore) Save(_ context.Context, p StoredProposal) error { s.p = p; return nil }
func (s *executionMemoryStore) Lock(_ context.Context, b, p, id string) (StoredProposal, error) {
	if b != s.p.BusinessID || p != s.p.PrincipalID || id != s.p.ID {
		return StoredProposal{}, errors.New("proposal not found")
	}
	return s.p, nil
}
func (s *executionMemoryStore) Complete(_ context.Context, b, id string, r ExecutionResult) error {
	s.p.Result = &r
	s.completions++
	return nil
}

type executionTransaction struct {
	repo  *executionRepo
	store *executionMemoryStore
}

func (tx executionTransaction) Within(ctx context.Context, fn func(context.Context) error) error {
	oldRepo := *tx.repo
	oldStore := *tx.store
	err := fn(ctx)
	if err != nil {
		*tx.repo = oldRepo
		*tx.store = oldStore
	}
	return err
}
func executionFixture() (ExecutionService, *executionRepo, *executionMemoryStore) {
	repo := &executionRepo{item: ports.CatalogItemRecord{ID: "item", BusinessID: "business", CatalogID: "catalog", ResourceVersion: 7, Attributes: []byte(`{"existing":true,"warranty":"1 year"}`)}}
	store := &executionMemoryStore{}
	return ExecutionService{Catalogs: repo, Store: store, Transactions: executionTransaction{repo, store}}, repo, store
}
func executionUpdate() ExecutionInput {
	return ExecutionInput{BusinessID: "business", PrincipalID: "principal", SessionID: "session", SelectedCatalog: ports.CatalogRecord{ID: "catalog"}, Proposal: Proposal{SchemaVersion: ProposalSchemaVersion, Status: StatusResolved, Operation: OperationUpdate, ResponseText: "Review", Update: &UpdateOperation{ItemID: "item", Changes: ItemChanges{Attributes: map[string]any{"warranty": "5 years"}}, NewVariants: []VariantCreate{{Ref: "new", Name: "New option"}}}}}
}
func TestApprovalUpdatesOneItemPreservesAttributesAndReplays(t *testing.T) {
	s, r, store := executionFixture()
	id, e := s.Prepare(context.Background(), executionUpdate())
	if e != nil {
		t.Fatal(e)
	}
	first, e := s.ExecuteApproved(context.Background(), "business", "principal", id)
	if e != nil {
		t.Fatal(e)
	}
	second, e := s.ExecuteApproved(context.Background(), "business", "principal", id)
	if e != nil {
		t.Fatal(e)
	}
	if first.ItemID != "item" || second.ItemID != first.ItemID || r.creates != 0 || r.updates != 1 || r.variants != 1 || store.completions != 1 {
		t.Fatalf("duplicate or incorrect writes: repo=%+v result=%+v", r, second)
	}
	var attrs map[string]any
	if e = json.Unmarshal(r.item.Attributes, &attrs); e != nil {
		t.Fatal(e)
	}
	if attrs["existing"] != true || attrs["warranty"] != "5 years" {
		t.Fatalf("attributes replaced: %s", r.item.Attributes)
	}
}
func TestApprovalRollsBackItemIfVariantFails(t *testing.T) {
	s, r, store := executionFixture()
	r.failVariant = true
	id, e := s.Prepare(context.Background(), executionUpdate())
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.ExecuteApproved(context.Background(), "business", "principal", id)
	if e == nil {
		t.Fatal("expected failure")
	}
	if r.updates != 0 || r.variants != 0 || store.p.Result != nil || r.item.ResourceVersion != 7 {
		t.Fatalf("partial mutation remained: %+v", r)
	}
}
func TestApprovalRejectsChangedSnapshotAndWrongPrincipal(t *testing.T) {
	s, r, _ := executionFixture()
	id, e := s.Prepare(context.Background(), executionUpdate())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ExecuteApproved(context.Background(), "business", "other", id); e == nil {
		t.Fatal("wrong principal accepted")
	}
	r.item.ResourceVersion++
	if _, e = s.ExecuteApproved(context.Background(), "business", "principal", id); e == nil {
		t.Fatal("stale review accepted")
	}
	if r.updates != 0 || r.variants != 0 {
		t.Fatal("writes before rejection")
	}
}
func TestApprovalRejectsCrossCatalogItem(t *testing.T) {
	s, r, _ := executionFixture()
	r.item.CatalogID = "other"
	if _, e := s.Prepare(context.Background(), executionUpdate()); e == nil {
		t.Fatal("cross-catalog item accepted")
	}
}

func TestApprovalCreateReplaysWithoutCreatingAnotherProduct(t *testing.T) {
	s, r, _ := executionFixture()
	amount, currency := "5000.1234", "YER"
	input := ExecutionInput{BusinessID: "business", PrincipalID: "principal", SessionID: "session", SelectedCatalog: ports.CatalogRecord{ID: "catalog"}, Proposal: Proposal{SchemaVersion: ProposalSchemaVersion, Status: StatusResolved, Operation: OperationCreate, ResponseText: "Review", Create: &ItemCreate{Name: "New entity", ItemType: "custom", PricingMode: "fixed", AvailabilityMode: "stock", FulfillmentMode: "delivery", Offers: []OfferCreate{{Name: DefaultOfferName, NameSource: OfferNameSourceSystemDefault, PricingMode: "fixed", Amount: &amount, PriceSource: OfferPriceSourceMerchantStated, Currency: &currency, AvailabilityMode: "stock", AvailabilityStatus: "available", FulfillmentMode: "delivery", Status: "active"}}}}}
	id, err := s.Prepare(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.ExecuteApproved(context.Background(), "business", "principal", id)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.ExecuteApproved(context.Background(), "business", "principal", id)
	if err != nil {
		t.Fatal(err)
	}
	if r.creates != 1 || first.ItemID != second.ItemID || first.OfferIDs[0] != second.OfferIDs[0] {
		t.Fatalf("duplicate product or offer: %+v %+v", first, second)
	}
}

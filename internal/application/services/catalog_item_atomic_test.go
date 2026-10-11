package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type atomicCatalogRepo struct {
	ports.CatalogRepository
	items     int
	offers    int
	failOffer bool
	lastOffer ports.OfferDraft
	lastPatch ports.OfferPatch
}

func (r *atomicCatalogRepo) CreateCatalogItem(_ context.Context, draft ports.CatalogItemDraft) (ports.CatalogItemRecord, error) {
	r.items++
	return ports.CatalogItemRecord{ID: draft.ID, BusinessID: draft.BusinessID, CatalogID: draft.CatalogID, ItemType: draft.ItemType, Name: draft.Name, Status: "draft", PricingMode: draft.PricingMode, AvailabilityMode: draft.AvailabilityMode, FulfillmentMode: draft.FulfillmentMode, ResourceVersion: 1}, nil
}

func (r *atomicCatalogRepo) CreateOffer(_ context.Context, draft ports.OfferDraft) (ports.OfferRecord, error) {
	if r.failOffer {
		return ports.OfferRecord{}, errors.New("offer failed")
	}
	r.offers++
	r.lastOffer = draft
	return ports.OfferRecord{ID: draft.ID}, nil
}

func (r *atomicCatalogRepo) GetOffer(_ context.Context, businessID, offerID string) (ports.OfferRecord, error) {
	amount := "10000.00"
	currency := "YER"
	return ports.OfferRecord{
		ID: offerID, BusinessID: businessID, CatalogItemID: "item-id",
		Name: "السعر الأساسي", PricingMode: "fixed", Amount: &amount, Currency: &currency,
		AvailabilityMode: "stock", AvailabilityStatus: "available",
		FulfillmentMode: "delivery", Status: "active", ResourceVersion: 1,
	}, nil
}

func (r *atomicCatalogRepo) UpdateOffer(_ context.Context, patch ports.OfferPatch) (ports.OfferRecord, error) {
	r.lastPatch = patch
	return ports.OfferRecord{ID: patch.ID, BusinessID: patch.BusinessID, Status: "active", ResourceVersion: patch.ExpectedVersion + 1}, nil
}

type atomicCatalogTx struct{ repo *atomicCatalogRepo }

func (t atomicCatalogTx) Within(ctx context.Context, fn func(context.Context) error) error {
	before := *t.repo
	err := fn(ctx)
	if err != nil {
		*t.repo = before
	}
	return err
}

func atomicCatalogService(repo *atomicCatalogRepo) CreateCatalogItemCommandService {
	ids := []string{"item-id", "offer-id"}
	index := 0
	base := CatalogCommandServices{
		Repository:   repo,
		Transactions: atomicCatalogTx{repo: repo},
		Now:          func() time.Time { return time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC) },
		NewID: func() string {
			id := ids[index]
			index++
			return id
		},
	}
	return CreateCatalogItemCommandService{CatalogCommandServices: base}
}

func atomicCreateCommand() commands.CreateCatalogItemCommand {
	amount := int64(1000000)
	currency := "YER"
	return commands.CreateCatalogItemCommand{
		Meta:      commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business"}},
		CatalogID: "catalog", ItemType: "product", Name: "Phone",
		PricingMode: "fixed", AvailabilityMode: "stock", FulfillmentMode: "delivery",
		InitialOffer: &commands.InitialOffer{
			Name: "السعر الأساسي", PricingMode: "fixed", AmountMinor: &amount, Currency: &currency,
			AvailabilityMode: "stock", AvailabilityStatus: "available", FulfillmentMode: "delivery", Status: "active",
		},
	}
}

func TestCreateCatalogItemAndInitialOfferCommitTogether(t *testing.T) {
	repo := &atomicCatalogRepo{}
	service := atomicCatalogService(repo)
	if _, err := service.Handle(context.Background(), atomicCreateCommand()); err != nil {
		t.Fatal(err)
	}
	if repo.items != 1 || repo.offers != 1 {
		t.Fatalf("expected one atomic item+offer write, got items=%d offers=%d", repo.items, repo.offers)
	}
	if repo.lastOffer.PriceSource == nil || *repo.lastOffer.PriceSource != "merchant" || repo.lastOffer.PriceVerificationStatus != "verified" || repo.lastOffer.PriceCheckedAt == nil || repo.lastOffer.AvailabilitySource == nil || *repo.lastOffer.AvailabilitySource != "merchant" || repo.lastOffer.AvailabilityCheckedAt == nil {
		t.Fatalf("merchant evidence metadata missing: %#v", repo.lastOffer)
	}
}

func TestCreateCatalogItemRollsBackWhenInitialOfferFails(t *testing.T) {
	repo := &atomicCatalogRepo{failOffer: true}
	service := atomicCatalogService(repo)
	if _, err := service.Handle(context.Background(), atomicCreateCommand()); err == nil {
		t.Fatal("expected initial offer failure")
	}
	if repo.items != 0 || repo.offers != 0 {
		t.Fatalf("partial create remained after rollback: items=%d offers=%d", repo.items, repo.offers)
	}
}

func TestUpdateOfferRefreshesOnlyChangedMerchantEvidence(t *testing.T) {
	repo := &atomicCatalogRepo{}
	base := CatalogCommandServices{
		Repository:   repo,
		Transactions: atomicCatalogTx{repo: repo},
		Now:          func() time.Time { return time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC) },
	}
	service := UpdateOfferCommandService{CatalogCommandServices: base}
	version := commands.ResourceVersion("1")
	amount := int64(250000)
	status := "available"
	_, err := service.Handle(context.Background(), commands.UpdateOfferCommand{
		Meta:    commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business"}, ExpectedVersion: &version},
		OfferID: "offer", AmountMinor: &amount, AvailabilityStatus: &status,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.lastPatch.PriceSource == nil || *repo.lastPatch.PriceSource != "merchant" ||
		repo.lastPatch.PriceVerificationStatus == nil || *repo.lastPatch.PriceVerificationStatus != "verified" ||
		repo.lastPatch.PriceCheckedAt == nil ||
		repo.lastPatch.AvailabilitySource == nil || *repo.lastPatch.AvailabilitySource != "merchant" ||
		repo.lastPatch.AvailabilityCheckedAt == nil {
		t.Fatalf("merchant update evidence metadata missing: %#v", repo.lastPatch)
	}
}

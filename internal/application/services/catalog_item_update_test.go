package services

import (
	"context"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type catalogItemUpdateRepo struct {
	ports.CatalogRepository
	last ports.CatalogItemPatch
}

func (r *catalogItemUpdateRepo) UpdateCatalogItem(_ context.Context, patch ports.CatalogItemPatch) (ports.CatalogItemRecord, error) {
	r.last = patch
	return ports.CatalogItemRecord{ID: patch.ID, BusinessID: patch.BusinessID, CatalogID: "catalog", Name: "Item", Status: "active", PricingMode: "fixed", AvailabilityMode: "stock", FulfillmentMode: "delivery", ResourceVersion: patch.ExpectedVersion + 1}, nil
}

type catalogItemUpdateTx struct{}
func (catalogItemUpdateTx) Within(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func TestUpdateCatalogItemPreservesDescriptionPresence(t *testing.T) {
	repo := &catalogItemUpdateRepo{}
	service := UpdateCatalogItemCommandService{CatalogCommandServices: CatalogCommandServices{
		Repository: repo,
		Transactions: catalogItemUpdateTx{},
		Now: func() time.Time { return time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC) },
	}}
	version := commands.ResourceVersion("3")
	value := "Updated short description"
	_, err := service.Handle(context.Background(), commands.UpdateCatalogItemCommand{
		Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business"}, ExpectedVersion: &version},
		CatalogID: "catalog", CatalogItemID: "item",
		ShortDescription: &value, ShortDescriptionPresent: true,
		LongDescription: nil, LongDescriptionPresent: true,
	})
	if err != nil { t.Fatal(err) }
	if !repo.last.ShortDescriptionPresent || repo.last.ShortDescription == nil || *repo.last.ShortDescription != value {
		t.Fatalf("short description update presence lost: %#v", repo.last)
	}
	if !repo.last.LongDescriptionPresent || repo.last.LongDescription != nil {
		t.Fatalf("explicit long description null was not preserved: %#v", repo.last)
	}
}

func TestUpdateCatalogItemOmittedDescriptionsRemainAbsent(t *testing.T) {
	repo := &catalogItemUpdateRepo{}
	service := UpdateCatalogItemCommandService{CatalogCommandServices: CatalogCommandServices{Repository: repo, Transactions: catalogItemUpdateTx{}}}
	version := commands.ResourceVersion("1")
	_, err := service.Handle(context.Background(), commands.UpdateCatalogItemCommand{
		Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business"}, ExpectedVersion: &version},
		CatalogID: "catalog", CatalogItemID: "item",
	})
	if err != nil { t.Fatal(err) }
	if repo.last.ShortDescriptionPresent || repo.last.LongDescriptionPresent {
		t.Fatalf("omitted descriptions became present: %#v", repo.last)
	}
}

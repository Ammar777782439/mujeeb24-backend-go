package services

import (
	"context"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type offerUpdateRepo struct {
	ports.CatalogRepository
	current ports.OfferRecord
	variant ports.VariantRecord
	patch   ports.OfferPatch
}

func (r *offerUpdateRepo) GetOffer(context.Context, string, string) (ports.OfferRecord, error) {
	return r.current, nil
}
func (r *offerUpdateRepo) GetVariant(context.Context, string, string) (ports.VariantRecord, error) {
	return r.variant, nil
}
func (r *offerUpdateRepo) UpdateOffer(_ context.Context, patch ports.OfferPatch) (ports.OfferRecord, error) {
	r.patch = patch
	next := r.current
	next.ResourceVersion++
	next.UpdatedAt = patch.UpdatedAt
	if patch.PricingMode != nil {
		next.PricingMode = *patch.PricingMode
	}
	if patch.AmountPresent && patch.AmountMinor == nil {
		next.Amount = nil
	}
	if patch.CurrencyPresent {
		next.Currency = patch.Currency
	}
	if patch.VariantIDPresent {
		next.VariantID = patch.VariantID
	}
	return next, nil
}

type offerUpdateTx struct{}

func (offerUpdateTx) Within(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func offerUpdateService(repo *offerUpdateRepo, now time.Time) UpdateOfferCommandService {
	return UpdateOfferCommandService{CatalogCommandServices{
		Repository: repo, Transactions: offerUpdateTx{}, Now: func() time.Time { return now },
	}}
}
func offerVersion(value string) *commands.ResourceVersion {
	v := commands.ResourceVersion(value)
	return &v
}

func TestUpdateOfferAllowsFixedToQuoteRequiredWithExplicitPriceClear(t *testing.T) {
	amount, currency := "1250.00", "YER"
	now := time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC)
	repo := &offerUpdateRepo{current: ports.OfferRecord{
		ID: "offer-1", BusinessID: "business-1", CatalogItemID: "item-1",
		Name: "سعر البيع", PricingMode: "fixed", Amount: &amount, Currency: &currency,
		AvailabilityMode: "stock", AvailabilityStatus: "available",
		FulfillmentMode: "delivery", Status: "active", ResourceVersion: 4,
	}}
	mode := "quote_required"
	_, err := offerUpdateService(repo, now).Handle(context.Background(), commands.UpdateOfferCommand{
		Meta:    commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}, ExpectedVersion: offerVersion("4")},
		OfferID: "offer-1", PricingMode: &mode, AmountPresent: true, CurrencyPresent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !repo.patch.AmountPresent || repo.patch.AmountMinor != nil || !repo.patch.CurrencyPresent || repo.patch.Currency != nil {
		t.Fatalf("explicit price clearing was lost: %+v", repo.patch)
	}
	if repo.patch.PriceSource == nil || *repo.patch.PriceSource != "merchant" ||
		repo.patch.PriceVerificationStatus == nil || *repo.patch.PriceVerificationStatus != "verified" ||
		repo.patch.PriceCheckedAt == nil || !repo.patch.PriceCheckedAt.Equal(now) {
		t.Fatalf("merchant price provenance was not refreshed: %+v", repo.patch)
	}
}

func TestUpdateOfferRejectsClearingAmountForFixedPrice(t *testing.T) {
	amount, currency := "1250.00", "YER"
	repo := &offerUpdateRepo{current: ports.OfferRecord{
		ID: "offer-1", BusinessID: "business-1", CatalogItemID: "item-1",
		Name: "سعر البيع", PricingMode: "fixed", Amount: &amount, Currency: &currency,
		AvailabilityMode: "stock", AvailabilityStatus: "available",
		FulfillmentMode: "delivery", Status: "active", ResourceVersion: 4,
	}}
	_, err := offerUpdateService(repo, time.Now()).Handle(context.Background(), commands.UpdateOfferCommand{
		Meta:    commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}, ExpectedVersion: offerVersion("4")},
		OfferID: "offer-1", AmountPresent: true,
	})
	if err == nil {
		t.Fatal("fixed pricing accepted an explicit amount clear")
	}
}

func TestUpdateOfferRejectsVariantFromAnotherItem(t *testing.T) {
	amount, currency := "1250.00", "YER"
	variantID := commands.VariantID("variant-2")
	repo := &offerUpdateRepo{
		current: ports.OfferRecord{
			ID: "offer-1", BusinessID: "business-1", CatalogItemID: "item-1",
			Name: "سعر البيع", PricingMode: "fixed", Amount: &amount, Currency: &currency,
			AvailabilityMode: "stock", AvailabilityStatus: "available",
			FulfillmentMode: "delivery", Status: "active", ResourceVersion: 4,
		},
		variant: ports.VariantRecord{ID: "variant-2", BusinessID: "business-1", CatalogItemID: "item-2"},
	}
	_, err := offerUpdateService(repo, time.Now()).Handle(context.Background(), commands.UpdateOfferCommand{
		Meta:    commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}, ExpectedVersion: offerVersion("4")},
		OfferID: "offer-1", VariantID: &variantID, VariantIDPresent: true,
	})
	if err == nil {
		t.Fatal("offer accepted a variant from another catalog item")
	}
}

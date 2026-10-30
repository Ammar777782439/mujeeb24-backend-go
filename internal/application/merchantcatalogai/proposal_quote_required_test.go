package merchantcatalogai

import "testing"

// Contract regression: no stated price must not force a fabricated lower bound
// or a needless merchant price question when quote_required is supported.
func TestProposalAllowsQuoteRequiredCreateWithoutMerchantAmount(t *testing.T) {
	currency := "YER"
	p := Proposal{
		SchemaVersion: ProposalSchemaVersion,
		Status:        StatusResolved,
		Operation:     OperationCreate,
		ResponseText:  "أعددت اقتراح إنشاء الخدمة للمراجعة.",
		Create: &ItemCreate{
			Name:                 "استشارة تجريبية",
			ItemType:             "service",
			AvailabilityMode:     "unknown",
			FulfillmentMode:      "appointment",
			RequiresConfirmation: false,
			Offers: []OfferCreate{{
				Name:               DefaultOfferName,
				NameSource:         OfferNameSourceSystemDefault,
				PricingMode:        "quote_required",
				PriceSource:        OfferPriceSourceNotStated,
				Currency:           &currency,
				AvailabilityMode:   "unknown",
				AvailabilityStatus: "unknown",
				FulfillmentMode:    "appointment",
				Status:             "draft",
			}},
		},
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("unpriced quote_required proposal should validate: %v", err)
	}
	p.Create.Offers[0].PricingMode = "starting_from"
	if err := p.Validate(); err == nil {
		t.Fatal("starting_from without merchant-stated numeric amount must fail")
	}
}

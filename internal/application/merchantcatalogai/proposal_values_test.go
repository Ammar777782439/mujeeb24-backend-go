package merchantcatalogai

import "testing"

func TestProposalRejectsUnsafeExistingOfferUpdates(t *testing.T) {
	valid := func() Proposal {
		return Proposal{SchemaVersion: ProposalSchemaVersion, Status: StatusResolved, Operation: OperationUpdate, ResponseText: "اقتراح تعديل", Update: &UpdateOperation{ItemID: "item-1", ExistingOffers: []OfferUpdate{{ID: "offer-1", Amount: stringPtr("400.0000")}}}}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Proposal)
	}{
		{"negative", func(p *Proposal) { p.Update.ExistingOffers[0].Amount = stringPtr("-1") }},
		{"non numeric", func(p *Proposal) { p.Update.ExistingOffers[0].Amount = stringPtr("NaN") }},
		{"excess precision", func(p *Proposal) { p.Update.ExistingOffers[0].Amount = stringPtr("1.00001") }},
		{"overflow", func(p *Proposal) { p.Update.ExistingOffers[0].Amount = stringPtr("10000000000000000") }},
		{"duplicate ID", func(p *Proposal) {
			p.Update.ExistingOffers = append(p.Update.ExistingOffers, p.Update.ExistingOffers[0])
		}},
		{"empty ID", func(p *Proposal) { p.Update.ExistingOffers[0].ID = "" }},
		{"no changed field", func(p *Proposal) { p.Update.ExistingOffers[0].Amount = nil }},
		{"invalid status", func(p *Proposal) { p.Update.Changes.Status = stringPtr("expired") }},
		{"nonresolved payload", func(p *Proposal) { p.Status = StatusNeedsMoreData; p.Operation = OperationAskMerchant }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := valid()
			tc.mutate(&p)
			if err := p.Validate(); err == nil {
				t.Fatal("unsafe proposal accepted")
			}
		})
	}
	for _, value := range []string{"0", "0.0000", "400.0000", "9999999999999999.9999"} {
		p := valid()
		p.Update.ExistingOffers[0].Amount = stringPtr(value)
		if err := p.Validate(); err != nil {
			t.Fatalf("valid amount %s: %v", value, err)
		}
	}
}

func TestProposalSupportsDescriptionOnlyUpdateAndCustomItemType(t *testing.T) {
	p := Proposal{SchemaVersion: ProposalSchemaVersion, Status: StatusResolved, Operation: OperationUpdate, ResponseText: "اقتراح تعديل الوصف", Update: &UpdateOperation{ItemID: "item-1", Changes: ItemChanges{LongDescription: stringPtr("تشمل الإفطار"), ItemType: stringPtr("custom_vertical_item")}}}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}

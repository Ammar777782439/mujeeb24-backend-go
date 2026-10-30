package merchantcatalogai

import (
	"context"
	"errors"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type parentEvidenceRepo struct {
	ports.CatalogRepository
	item      ports.CatalogItemRecord
	variants  []ports.VariantRecord
	offers    []ports.OfferRecord
	failRead  bool
	parentGet int
}

func (r *parentEvidenceRepo) GetCatalogItem(_ context.Context, business, catalog, item string) (ports.CatalogItemRecord, error) {
	r.parentGet++
	if r.failRead || business != "business-1" || catalog != "catalog-1" || item != "item-1" {
		return ports.CatalogItemRecord{}, errors.New("item not found in authorized scope")
	}
	return r.item, nil
}
func (r *parentEvidenceRepo) ListVariants(_ context.Context, business, item, status string, limit int, cursor string) (ports.VariantPage, error) {
	if business != "business-1" || item != "item-1" {
		return ports.VariantPage{}, errors.New("invalid variant scope")
	}
	return ports.VariantPage{Items: r.variants}, nil
}
func (r *parentEvidenceRepo) ListOffers(_ context.Context, business, item, status string, limit int, cursor string) (ports.OfferPage, error) {
	if business != "business-1" || item != "item-1" {
		return ports.OfferPage{}, errors.New("invalid offer scope")
	}
	return ports.OfferPage{Items: r.offers}, nil
}

func verifiedParentEvidenceRepo() *parentEvidenceRepo {
	return &parentEvidenceRepo{
		item: ports.CatalogItemRecord{
			ID: "item-1", BusinessID: "business-1", CatalogID: "catalog-1",
		},
		variants: []ports.VariantRecord{{
			ID: "variant-black", CatalogItemID: "item-1", BusinessID: "business-1",
		}},
		offers: []ports.OfferRecord{{
			ID: "offer-white", CatalogItemID: "item-1", BusinessID: "business-1",
		}},
	}
}

func TestCatalogParentReferenceIsProvenByScopedChildRead(t *testing.T) {
	for _, tool := range []string{"merchant_catalog_list_variants", "merchant_catalog_list_offers"} {
		for _, empty := range []bool{false, true} {
			t.Run(tool+"/empty="+map[bool]string{true: "yes", false: "no"}[empty], func(t *testing.T) {
				repo := verifiedParentEvidenceRepo()
				if empty {
					repo.variants, repo.offers = nil, nil
				}
				r := NewReadOnlyCapabilityRegistry(repo, "catalog-1")
				scope := MerchantCatalogDiscoveryExecutionContext{
					BusinessID: "business-1", PrincipalID: "owner-1", ConversationID: "session-1",
				}
				result, err := r.Execute(context.Background(), scope, tool, []byte(`{"item_id":"item-1"}`))
				if err != nil {
					t.Fatal(err)
				}
				if repo.parentGet != 1 {
					t.Fatalf("expected one scoped parent verification, got %d", repo.parentGet)
				}
				if len(result.EvidenceReferences) == 0 || result.EvidenceReferences[0] != "item-1" {
					t.Fatalf("verified parent missing from capability result: %#v", result.EvidenceReferences)
				}
				p := Proposal{Status: StatusResolved, Operation: OperationUpdate, Update: &UpdateOperation{
					ItemID: "item-1", NewVariants: []VariantCreate{{Ref: "new-yellow", Name: "أصفر"}},
				}}
				if err := r.ValidateProposalReferences(p); err != nil {
					t.Fatalf("verified parent should be accepted for update: %v", err)
				}
				p.Update.ItemID = "fabricated-id"
				if err := r.ValidateProposalReferences(p); err == nil {
					t.Fatal("unread item ID accepted just because a child tool ran")
				}
			})
		}
	}
}

func TestCatalogParentReferenceIsNotGrantedOnFailedOrMismatchedRead(t *testing.T) {
	for _, mode := range []string{"missing", "wrong-business", "wrong-catalog", "wrong-id"} {
		t.Run(mode, func(t *testing.T) {
			repo := verifiedParentEvidenceRepo()
			switch mode {
			case "missing":
				repo.failRead = true
			case "wrong-business":
				repo.item.BusinessID = "business-other"
			case "wrong-catalog":
				repo.item.CatalogID = "catalog-other"
			case "wrong-id":
				repo.item.ID = "item-other"
			}
			r := NewReadOnlyCapabilityRegistry(repo, "catalog-1")
			_, err := r.Execute(context.Background(), MerchantCatalogDiscoveryExecutionContext{
				BusinessID: "business-1", PrincipalID: "owner-1", ConversationID: "session-1",
			}, "merchant_catalog_list_variants", []byte(`{"item_id":"item-1"}`))
			if err == nil {
				t.Fatalf("invalid parent proof accepted: %s", mode)
			}
			p := Proposal{Status: StatusResolved, Operation: OperationUpdate, Update: &UpdateOperation{ItemID: "item-1"}}
			if err := r.ValidateProposalReferences(p); err == nil {
				t.Fatal("failed or cross-scope read incorrectly granted mutation authority")
			}
		})
	}
}

func TestUnrelatedReadCannotGrantParentItemAuthority(t *testing.T) {
	r := NewReadOnlyCapabilityRegistry(nil, "catalog-1")
	r.recordEvidence(MerchantCatalogDiscoveryResult{
		EvidenceReferences: []string{"variant-black"},
		Operation:          "merchant_catalog_list_variants",
	})
	p := Proposal{Status: StatusResolved, Operation: OperationUpdate, Update: &UpdateOperation{ItemID: "item-1"}}
	if err := r.ValidateProposalReferences(p); err == nil {
		t.Fatal("unrelated evidence incorrectly authorizes a catalog item")
	}
}

package merchantcatalogai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

type StoredProposal struct {
	VariantAttributes map[string][]byte
	ID                string
	BusinessID        string
	PrincipalID       string
	SessionID         string
	CatalogID         string
	Proposal          Proposal
	Versions          map[string]int64
	Result            *ExecutionResult
}

type ExecutionStore interface {
	Save(context.Context, StoredProposal) error
	Lock(context.Context, string, string, string) (StoredProposal, error)
	Complete(context.Context, string, string, ExecutionResult) error
}

type ExecutionService struct {
	Catalogs     ports.CatalogRepository
	Transactions ports.TransactionManager
	Store        ExecutionStore
	Sessions     SessionStore
}

func (s ExecutionService) Prepare(ctx context.Context, in ExecutionInput) (string, error) {
	if err := in.Proposal.Validate(); err != nil {
		return "", err
	}
	if in.Proposal.Status != StatusResolved || !in.Proposal.IsMutation() {
		return "", nil
	}
	p := StoredProposal{ID: uuid.NewString(), BusinessID: in.BusinessID, PrincipalID: in.PrincipalID, SessionID: in.SessionID, CatalogID: in.SelectedCatalog.ID, Proposal: in.Proposal, Versions: map[string]int64{}}
	if err := s.snapshot(ctx, &p); err != nil {
		return "", err
	}
	if err := s.Store.Save(ctx, p); err != nil {
		return "", err
	}
	log.Printf("[MerchantCatalogAI][PROPOSAL_STORED] business=%s session=%s proposal=%s catalog=%s operation=%s", p.BusinessID, p.SessionID, p.ID, p.CatalogID, p.Proposal.Operation)
	return p.ID, nil
}

func (s ExecutionService) snapshot(ctx context.Context, p *StoredProposal) error {
	p.VariantAttributes = map[string][]byte{}
	itemID := ""
	if p.Proposal.Update != nil {
		itemID = p.Proposal.Update.ItemID
	}
	if p.Proposal.Delete != nil {
		itemID = p.Proposal.Delete.ItemID
	}
	if itemID == "" {
		return nil
	}
	item, err := s.Catalogs.GetCatalogItem(ctx, p.BusinessID, p.CatalogID, itemID)
	if err != nil {
		return err
	}
	if item.BusinessID != p.BusinessID || item.CatalogID != p.CatalogID {
		return appErrors.New(appErrors.CodeForbidden, "catalog item scope mismatch")
	}
	p.Versions["item:"+itemID] = item.ResourceVersion
	if p.Proposal.Update == nil {
		return nil
	}
	u := p.Proposal.Update
	offers := map[string]int64{}
	variants := map[string]int64{}
	if len(u.ExistingOffers) > 0 {
		cursor := ""
		for {
			page, e := s.Catalogs.ListOffers(ctx, p.BusinessID, itemID, "", 100, cursor)
			if e != nil {
				return e
			}
			for _, v := range page.Items {
				if v.BusinessID != p.BusinessID || v.CatalogItemID != itemID {
					return appErrors.New(appErrors.CodeForbidden, "offer scope mismatch")
				}
				offers[v.ID] = v.ResourceVersion
			}
			if !page.HasMore {
				break
			}
			if page.NextCursor == "" || page.NextCursor == cursor {
				return fmt.Errorf("invalid offer cursor")
			}
			cursor = page.NextCursor
		}
	}
	if len(u.ExistingVariants) > 0 || len(u.NewOffers) > 0 {
		cursor := ""
		for {
			page, e := s.Catalogs.ListVariants(ctx, p.BusinessID, itemID, "", 100, cursor)
			if e != nil {
				return e
			}
			for _, v := range page.Items {
				if v.BusinessID != p.BusinessID || v.CatalogItemID != itemID {
					return appErrors.New(appErrors.CodeForbidden, "variant scope mismatch")
				}
				variants[v.ID] = v.ResourceVersion
				p.VariantAttributes[v.ID] = v.Attributes
			}
			if !page.HasMore {
				break
			}
			if page.NextCursor == "" || page.NextCursor == cursor {
				return fmt.Errorf("invalid variant cursor")
			}
			cursor = page.NextCursor
		}
	}
	for _, v := range u.ExistingOffers {
		version, ok := offers[v.ID]
		if !ok {
			return appErrors.New(appErrors.CodeNotFound, "offer does not belong to target item")
		}
		p.Versions["offer:"+v.ID] = version
	}
	for _, v := range u.ExistingVariants {
		version, ok := variants[v.ID]
		if !ok {
			return appErrors.New(appErrors.CodeNotFound, "variant does not belong to target item")
		}
		p.Versions["variant:"+v.ID] = version
	}
	for _, v := range u.NewOffers {
		if v.VariantID != nil {
			version, ok := variants[*v.VariantID]
			if !ok {
				return appErrors.New(appErrors.CodeNotFound, "offer variant does not belong to target item")
			}
			p.Versions["variant:"+*v.VariantID] = version
		}
	}
	return nil
}

func (s ExecutionService) ExecuteApproved(ctx context.Context, businessID, principalID, proposalID string) (ExecutionResult, error) {
	var result ExecutionResult
	err := s.Transactions.Within(ctx, func(tx context.Context) error {
		p, e := s.Store.Lock(tx, businessID, principalID, proposalID)
		if e != nil {
			return e
		}
		if p.Result != nil {
			result = *p.Result
			return nil
		}
		if e = p.Proposal.Validate(); e != nil {
			return e
		}
		current := p
		current.Versions = map[string]int64{}
		if e = s.snapshot(tx, &current); e != nil {
			return e
		}
		for key, version := range p.Versions {
			if current.Versions[key] != version {
				return appErrors.New(appErrors.CodeStaleResource, "catalog changed since proposal review; request a fresh proposal")
			}
		}
		result, e = s.apply(tx, p)
		if e != nil {
			return e
		}
		if e = s.Store.Complete(tx, businessID, proposalID, result); e != nil {
			return e
		}
		if s.Sessions != nil {
			data, _ := json.Marshal(result)
			_, e = s.Sessions.AppendMessage(tx, businessID, p.SessionID, "assistant", "تم حفظ تغييرات الكتالوج بنجاح. نتيجة التنفيذ: "+string(data))
			if e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("[MerchantCatalogAI][EXECUTION_FAILED] business=%s proposal=%s error=%v", businessID, proposalID, err)
		return ExecutionResult{}, err
	}
	log.Printf("[MerchantCatalogAI][EXECUTION_COMMITTED] business=%s proposal=%s item=%s variants=%v offers=%v", businessID, proposalID, result.ItemID, result.VariantIDs, result.OfferIDs)
	return result, nil
}

func encodedAttributes(v map[string]any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
func mergeAttributes(old []byte, changes map[string]any) ([]byte, error) {
	if changes == nil {
		return nil, nil
	}
	v := map[string]any{}
	if len(old) > 0 {
		if e := json.Unmarshal(old, &v); e != nil {
			return nil, e
		}
	}
	for k, x := range changes {
		v[k] = x
	}
	return json.Marshal(v)
}

func (s ExecutionService) apply(ctx context.Context, p StoredProposal) (ExecutionResult, error) {
	r := ExecutionResult{}
	now := time.Now().UTC()
	q := p.Proposal
	var variants []VariantCreate
	var offers []OfferCreate
	if q.Create != nil {
		c := q.Create
		attrs, e := encodedAttributes(c.Attributes)
		if e != nil {
			return r, e
		}
		var sv *int
		if c.AttributeSchemaID != nil {
			schema, e := s.Catalogs.GetAttributeSchema(ctx, p.BusinessID, *c.AttributeSchemaID)
			if e != nil {
				return r, e
			}
			sv = &schema.Version
		}
		item, e := s.Catalogs.CreateCatalogItem(ctx, ports.CatalogItemDraft{ID: uuid.NewString(), BusinessID: p.BusinessID, CatalogID: p.CatalogID, Name: c.Name, ItemType: c.ItemType, ShortDescription: c.ShortDescription, LongDescription: c.LongDescription, AttributeSchemaID: c.AttributeSchemaID, AttributeSchemaVersion: sv, PricingMode: c.PricingMode, AvailabilityMode: c.AvailabilityMode, FulfillmentMode: c.FulfillmentMode, RequiresConfirmation: c.RequiresConfirmation, Attributes: attrs, CreatedAt: now, UpdatedAt: now})
		if e != nil {
			return r, e
		}
		r.ItemID = item.ID
		variants = c.Variants
		offers = c.Offers
	}
	if q.Delete != nil {
		r.ItemID = q.Delete.ItemID
		status := "archived"
		_, e := s.Catalogs.UpdateCatalogItem(ctx, ports.CatalogItemPatch{ID: r.ItemID, BusinessID: p.BusinessID, Status: &status, ExpectedVersion: p.Versions["item:"+r.ItemID], UpdatedAt: now})
		return r, e
	}
	if q.Update != nil {
		u := q.Update
		r.ItemID = u.ItemID
		c := u.Changes
		item, e := s.Catalogs.GetCatalogItem(ctx, p.BusinessID, p.CatalogID, r.ItemID)
		if e != nil {
			return r, e
		}
		attrs, e := mergeAttributes(item.Attributes, c.Attributes)
		if e != nil {
			return r, e
		}
		if c.Name != nil || c.ItemType != nil || c.Status != nil || c.ShortDescription != nil || c.LongDescription != nil || c.PricingMode != nil || c.AvailabilityMode != nil || c.FulfillmentMode != nil || c.RequiresConfirmation != nil || c.Attributes != nil {
			_, e = s.Catalogs.UpdateCatalogItem(ctx, ports.CatalogItemPatch{ID: r.ItemID, BusinessID: p.BusinessID, Name: c.Name, ItemType: c.ItemType, Status: c.Status, ShortDescription: c.ShortDescription, LongDescription: c.LongDescription, PricingMode: c.PricingMode, AvailabilityMode: c.AvailabilityMode, FulfillmentMode: c.FulfillmentMode, RequiresConfirmation: c.RequiresConfirmation, Attributes: attrs, ExpectedVersion: p.Versions["item:"+r.ItemID], UpdatedAt: now})
			if e != nil {
				return r, e
			}
		}
		for _, v := range u.ExistingOffers {
			_, e = s.Catalogs.UpdateOffer(ctx, ports.OfferPatch{ID: v.ID, BusinessID: p.BusinessID, Name: v.Name, AmountDecimal: v.Amount, Status: v.Status, AvailabilityStatus: v.AvailabilityStatus, ExpectedVersion: p.Versions["offer:"+v.ID], UpdatedAt: now})
			if e != nil {
				return r, e
			}
			r.OfferIDs = append(r.OfferIDs, v.ID)
		}
		for _, v := range u.ExistingVariants {
			attrs, e := mergeAttributes(p.VariantAttributes[v.ID], v.Attributes)
			if e != nil {
				return r, e
			}
			_, e = s.Catalogs.UpdateVariant(ctx, ports.VariantPatch{ID: v.ID, BusinessID: p.BusinessID, Name: v.Name, Attributes: attrs, Status: v.Status, ExpectedVersion: p.Versions["variant:"+v.ID], UpdatedAt: now})
			if e != nil {
				return r, e
			}
			r.VariantIDs = append(r.VariantIDs, v.ID)
		}
		variants = u.NewVariants
		offers = u.NewOffers
	}
	refs := map[string]string{}
	for _, v := range variants {
		attrs, e := encodedAttributes(v.Attributes)
		if e != nil {
			return r, e
		}
		created, e := s.Catalogs.CreateVariant(ctx, ports.VariantDraft{ID: uuid.NewString(), BusinessID: p.BusinessID, CatalogItemID: r.ItemID, Name: v.Name, Attributes: attrs, Status: "active", CreatedAt: now, UpdatedAt: now})
		if e != nil {
			return r, e
		}
		refs[v.Ref] = created.ID
		r.VariantIDs = append(r.VariantIDs, created.ID)
	}
	for _, o := range offers {
		variantID := o.VariantID
		if o.VariantRef != nil {
			v, ok := refs[*o.VariantRef]
			if !ok {
				return r, fmt.Errorf("unknown variant ref")
			}
			variantID = &v
		}
		created, e := s.Catalogs.CreateOffer(ctx, ports.OfferDraft{ID: uuid.NewString(), BusinessID: p.BusinessID, CatalogItemID: r.ItemID, VariantID: variantID, Name: o.Name, PricingMode: o.PricingMode, AmountDecimal: o.Amount, PriceSource: &o.PriceSource, Currency: o.Currency, PricingUnit: o.PricingUnit, AvailabilityMode: o.AvailabilityMode, AvailabilityStatus: o.AvailabilityStatus, FulfillmentMode: o.FulfillmentMode, Status: o.Status, CreatedAt: now, UpdatedAt: now})
		if e != nil {
			return r, e
		}
		r.OfferIDs = append(r.OfferIDs, created.ID)
	}
	return r, nil
}

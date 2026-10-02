package merchantcatalogai

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var attributeKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)

func validateAttributeObject(attributes map[string]any, path string) error {
	for key := range attributes {
		if !attributeKeyPattern.MatchString(key) {
			return fmt.Errorf("%s contains invalid attribute key %q: expected English snake_case", path, key)
		}
	}
	return nil
}

func validateOfferCreate(offer OfferCreate, path string, variantRefs map[string]struct{}, allowVariantID bool) error {
	if offer.Currency == nil || strings.TrimSpace(*offer.Currency) == "" {
		return fmt.Errorf("%s requires currency from merchant statement or business default_currency", path)
	}
	if strings.TrimSpace(offer.Name) == "" {
		return fmt.Errorf("%s requires name", path)
	}
	switch offer.NameSource {
	case OfferNameSourceSystemDefault:
		if offer.Name != DefaultOfferName {
			return fmt.Errorf("system-default %s name must be %q", path, DefaultOfferName)
		}
	case OfferNameSourceMerchantStated:
	default:
		return fmt.Errorf("%s requires a valid name_source", path)
	}

	switch offer.PriceSource {
	case OfferPriceSourceMerchantStated:
		if offer.Amount == nil || strings.TrimSpace(*offer.Amount) == "" {
			return fmt.Errorf("merchant-stated %s price requires amount", path)
		}
		if offer.PricingMode == "quote_required" {
			return fmt.Errorf("merchant-stated %s price cannot use quote_required pricing_mode", path)
		}
	case OfferPriceSourceNotStated:
		if offer.Amount != nil && strings.TrimSpace(*offer.Amount) != "" {
			return fmt.Errorf("not-stated %s price cannot contain amount", path)
		}
	default:
		return fmt.Errorf("%s requires a valid price_source", path)
	}

	switch offer.PricingMode {
	case "fixed", "starting_from":
		if offer.Amount == nil || strings.TrimSpace(*offer.Amount) == "" {
			return fmt.Errorf("%s pricing_mode %s requires amount", path, offer.PricingMode)
		}
	case "per_unit", "per_person", "per_day":
		if offer.Amount == nil || strings.TrimSpace(*offer.Amount) == "" {
			return fmt.Errorf("%s pricing_mode %s requires amount", path, offer.PricingMode)
		}
		if offer.PricingUnit == nil || strings.TrimSpace(*offer.PricingUnit) == "" {
			return fmt.Errorf("%s pricing_mode %s requires pricing_unit", path, offer.PricingMode)
		}
	case "quote_required":
		if offer.Amount != nil && strings.TrimSpace(*offer.Amount) != "" {
			return fmt.Errorf("%s quote_required pricing cannot contain amount", path)
		}
	case "dynamic":
	default:
		return fmt.Errorf("%s pricing_mode is invalid: %s", path, offer.PricingMode)
	}

	if !allowVariantID && offer.VariantID != nil {
		return fmt.Errorf("%s cannot contain variant_id; use variant_ref for a new variant", path)
	}
	if offer.VariantID != nil && offer.VariantRef != nil {
		return fmt.Errorf("%s cannot contain both variant_id and variant_ref", path)
	}
	if offer.VariantRef != nil {
		ref := strings.TrimSpace(*offer.VariantRef)
		if ref == "" {
			return fmt.Errorf("%s variant_ref cannot be empty", path)
		}
		if _, exists := variantRefs[ref]; !exists {
			return fmt.Errorf("%s references unknown variant_ref: %s", path, ref)
		}
	}
	return nil
}
type Proposal struct {
	SchemaVersion int `json:"schema_version"`
	Status ProposalStatus `json:"status"`
	Operation Operation `json:"operation"`
	ResponseText string `json:"response_text"`
	EvidenceReferences []string `json:"evidence_references,omitempty"`
	MissingInformation []MissingField `json:"missing_information,omitempty"`
	Create *ItemCreate `json:"create,omitempty"`
	Update *UpdateOperation `json:"update,omitempty"`
	Delete *DeleteOperation `json:"delete,omitempty"`
}

func (p Proposal) IsMutation() bool {
	return p.Operation == OperationCreate || p.Operation == OperationUpdate || p.Operation == OperationDelete
}

// Normalize enforces the B2B semantic boundary after Gemini's structured output.
// Structured output guarantees shape, not that the requested business operation
// is actually complete. A resolved empty update is therefore converted to a
// clarification instead of being presented as processed.
func (p Proposal) Normalize() Proposal {
	// Gemini can occasionally return a mutation operation together with a
	// non-resolved status (for example: status=needs_more_data, operation=update).
	// That shape is semantically invalid for the closed B2B contract. Normalize it
	// into a pure clarification proposal before Validate(), rather than leaking a
	// provider formatting mistake as a 500 response.
	if p.Status != StatusResolved && p.IsMutation() {
		p.Operation = OperationAskMerchant
		p.Create = nil
		p.Update = nil
		p.Delete = nil
		if strings.TrimSpace(p.ResponseText) == "" {
			p.ResponseText = "حدّد البيانات الناقصة المطلوبة لإتمام العملية."
		}
		if len(p.MissingInformation) == 0 {
			p.MissingInformation = []MissingField{
				{
					Path:        "proposal",
					DisplayName: "بيانات العملية",
					DataType:    "object",
					Reason:      "لا يمكن اعتماد عملية تعديل أو إنشاء قبل اكتمال البيانات المطلوبة.",
				},
			}
		}
		return p
	}

	if p.Status == StatusResolved && p.Operation == OperationUpdate && p.Update != nil {
		u := p.Update
		emptyItemChanges := u.Changes.Name == nil &&
			u.Changes.ItemType == nil &&
			u.Changes.ShortDescription == nil &&
			u.Changes.LongDescription == nil &&
			u.Changes.Status == nil &&
			u.Changes.PricingMode == nil &&
			u.Changes.AvailabilityMode == nil &&
			u.Changes.FulfillmentMode == nil &&
			u.Changes.Attributes == nil &&
			u.Changes.RequiresConfirmation == nil
		if strings.TrimSpace(u.ItemID) == "" ||
			(emptyItemChanges && len(u.ExistingVariants) == 0 && len(u.NewVariants) == 0 &&
				len(u.ExistingOffers) == 0 && len(u.NewOffers) == 0) {
			p.Status = StatusNeedsMoreData
			p.Operation = OperationAskMerchant
			p.Update = nil
			p.Create = nil
			p.Delete = nil
			p.MissingInformation = []MissingField{
				{Path: "update.item_id", DisplayName: "المنتج", DataType: "catalog_item", Reason: "يجب تحديد المنتج المراد تعديله."},
				{Path: "update.changes", DisplayName: "بيانات التعديل", DataType: "object", Reason: "يجب تحديد القيمة أو البيانات الجديدة المطلوب اعتمادها."},
			}
			p.ResponseText = "حدّد المنتج والبيانات الجديدة التي تريد تعديلها."
		}
	}
	return p
}

func (p Proposal) Validate() error {
	if p.SchemaVersion != ProposalSchemaVersion {
		return fmt.Errorf("merchant catalog proposal schema_version must be %d", ProposalSchemaVersion)
	}
	if strings.TrimSpace(p.ResponseText) == "" {
		return errors.New("merchant catalog proposal response_text is required")
	}
	switch p.Status {
	case StatusResolved, StatusAmbiguous, StatusNotFound, StatusNeedsMoreData:
	default:
		return errors.New("merchant catalog proposal status is invalid")
	}
	switch p.Operation {
	case OperationCreate, OperationUpdate, OperationDelete, OperationAskMerchant:
	default:
		return errors.New("merchant catalog proposal operation is invalid")
	}
	if p.Status != StatusResolved {
		if p.IsMutation() {
			return errors.New("non-resolved proposal cannot contain a mutation operation")
		}
		return nil
	}
	switch p.Operation {
	case OperationCreate:
		if p.Create == nil || strings.TrimSpace(p.Create.Name) == "" {
			return errors.New("create proposal requires item data")
		}
		if p.Update != nil || p.Delete != nil {
			return errors.New("create proposal cannot contain update/delete data")
		}
		if err := validateAttributeObject(p.Create.Attributes, "create.attributes"); err != nil {
			return err
		}
		for _, variant := range p.Create.Variants {
			if err := validateAttributeObject(variant.Attributes, "create.variants.attributes"); err != nil {
				return err
			}
		}
		variantRefs := make(map[string]struct{}, len(p.Create.Variants))
		for _, variant := range p.Create.Variants {
			ref := strings.TrimSpace(variant.Ref)
			if ref == "" {
				return errors.New("create variant requires ref")
			}
			if _, exists := variantRefs[ref]; exists {
				return fmt.Errorf("duplicate create variant ref: %s", ref)
			}
			variantRefs[ref] = struct{}{}
		}

		for _, offer := range p.Create.Offers {
			if err := validateOfferCreate(offer, "create.offer", variantRefs, false); err != nil {
				return err
			}
		}
	case OperationUpdate:
		if p.Update == nil || strings.TrimSpace(p.Update.ItemID) == "" {
			return errors.New("update proposal requires item_id")
		}
		if p.Create != nil || p.Delete != nil {
			return errors.New("update proposal cannot contain create/delete data")
		}
		if err := validateAttributeObject(p.Update.Changes.Attributes, "update.changes.attributes"); err != nil {
			return err
		}
		for _, variant := range p.Update.ExistingVariants {
			if err := validateAttributeObject(variant.Attributes, "update.existing_variants.attributes"); err != nil {
				return err
			}
		}
		for _, variant := range p.Update.NewVariants {
			if err := validateAttributeObject(variant.Attributes, "update.new_variants.attributes"); err != nil {
				return err
			}
		}

		newVariantRefs := make(map[string]struct{}, len(p.Update.NewVariants))
		for _, variant := range p.Update.NewVariants {
			ref := strings.TrimSpace(variant.Ref)
			if ref == "" {
				return errors.New("new variant requires ref")
			}
			if _, exists := newVariantRefs[ref]; exists {
				return fmt.Errorf("duplicate new variant ref: %s", ref)
			}
			newVariantRefs[ref] = struct{}{}
		}
		for _, offer := range p.Update.NewOffers {
			if err := validateOfferCreate(offer, "update.new_offers.offer", newVariantRefs, true); err != nil {
				return err
			}
		}
	case OperationDelete:
		if p.Delete == nil || strings.TrimSpace(p.Delete.ItemID) == "" {
			return errors.New("delete proposal requires item_id")
		}
		if p.Create != nil || p.Update != nil {
			return errors.New("delete proposal cannot contain create/update data")
		}
	case OperationAskMerchant:
		if p.Create != nil || p.Update != nil || p.Delete != nil {
			return errors.New("ask_merchant proposal cannot contain mutation data")
		}
	}
	return nil
}

package merchantcatalogai

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

var proposalAmountPattern = regexp.MustCompile(`^[0-9]{1,16}(\.[0-9]{1,4})?$`)
var proposalCurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

func validateProposalValues(p Proposal) error {
	d := services.DefaultCatalogEntityContractDescriptor()
	check := func(value, path string, allowed map[string]string) error {
		if _, ok := allowed[value]; !ok {
			return fmt.Errorf("%s has unsupported value %q", path, value)
		}
		return nil
	}
	optional := func(value *string, path string, allowed map[string]string) error {
		if value == nil {
			return nil
		}
		return check(*value, path, allowed)
	}
	amount := func(value *string, path string) error {
		if value != nil && !proposalAmountPattern.MatchString(*value) {
			return fmt.Errorf("%s must be a non-negative decimal within NUMERIC(20,4)", path)
		}
		return nil
	}
	validateOffers := func(offers []OfferCreate) error {
		for _, offer := range offers {
			if err := amount(offer.Amount, "offer.amount"); err != nil {
				return err
			}
			if offer.Currency == nil || !proposalCurrencyPattern.MatchString(*offer.Currency) {
				return fmt.Errorf("offer.currency must be a three-letter uppercase code")
			}
			for _, c := range []struct {
				value, path string
				allowed     map[string]string
			}{
				{offer.AvailabilityMode, "offer.availability_mode", d.AvailabilityModes},
				{offer.AvailabilityStatus, "offer.availability_status", d.AvailabilityStatuses},
				{offer.FulfillmentMode, "offer.fulfillment_mode", d.FulfillmentModes},
				{offer.Status, "offer.status", d.OfferStatuses},
			} {
				if err := check(c.value, c.path, c.allowed); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if p.Create != nil {
		c := p.Create
		if strings.TrimSpace(c.ItemType) == "" {
			return fmt.Errorf("create.item_type is required")
		}
		for _, c := range []struct {
			value, path string
			allowed     map[string]string
		}{
			{c.PricingMode, "create.pricing_mode", d.PricingModes},
			{c.AvailabilityMode, "create.availability_mode", d.AvailabilityModes},
			{c.FulfillmentMode, "create.fulfillment_mode", d.FulfillmentModes},
		} {
			if err := check(c.value, c.path, c.allowed); err != nil {
				return err
			}
		}
		if len(c.Offers) == 0 {
			return fmt.Errorf("resolved create requires at least one offer")
		}
		for _, variant := range c.Variants {
			if strings.TrimSpace(variant.Name) == "" {
				return fmt.Errorf("create variant requires name")
			}
		}
		if err := validateOffers(c.Offers); err != nil {
			return err
		}
	}
	if p.Update != nil {
		u := p.Update
		for _, value := range []*string{u.Changes.Name, u.Changes.ItemType} {
			if value != nil && strings.TrimSpace(*value) == "" {
				return fmt.Errorf("updated name/item_type cannot be empty")
			}
		}
		for _, c := range []struct {
			value   *string
			path    string
			allowed map[string]string
		}{
			{u.Changes.PricingMode, "changes.pricing_mode", d.PricingModes},
			{u.Changes.AvailabilityMode, "changes.availability_mode", d.AvailabilityModes},
			{u.Changes.FulfillmentMode, "changes.fulfillment_mode", d.FulfillmentModes},
			{u.Changes.Status, "changes.status", d.ItemStatuses},
		} {
			if err := optional(c.value, c.path, c.allowed); err != nil {
				return err
			}
		}
		ids := make(map[string]bool)
		clearRequested := func(value *bool) bool { return value != nil && *value }
		for _, offer := range u.ExistingOffers {
			if strings.TrimSpace(offer.ID) == "" || ids[offer.ID] {
				return fmt.Errorf("existing offer ID is empty or duplicated")
			}
			ids[offer.ID] = true
			changed := offer.VariantID != nil || clearRequested(offer.ClearVariant) ||
				offer.Name != nil || offer.PricingMode != nil ||
				offer.Amount != nil || clearRequested(offer.ClearAmount) ||
				offer.Currency != nil || clearRequested(offer.ClearCurrency) ||
				offer.PricingUnit != nil || clearRequested(offer.ClearPricingUnit) ||
				offer.AvailabilityMode != nil || offer.AvailabilityStatus != nil ||
				offer.FulfillmentMode != nil ||
				offer.ValidityFrom != nil || clearRequested(offer.ClearValidityFrom) ||
				offer.ValidityUntil != nil || clearRequested(offer.ClearValidityUntil) ||
				offer.Status != nil
			if !changed {
				return fmt.Errorf("existing offer update requires a changed field")
			}
			for _, pair := range []struct {
				value *string
				clear *bool
				path  string
			}{
				{offer.VariantID, offer.ClearVariant, "existing_offer.variant_id"},
				{offer.Amount, offer.ClearAmount, "existing_offer.amount"},
				{offer.Currency, offer.ClearCurrency, "existing_offer.currency"},
				{offer.PricingUnit, offer.ClearPricingUnit, "existing_offer.pricing_unit"},
				{offer.ValidityFrom, offer.ClearValidityFrom, "existing_offer.validity_from"},
				{offer.ValidityUntil, offer.ClearValidityUntil, "existing_offer.validity_until"},
			} {
				if pair.value != nil && clearRequested(pair.clear) {
					return fmt.Errorf("%s cannot be set and cleared in the same update", pair.path)
				}
			}
			if offer.Name != nil && strings.TrimSpace(*offer.Name) == "" {
				return fmt.Errorf("existing_offer.name cannot be empty")
			}
			if offer.VariantID != nil && strings.TrimSpace(*offer.VariantID) == "" {
				return fmt.Errorf("existing_offer.variant_id cannot be empty")
			}
			if err := amount(offer.Amount, "existing_offer.amount"); err != nil {
				return err
			}
			if offer.Currency != nil && !proposalCurrencyPattern.MatchString(*offer.Currency) {
				return fmt.Errorf("existing_offer.currency must be a three-letter uppercase code")
			}
			for _, value := range []*string{offer.ValidityFrom, offer.ValidityUntil} {
				if value != nil {
					if _, err := time.Parse(time.RFC3339, *value); err != nil {
						return fmt.Errorf("offer validity timestamps must use RFC3339")
					}
				}
			}
			for _, choice := range []struct {
				value   *string
				path    string
				allowed map[string]string
			}{
				{offer.PricingMode, "existing_offer.pricing_mode", d.PricingModes},
				{offer.AvailabilityMode, "existing_offer.availability_mode", d.AvailabilityModes},
				{offer.AvailabilityStatus, "existing_offer.availability_status", d.AvailabilityStatuses},
				{offer.FulfillmentMode, "existing_offer.fulfillment_mode", d.FulfillmentModes},
				{offer.Status, "existing_offer.status", d.OfferStatuses},
			} {
				if err := optional(choice.value, choice.path, choice.allowed); err != nil {
					return err
				}
			}
		}
		ids = make(map[string]bool)
		for _, variant := range u.ExistingVariants {
			if strings.TrimSpace(variant.ID) == "" || ids[variant.ID] {
				return fmt.Errorf("existing variant ID is empty or duplicated")
			}
			ids[variant.ID] = true
			if variant.Name == nil && variant.Attributes == nil && variant.Status == nil {
				return fmt.Errorf("existing variant update requires a changed field")
			}
			if err := optional(variant.Status, "existing_variant.status", d.VariantStatuses); err != nil {
				return err
			}
		}
		for _, variant := range u.NewVariants {
			if strings.TrimSpace(variant.Name) == "" {
				return fmt.Errorf("new variant requires name")
			}
		}
		if err := validateOffers(u.NewOffers); err != nil {
			return err
		}
	}
	return nil
}

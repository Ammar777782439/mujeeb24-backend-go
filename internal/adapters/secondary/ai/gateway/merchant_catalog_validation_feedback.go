package gateway

import (
	"sort"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

// merchantCatalogValidationFeedback provides only allowlisted contract facts.
// Do NOT interpolate raw validator errors into prompts: some errors contain
// untrusted model-authored field values, labels or references.
func merchantCatalogValidationFeedback(err error) string {
	const general = "The previous proposal failed Go validation. Rebuild the entire strict merchant_catalog_wire. If merchant facts are insufficient, return operation=ask_merchant with status=needs_more_data and empty mutation and attribute-definition sections. Never invent prices, availability, delivery modes, references or merchant consent."
	if err == nil {
		return general
	}
	d := services.DefaultCatalogEntityContractDescriptor()
	fields := []struct {
		path    string
		choices map[string]string
	}{
		{path: "offer.availability_mode", choices: d.AvailabilityModes},
		{path: "offer.availability_status", choices: d.AvailabilityStatuses},
		{path: "offer.fulfillment_mode", choices: d.FulfillmentModes},
		{path: "offer.status", choices: d.OfferStatuses},
		{path: "create.availability_mode", choices: d.AvailabilityModes},
		{path: "create.fulfillment_mode", choices: d.FulfillmentModes},
		{path: "create.pricing_mode", choices: d.PricingModes},
	}
	for _, field := range fields {
		if !strings.HasPrefix(err.Error(), field.path+" has unsupported value ") {
			continue
		}
		allowed := make([]string, 0, len(field.choices))
		for value := range field.choices {
			allowed = append(allowed, value)
		}
		sort.Strings(allowed)
		return "Go rejected " + field.path + " because its value is not in the canonical catalog contract. Allowed values: " +
			strings.Join(allowed, ", ") + ". Every required offer field must have an explicit allowed value, not an empty string. " +
			"For availability_mode or availability_status, 'unknown' explicitly means not confirmed; never claim availability without evidence. " +
			"If the missing facts prevent a trustworthy commercial proposal, ask the merchant instead of inventing fields. " +
			"Return a complete strict proposal. Keep inactive create/update/delete payloads empty when asking."
	}
	return general
}

package gateway

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

const merchantCatalogMaxRepairIssues = 24

var merchantCatalogSafeRepairKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// merchantCatalogWireFeedback reports multiple invalid fields in one pass.
// Never echo model-authored values, labels, IDs or validator strings into a prompt.
func merchantCatalogWireFeedback(raw string) string {
	const fallback = "Rebuild the complete strict wire from merchant facts and trusted catalog evidence. If required facts or select options are unknown, return operation=ask_merchant, status=needs_more_data and empty mutations and attribute definitions. Never invent prices, availability, delivery promises, choices or IDs."
	if len(raw) == 0 || len(raw) > 524288 {
		return fallback
	}
	var w catalogWireProposal
	if json.Unmarshal([]byte(raw), &w) != nil {
		return fallback
	}
	d := services.DefaultCatalogEntityContractDescriptor()
	issues := make([]string, 0, 12)
	seen := map[string]bool{}
	add := func(s string) {
		if len(issues) < merchantCatalogMaxRepairIssues && !seen[s] {
			seen[s] = true
			issues = append(issues, s)
		}
	}
	enum := func(path string, fields []catalogWireField, name string, required bool, allowed map[string]string) {
		found, valid := false, false
		for _, f := range fields {
			if f.Name != name {
				continue
			}
			found = true
			var value string
			if json.Unmarshal([]byte(f.ValueJSON), &value) == nil {
				_, valid = allowed[value]
			}
			break
		}
		if valid || (!found && !required) {
			return
		}
		choices := make([]string, 0, len(allowed))
		for value := range allowed {
			choices = append(choices, value)
		}
		sort.Strings(choices)
		add(path + "." + name + ": missing, empty or invalid; valid values: [" + strings.Join(choices, ", ") + "]")
	}
	text := func(path string, fields []catalogWireField, name string) {
		for _, f := range fields {
			if f.Name != name {
				continue
			}
			var value string
			if json.Unmarshal([]byte(f.ValueJSON), &value) == nil && strings.TrimSpace(value) != "" {
				return
			}
			break
		}
		add(path + "." + name + ": required nonempty string")
	}
	offer := func(path string, fields []catalogWireField, required bool) {
		enum(path, fields, "pricing_mode", required, d.PricingModes)
		enum(path, fields, "availability_mode", required, d.AvailabilityModes)
		enum(path, fields, "availability_status", required, d.AvailabilityStatuses)
		enum(path, fields, "fulfillment_mode", required, d.FulfillmentModes)
		enum(path, fields, "status", required, d.OfferStatuses)
		if required {
			text(path, fields, "name")
			text(path, fields, "currency")
			enum(path, fields, "name_source", true, map[string]string{
				merchantcatalogai.OfferNameSourceSystemDefault:  "",
				merchantcatalogai.OfferNameSourceMerchantStated: "",
			})
			enum(path, fields, "price_source", true, map[string]string{
				merchantcatalogai.OfferPriceSourceMerchantStated: "",
				merchantcatalogai.OfferPriceSourceNotStated:      "",
			})
		}
	}
	switch w.Operation {
	case merchantcatalogai.OperationCreate:
		text("create", w.Create.Fields, "name")
		text("create", w.Create.Fields, "item_type")
		enum("create", w.Create.Fields, "pricing_mode", true, d.PricingModes)
		enum("create", w.Create.Fields, "availability_mode", true, d.AvailabilityModes)
		enum("create", w.Create.Fields, "fulfillment_mode", true, d.FulfillmentModes)
		if len(w.Create.Offers) == 0 {
			add("create.offers: a resolved create needs a valid offer; ask_merchant when facts are missing")
		}
		for i, o := range w.Create.Offers {
			if i >= merchantCatalogMaxRepairIssues {
				break
			}
			offer(fmt.Sprintf("create.offers[%d]", i), o.Fields, true)
		}
	case merchantcatalogai.OperationUpdate:
		enum("update.changes", w.Update.Changes, "pricing_mode", false, d.PricingModes)
		enum("update.changes", w.Update.Changes, "availability_mode", false, d.AvailabilityModes)
		enum("update.changes", w.Update.Changes, "fulfillment_mode", false, d.FulfillmentModes)
		enum("update.changes", w.Update.Changes, "status", false, d.ItemStatuses)
		for i, o := range w.Update.NewOffers {
			if i >= merchantCatalogMaxRepairIssues {
				break
			}
			offer(fmt.Sprintf("update.new_offers[%d]", i), o.Fields, true)
		}
		for i, o := range w.Update.ExistingOffers {
			if i >= merchantCatalogMaxRepairIssues {
				break
			}
			offer(fmt.Sprintf("update.existing_offers[%d]", i), o.Fields, false)
		}
	case merchantcatalogai.OperationAskMerchant:
		if len(w.AttributeDefinitions) > 0 || w.AttributeSchemaName != "" ||
			len(w.Create.Fields) > 0 || len(w.Create.Variants) > 0 || len(w.Create.Offers) > 0 ||
			len(w.Update.Changes) > 0 || len(w.Update.NewOffers) > 0 || len(w.Update.NewVariants) > 0 ||
			len(w.Update.ExistingOffers) > 0 || len(w.Update.ExistingVariants) > 0 ||
			w.Update.ItemID != "" || w.Update.AttributeSchemaID != "" || w.Delete.ID != "" {
			add("ask_merchant: all inactive mutation and template payloads must be empty")
		}
	}
	for i, def := range w.AttributeDefinitions {
		if def.DataType != "select" && def.DataType != "multi_select" {
			continue
		}
		var rules struct {
			Enum []json.RawMessage `json:"enum"`
		}
		if json.Unmarshal([]byte(def.ValidationRulesJSON), &rules) == nil && len(rules.Enum) >= 1 && len(rules.Enum) <= 100 {
			continue
		}
		name := fmt.Sprintf("attribute_definitions[%d]", i)
		if merchantCatalogSafeRepairKey.MatchString(def.Key) {
			name += "." + def.Key
		}
		add(name + ": select/multi_select needs explicit nonempty enum of merchant-stated or evidence-backed values; otherwise ask_merchant")
	}
	if len(issues) == 0 {
		return fallback
	}
	return "Go rejected ALL the following fields (raw values withheld):\n- " + strings.Join(issues, "\n- ") +
		"\nCorrect them together using merchant facts or authorized evidence only. " +
		"'unknown' availability is explicitly unconfirmed, not available. " +
		"Never invent select choices, pricing, fulfillment promises or catalog IDs. " +
		"If facts are missing, choose operation=ask_merchant with status=needs_more_data, " +
		"attribute_definitions=[], attribute_schema_name=\"\" and empty create/update/delete payloads. " +
		"Return a COMPLETE strict wire."
}

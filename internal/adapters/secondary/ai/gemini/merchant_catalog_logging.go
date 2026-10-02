package gemini

import (
	"log"

	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

func merchantCatalogStepTypes(resp merchantCatalogInteractionResponse) []string {
	types := make([]string, 0, len(resp.Steps))
	for _, step := range resp.Steps {
		types = append(types, step.Type)
	}
	return types
}

func merchantCatalogCallNames(calls []merchantCatalogFunctionCall) []string {
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.Name)
	}
	return names
}

func derefString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func logMerchantCatalogProposalDetail(input merchantcatalogai.RuntimeInput, proposal merchantcatalogai.Proposal) {
	detail := map[string]any{
		"business":       input.BusinessID,
		"session":        input.SessionID,
		"status":         proposal.Status,
		"operation":      proposal.Operation,
		"schema_version": proposal.SchemaVersion,
		"evidence_count": len(proposal.EvidenceReferences),
		"missing_count":  len(proposal.MissingInformation),
	}

	if proposal.Create != nil {
		detail["create"] = map[string]any{
			"name":          proposal.Create.Name,
			"item_type":     proposal.Create.ItemType,
			"pricing_mode":  proposal.Create.PricingMode,
			"availability":  proposal.Create.AvailabilityMode,
			"fulfillment":   proposal.Create.FulfillmentMode,
			"variant_count": len(proposal.Create.Variants),
			"offer_count":   len(proposal.Create.Offers),
			"variants": func() []map[string]string {
				out := make([]map[string]string, 0, len(proposal.Create.Variants))
				for _, v := range proposal.Create.Variants {
					out = append(out, map[string]string{"ref": v.Ref, "name": v.Name})
				}
				return out
			}(),
			"offers": func() []map[string]any {
				out := make([]map[string]any, 0, len(proposal.Create.Offers))
				for _, o := range proposal.Create.Offers {
					out = append(out, map[string]any{
						"variant_ref":         derefString(o.VariantRef),
						"name":                o.Name,
						"name_source":         o.NameSource,
						"pricing_mode":        o.PricingMode,
						"amount":              derefString(o.Amount),
						"price_source":        o.PriceSource,
						"currency":            derefString(o.Currency),
						"availability_status": o.AvailabilityStatus,
						"fulfillment_mode":    o.FulfillmentMode,
					})
				}
				return out
			}(),
		}
	}

	if proposal.Update != nil {
		detail["update"] = map[string]any{
			"item_id":           proposal.Update.ItemID,
			"existing_variants": len(proposal.Update.ExistingVariants),
			"new_variants":      len(proposal.Update.NewVariants),
			"existing_offers":   len(proposal.Update.ExistingOffers),
			"new_offers":        len(proposal.Update.NewOffers),
		}
	}

	log.Printf("[MerchantCatalogAI][PROPOSAL_DETAIL] %+v", detail)
}

func logGeminiRetry(status, attempt, maxAttempts int, delay time.Duration) {
	log.Printf("[MerchantCatalogAI] GEMINI_RETRY status=%d attempt=%d/%d delay=%s", status, attempt, maxAttempts, delay)
}

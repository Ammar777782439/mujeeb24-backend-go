package gemini

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

func TestMerchantCatalogUpdateSchemaMatchesSupportedChanges(t *testing.T) {
	schema := merchantCatalogProposalSchema()
	properties := schema["properties"].(map[string]any)
	update := properties["update"].(map[string]any)["anyOf"].([]map[string]any)[0]["properties"].(map[string]any)
	checks := []struct {
		name  string
		typ   reflect.Type
		props map[string]any
	}{
		{"changes", reflect.TypeOf(merchantcatalogai.ItemChanges{}), update["changes"].(map[string]any)["properties"].(map[string]any)},
		{"existing_offers", reflect.TypeOf(merchantcatalogai.OfferUpdate{}), update["existing_offers"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)},
	}
	for _, check := range checks {
		for i := 0; i < check.typ.NumField(); i++ {
			key := strings.Split(check.typ.Field(i).Tag.Get("json"), ",")[0]
			if _, ok := check.props[key]; !ok {
				t.Errorf("%s missing supported field %s", check.name, key)
			}
		}
		if len(check.props) != check.typ.NumField() {
			t.Errorf("%s exposes unsupported fields", check.name)
		}
	}
}

func TestParseMerchantCatalogExistingOfferPriceAndDescriptionUpdate(t *testing.T) {
	proposal, err := parseMerchantCatalogProposal(merchantCatalogInteractionResponse{Steps: []merchantCatalogInteractionStep{{Type: "model_output", Content: []merchantCatalogOutputPart{{Type: "text", Text: `{"schema_version":3,"status":"resolved","operation":"update","response_text":"اقتراح تعديل السعر والوصف","create":null,"delete":null,"update":{"item_id":"item-1","changes":{"long_description":"رحلة تشمل وجبة الغداء"},"existing_offers":[{"id":"offer-1","amount":"450.0000"}]}}`}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Update == nil || proposal.Update.Changes.LongDescription == nil || *proposal.Update.Changes.LongDescription != "رحلة تشمل وجبة الغداء" {
		t.Fatal("description update lost")
	}
	if len(proposal.Update.ExistingOffers) != 1 || proposal.Update.ExistingOffers[0].Amount == nil || *proposal.Update.ExistingOffers[0].Amount != "450.0000" {
		t.Fatal("existing offer price update lost")
	}
	if len(proposal.Update.NewOffers) != 0 {
		t.Fatal("price update became new offer")
	}
}

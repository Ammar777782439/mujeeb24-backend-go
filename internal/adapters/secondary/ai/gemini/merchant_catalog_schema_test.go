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

func TestMerchantCatalogProposalSchemaCoversAllOperationFields(t *testing.T) {
	schema := merchantCatalogProposalSchema()
	properties := schema["properties"].(map[string]any)
	create := properties["create"].(map[string]any)["anyOf"].([]map[string]any)[0]["properties"].(map[string]any)
	update := properties["update"].(map[string]any)["anyOf"].([]map[string]any)[0]["properties"].(map[string]any)
	deletion := properties["delete"].(map[string]any)["anyOf"].([]map[string]any)[0]["properties"].(map[string]any)
	checks := []struct {
		name     string
		typ      reflect.Type
		props    map[string]any
		excluded string
	}{
		{"proposal", reflect.TypeOf(merchantcatalogai.Proposal{}), properties, ""},
		{"create", reflect.TypeOf(merchantcatalogai.ItemCreate{}), create, ""},
		{"update", reflect.TypeOf(merchantcatalogai.UpdateOperation{}), update, ""},
		{"delete", reflect.TypeOf(merchantcatalogai.DeleteOperation{}), deletion, ""},
		{"create.variants", reflect.TypeOf(merchantcatalogai.VariantCreate{}), create["variants"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any), ""},
		{"create.offers", reflect.TypeOf(merchantcatalogai.OfferCreate{}), create["offers"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any), "variant_id"},
		{"update.new_variants", reflect.TypeOf(merchantcatalogai.VariantCreate{}), update["new_variants"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any), ""},
		{"update.existing_variants", reflect.TypeOf(merchantcatalogai.VariantUpdate{}), update["existing_variants"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any), ""},
		{"update.new_offers", reflect.TypeOf(merchantcatalogai.OfferCreate{}), update["new_offers"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any), ""},
		{"missing_information", reflect.TypeOf(merchantcatalogai.MissingField{}), properties["missing_information"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any), ""},
	}
	for _, check := range checks {
		for i := 0; i < check.typ.NumField(); i++ {
			key := strings.Split(check.typ.Field(i).Tag.Get("json"), ",")[0]
			if key == check.excluded {
				continue
			}
			if _, ok := check.props[key]; !ok {
				t.Errorf("%s missing supported field %s", check.name, key)
			}
		}
	}
}

func TestParseMerchantCatalogProposalRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	for _, raw := range []string{
		`{"schema_version":3,"create":{"discount_percent":20}}`,
		`{"schema_version":3,"delete":{"item_id":"wrong-contract"}}`,
		`{"schema_version":3} {"operation":"delete"}`,
	} {
		if _, err := parseMerchantCatalogProposal(merchantCatalogInteractionResponse{OutputText: raw}); err == nil {
			t.Fatalf("accepted unsupported or trailing data: %s", raw)
		}
	}
	if _, err := parseMerchantCatalogProposal(merchantCatalogInteractionResponse{OutputText: `{"schema_version":3,"create":{"attributes":{"discount_label":{"text":"informational only"},"custom_array":[1,false,null]}}}`}); err != nil {
		t.Fatalf("dynamic JSON attributes rejected: %v", err)
	}
}

func TestMerchantCatalogOfferSchemaRequiresBothProvenances(t *testing.T) {
	root := merchantCatalogProposalSchema()["properties"].(map[string]any)
	create := root["create"].(map[string]any)["anyOf"].([]map[string]any)[0]["properties"].(map[string]any)
	offer := create["offers"].(map[string]any)["items"].(map[string]any)
	for _, branch := range offer["anyOf"].([]map[string]any) {
		properties := branch["properties"].(map[string]any)
		if properties["name_source"] == nil || properties["price_source"] == nil {
			t.Fatal("offer schema permits one provenance rule to bypass the other")
		}
	}
	if offer["additionalProperties"] != false {
		t.Fatal("structured offer must reject unknown fields")
	}
	attributes := create["attributes"].(map[string]any)
	if attributes["additionalProperties"] != true {
		t.Fatal("dynamic attributes must remain open")
	}
}

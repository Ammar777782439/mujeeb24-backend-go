package gateway

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
	proposal, err := parseMerchantCatalogProposal(`{"schema_version":3,"status":"resolved","operation":"update","response_text":"اقتراح تعديل السعر والوصف","create":null,"delete":null,"update":{"item_id":"item-1","changes":{"long_description":"رحلة تشمل وجبة الغداء"},"existing_offers":[{"id":"offer-1","amount":"450.0000"}]}}`)
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
		if _, err := parseMerchantCatalogProposal(raw); err == nil {
			t.Fatalf("accepted unsupported or trailing data: %s", raw)
		}
	}
	if _, err := parseMerchantCatalogProposal(`{"schema_version":3,"create":{"attributes":{"discount_label":{"text":"informational only"},"custom_array":[1,false,null]}}}`); err != nil {
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

func TestMerchantCatalogOfferVariantReferencesCannotBeEmpty(t *testing.T) {
	root := merchantCatalogProposalSchema()["properties"].(map[string]any)
	update := root["update"].(map[string]any)["anyOf"].([]map[string]any)[0]["properties"].(map[string]any)

	existingOffer := update["existing_offers"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	newOffer := update["new_offers"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)

	for name, field := range map[string]map[string]any{
		"existing_offers.variant_id": existingOffer["variant_id"].(map[string]any),
		"new_offers.variant_id":      newOffer["variant_id"].(map[string]any),
	} {
		if field["minLength"] != 1 {
			t.Fatalf("%s must reject empty strings: %#v", name, field)
		}
		description, _ := field["description"].(string)
		if !strings.Contains(description, "omit") || !strings.Contains(description, "empty string") {
			t.Fatalf("%s must explain direct-offer omission semantics: %q", name, description)
		}
	}
}

func TestMerchantCatalogPricingModeOwnedOnlyByOffers(t *testing.T) {
	wire := merchantCatalogWireSchema()["properties"].(map[string]any)
	create := wire["create"].(map[string]any)["properties"].(map[string]any)
	update := wire["update"].(map[string]any)["properties"].(map[string]any)
	fieldNames := func(fields map[string]any) []string {
		return fields["items"].(map[string]any)["properties"].(map[string]any)["name"].(map[string]any)["enum"].([]string)
	}
	has := func(names []string, key string) bool {
		for _, name := range names {
			if name == key {
				return true
			}
		}
		return false
	}
	for scope, fields := range map[string]map[string]any{
		"create.fields":  create["fields"].(map[string]any),
		"update.changes": update["changes"].(map[string]any),
	} {
		if has(fieldNames(fields), "pricing_mode") {
			t.Errorf("%s must not expose catalog item pricing_mode", scope)
		}
	}
	if !has(fieldNames(create["offers"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["fields"].(map[string]any)), "pricing_mode") {
		t.Error("create.offers must expose pricing_mode")
	}
	if !has(fieldNames(update["new_offers"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["fields"].(map[string]any)), "pricing_mode") {
		t.Error("update.new_offers must expose pricing_mode")
	}
	if !has(fieldNames(update["existing_offers"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["fields"].(map[string]any)), "pricing_mode") {
		t.Error("update.existing_offers must expose pricing_mode")
	}

	proposal := merchantCatalogProposalSchema()["properties"].(map[string]any)
	item := proposal["create"].(map[string]any)["anyOf"].([]map[string]any)[0]["properties"].(map[string]any)
	changes := proposal["update"].(map[string]any)["anyOf"].([]map[string]any)[0]["properties"].(map[string]any)["changes"].(map[string]any)["properties"].(map[string]any)
	if _, ok := item["pricing_mode"]; ok {
		t.Error("canonical item create schema exposes duplicated pricing_mode")
	}
	if _, ok := changes["pricing_mode"]; ok {
		t.Error("canonical item changes schema exposes duplicated pricing_mode")
	}
}

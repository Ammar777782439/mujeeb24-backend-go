package gateway

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

func TestMerchantFeedbackReportsDependentCommercialAndVariantFields(t *testing.T) {
	wire := emptyCatalogWireProposal()
	wire.Operation = merchantcatalogai.OperationCreate
	wire.Status = merchantcatalogai.StatusResolved
	wire.Update.ItemID = ""
	wire.Create.Fields = []catalogWireField{
		{Name: "name", ValueJSON: `"هاتف تجريبي"`},
		{Name: "item_type", ValueJSON: `"physical_good"`},
		{Name: "pricing_mode", ValueJSON: `"per_unit"`},
		{Name: "availability_mode", ValueJSON: `"unknown"`},
		{Name: "fulfillment_mode", ValueJSON: `"manual"`},
	}
	wire.Create.Variants = []catalogWireEntity{{Fields: []catalogWireField{
		{Name: "name", ValueJSON: `"المقاس الكبير"`},
	}}}
	wire.Create.Offers = []catalogWireEntity{{Fields: []catalogWireField{
		{Name: "name", ValueJSON: `"سعر البيع"`},
		{Name: "name_source", ValueJSON: `"system_default"`},
		{Name: "price_source", ValueJSON: `"merchant_stated"`},
		{Name: "pricing_mode", ValueJSON: `"per_unit"`},
		{Name: "amount", ValueJSON: `""`},
		{Name: "currency", ValueJSON: `"secret-lowercase-yer"`},
		{Name: "availability_mode", ValueJSON: `"unknown"`},
		{Name: "availability_status", ValueJSON: `"unknown"`},
		{Name: "fulfillment_mode", ValueJSON: `"manual"`},
		{Name: "status", ValueJSON: `"draft"`},
	}}}
	wire.AttributeDefinitions = []catalogWireAttributeDefinition{{
		Key: "color", Label: "secret-english-label", DataType: "select", ValidationRulesJSON: `{}`,
	}}
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	feedback := merchantCatalogWireFeedback(string(raw))
	for _, field := range []string{
		"create.requires_confirmation",
		"create.variants[0].ref",
		"create.offers[0].amount",
		"create.offers[0].pricing_unit",
		"create.offers[0].currency",
		"attribute_definitions[0].color.label",
		"attribute_definitions[0].color",
	} {
		if !strings.Contains(feedback, field) {
			t.Errorf("missing dependent field feedback %q", field)
		}
	}
	if strings.Contains(feedback, "secret-lowercase-yer") ||
		strings.Contains(feedback, "secret-english-label") {
		t.Fatal("merchant data leaked into repair instruction")
	}
}

func TestMerchantFeedbackPermitsUnpricedQuoteRequiredOffer(t *testing.T) {
	wire := emptyCatalogWireProposal()
	wire.Operation = merchantcatalogai.OperationCreate
	wire.Status = merchantcatalogai.StatusResolved
	wire.Update.ItemID = ""
	wire.Create.Fields = []catalogWireField{
		{Name: "name", ValueJSON: `"خدمة تجريبية"`},
		{Name: "item_type", ValueJSON: `"service"`},
		{Name: "pricing_mode", ValueJSON: `"quote_required"`},
		{Name: "availability_mode", ValueJSON: `"unknown"`},
		{Name: "fulfillment_mode", ValueJSON: `"manual"`},
		{Name: "requires_confirmation", ValueJSON: `true`},
	}
	wire.Create.Offers = []catalogWireEntity{{Fields: []catalogWireField{
		{Name: "name", ValueJSON: `"سعر البيع"`},
		{Name: "name_source", ValueJSON: `"system_default"`},
		{Name: "price_source", ValueJSON: `"not_stated"`},
		{Name: "pricing_mode", ValueJSON: `"quote_required"`},
		{Name: "amount", ValueJSON: `null`},
		{Name: "currency", ValueJSON: `"YER"`},
		{Name: "availability_mode", ValueJSON: `"unknown"`},
		{Name: "availability_status", ValueJSON: `"unknown"`},
		{Name: "fulfillment_mode", ValueJSON: `"manual"`},
		{Name: "status", ValueJSON: `"draft"`},
	}}}
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	feedback := merchantCatalogWireFeedback(string(raw))
	if strings.Contains(feedback, "create.offers[0].amount") ||
		strings.Contains(feedback, "create.offers[0].pricing_unit") {
		t.Fatalf("valid quote_required pricing incorrectly requires a specified price: %s", feedback)
	}
}

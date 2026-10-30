package gateway

import (
	"encoding/json"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

func emptyCatalogWireProposal() catalogWireProposal {
	return catalogWireProposal{
		SchemaVersion:        merchantcatalogai.ProposalSchemaVersion,
		AttributeDefinitions: []catalogWireAttributeDefinition{},
		Status:               merchantcatalogai.StatusResolved,
		Operation:            merchantcatalogai.OperationUpdate,
		ResponseText:         "اقتراح جاهز للمراجعة",
		EvidenceReferences:   []string{},
		MissingInformation:   []merchantcatalogai.MissingField{},
		Create: catalogWireCreate{
			Fields: []catalogWireField{}, Variants: []catalogWireEntity{}, Offers: []catalogWireEntity{},
		},
		Update: catalogWireUpdate{
			ItemID: "item-1", Changes: []catalogWireField{},
			ExistingVariants: []catalogWireExistingEntity{}, NewVariants: []catalogWireEntity{},
			ExistingOffers: []catalogWireExistingEntity{}, NewOffers: []catalogWireEntity{},
		},
		Delete: catalogWireDelete{},
	}
}
func decodeCatalogWireForTest(t *testing.T, wire catalogWireProposal) (merchantcatalogai.Proposal, error) {
	t.Helper()
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	return catalogWireParse(string(encoded))
}

func TestMerchantStrictWireExistingOfferAndDynamicAttributes(t *testing.T) {
	wire := emptyCatalogWireProposal()
	wire.AttributeDefinitions = []catalogWireAttributeDefinition{
		{Key: "warranty", Label: "الضمان", DataType: "text", ValidationRulesJSON: `{}`},
		{Key: "battery_health", Label: "صحة البطارية", DataType: "number", ValidationRulesJSON: `{}`},
		{Key: "certified", Label: "معتمد", DataType: "boolean", ValidationRulesJSON: `{}`},
		{Key: "features", Label: "المميزات", DataType: "multi_select", ValidationRulesJSON: `{"enum":["A18","OLED"]}`},
		{Key: "details", Label: "التفاصيل", DataType: "location", ValidationRulesJSON: `{}`},
	}
	wire.Update.Changes = []catalogWireField{
		{Name: "attributes", ValueJSON: `{"warranty":"سنتان","battery_health":99,"certified":true,"features":["A18","OLED"],"details":{"country":"YE"}}`},
	}
	wire.Update.ExistingOffers = []catalogWireExistingEntity{{
		ID: "offer-1",
		Fields: []catalogWireField{
			{Name: "amount", ValueJSON: `"600.0000"`},
			{Name: "currency", ValueJSON: `"YER"`},
		},
	}}
	proposal, err := decodeCatalogWireForTest(t, wire)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Update == nil || proposal.Update.ItemID != "item-1" {
		t.Fatal("update target lost")
	}
	attrs := proposal.Update.Changes.Attributes
	if attrs["warranty"] != "سنتان" || attrs["certified"] != true || attrs["battery_health"] == nil {
		t.Fatalf("attribute values lost: %v", attrs)
	}
	if len(proposal.Update.ExistingOffers) != 1 ||
		proposal.Update.ExistingOffers[0].ID != "offer-1" ||
		proposal.Update.ExistingOffers[0].Amount == nil ||
		*proposal.Update.ExistingOffers[0].Amount != "600.0000" {
		t.Fatalf("offer price changed incorrectly: %+v", proposal.Update.ExistingOffers)
	}
	if len(proposal.Update.NewOffers) > 0 {
		t.Fatal("existing offer update was converted into new offer")
	}
}

func TestMerchantStrictWireRejectsMisplacedExistingOffers(t *testing.T) {
	wire := emptyCatalogWireProposal()
	encoded, _ := json.Marshal(wire)
	var doc map[string]any
	if err := json.Unmarshal(encoded, &doc); err != nil {
		t.Fatal(err)
	}
	doc["existing_offers"] = []any{map[string]any{"id": "offer-1"}}
	encoded, _ = json.Marshal(doc)
	if _, err := catalogWireParse(string(encoded)); err == nil {
		t.Fatal("accepted existing_offers outside update")
	}
}

func TestMerchantStrictWireRejectsUnknownDuplicateAndTrailingValue(t *testing.T) {
	for name, fields := range map[string][]catalogWireField{
		"unknown_field":   {{Name: "existing_offers", ValueJSON: `[]`}},
		"duplicate":       {{Name: "name", ValueJSON: `"new"`}, {Name: "name", ValueJSON: `"other"`}},
		"trailing":        {{Name: "name", ValueJSON: `"new" "other"`}},
		"unintended_null": {{Name: "name", ValueJSON: `null`}},
	} {
		t.Run(name, func(t *testing.T) {
			wire := emptyCatalogWireProposal()
			wire.Update.Changes = fields
			if _, err := decodeCatalogWireForTest(t, wire); err == nil {
				t.Fatal("accepted unsafe field payload")
			}
		})
	}
}

func TestMerchantStrictWireRejectsInactiveMutations(t *testing.T) {
	wire := emptyCatalogWireProposal()
	wire.Operation = merchantcatalogai.OperationAskMerchant
	wire.Update.Changes = []catalogWireField{{Name: "name", ValueJSON: `"ignored"`}}
	if _, err := decodeCatalogWireForTest(t, wire); err == nil {
		t.Fatal("accepted writes in ask_merchant operation")
	}
}

func TestMerchantStrictWireCreatesVariantsAndOffers(t *testing.T) {
	wire := emptyCatalogWireProposal()
	wire.Operation = merchantcatalogai.OperationCreate
	wire.Update.ItemID = ""
	wire.AttributeDefinitions = []catalogWireAttributeDefinition{
		{Key: "duration_minutes", Label: "المدة بالدقائق", DataType: "number", ValidationRulesJSON: `{}`},
	}
	wire.Create.Fields = []catalogWireField{
		{Name: "name", ValueJSON: `"خدمة تجريبية"`},
		{Name: "item_type", ValueJSON: `"service"`},
		{Name: "pricing_mode", ValueJSON: `"fixed"`},
		{Name: "availability_mode", ValueJSON: `"always_available"`},
		{Name: "fulfillment_mode", ValueJSON: `"pickup"`},
		{Name: "requires_confirmation", ValueJSON: `true`},
		{Name: "attributes", ValueJSON: `{"duration_minutes":45}`},
	}
	wire.Create.Variants = []catalogWireEntity{{Fields: []catalogWireField{
		{Name: "ref", ValueJSON: `"v-1"`}, {Name: "name", ValueJSON: `"أساسي"`},
	}}}
	wire.Create.Offers = []catalogWireEntity{{Fields: []catalogWireField{
		{Name: "name", ValueJSON: `"سعر البيع"`}, {Name: "name_source", ValueJSON: `"system_default"`},
		{Name: "pricing_mode", ValueJSON: `"fixed"`}, {Name: "amount", ValueJSON: `"500.0000"`},
		{Name: "price_source", ValueJSON: `"merchant_stated"`}, {Name: "currency", ValueJSON: `"YER"`},
		{Name: "variant_ref", ValueJSON: `"v-1"`},
		{Name: "availability_mode", ValueJSON: `"always_available"`},
		{Name: "availability_status", ValueJSON: `"available"`},
		{Name: "fulfillment_mode", ValueJSON: `"pickup"`},
		{Name: "status", ValueJSON: `"active"`},
	}}}
	proposal, err := decodeCatalogWireForTest(t, wire)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Create == nil || len(proposal.Create.Variants) != 1 || len(proposal.Create.Offers) != 1 {
		t.Fatal("wire create payload lost variants or offers")
	}
}

package gateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

func merchantInvalidMultiFieldWire(t *testing.T, brokenEnum bool) string {
	t.Helper()
	w := emptyCatalogWireProposal()
	w.Operation = merchantcatalogai.OperationCreate
	w.Update.ItemID = ""
	w.ResponseText = "أعددت اقتراحًا للمراجعة"
	w.Create.Fields = []catalogWireField{
		{Name: "name", ValueJSON: `"secret-sample-name"`},
		{Name: "item_type", ValueJSON: `"physical_good"`},
		{Name: "pricing_mode", ValueJSON: `""`},
		{Name: "availability_mode", ValueJSON: `""`},
		{Name: "fulfillment_mode", ValueJSON: `""`},
	}
	w.Create.Offers = []catalogWireEntity{{Fields: []catalogWireField{
		{Name: "name", ValueJSON: `"سعر البيع"`},
		{Name: "name_source", ValueJSON: `"system_default"`},
		{Name: "price_source", ValueJSON: `"merchant_stated"`},
		{Name: "pricing_mode", ValueJSON: `""`},
		{Name: "amount", ValueJSON: `"500.0000"`},
		{Name: "currency", ValueJSON: `"YER"`},
		{Name: "availability_mode", ValueJSON: `""`},
		{Name: "availability_status", ValueJSON: `""`},
		{Name: "fulfillment_mode", ValueJSON: `""`},
		{Name: "status", ValueJSON: `""`},
	}}}
	if brokenEnum {
		w.AttributeDefinitions = []catalogWireAttributeDefinition{{
			Key: "color", Label: "secret-merchant-color-label", DataType: "select", ValidationRulesJSON: `{}`,
		}}
	}
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMerchantWireFeedbackEnumeratesAllProblemsWithoutLeakingMerchantValues(t *testing.T) {
	raw := merchantInvalidMultiFieldWire(t, true)
	correction := merchantCatalogWireFeedback(raw)
	for _, field := range []string{
		"create.pricing_mode", "create.availability_mode", "create.fulfillment_mode",
		"create.offers[0].pricing_mode", "create.offers[0].availability_mode",
		"create.offers[0].availability_status", "create.offers[0].fulfillment_mode",
		"create.offers[0].status", "attribute_definitions[0].color", "ask_merchant",
	} {
		if !strings.Contains(correction, field) {
			t.Errorf("correction missing %q", field)
		}
	}
	for _, secret := range []string{"secret-sample-name", "secret-merchant-color-label"} {
		if strings.Contains(correction, secret) {
			t.Errorf("merchant value leaked: %q", secret)
		}
	}
	if !strings.Contains(correction, "not available") {
		t.Fatal("unknown availability semantics lost")
	}
}

func TestMerchantWireFeedbackDoesNotFlagValidEnums(t *testing.T) {
	good := offerAvailabilityWire(t, "unknown")
	correction := merchantCatalogWireFeedback(good)
	if strings.Contains(correction, "create.pricing_mode") ||
		strings.Contains(correction, "create.offers[0].availability_mode") {
		t.Errorf("valid offer was called invalid: %s", correction)
	}
}

func TestMerchantParserAndCanonicalErrorsGiveModelAllFieldFeedback(t *testing.T) {
	for _, tc := range []struct {
		name          string
		parserError   bool
		secondInvalid bool
	}{
		{name: "invalid_select_rules_then_ask", parserError: true},
		{name: "invalid_required_fields_then_ask"},
		{name: "repeated_invalid_still_fails_closed", parserError: true, secondInvalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := merchantInvalidMultiFieldWire(t, tc.parserError)
			rounds := 0
			provider := &nativeContractProvider{name: "gemini"}
			provider.call = func(req native.Request) (native.Response, error) {
				rounds++
				var request struct {
					Messages []struct {
						Content string `json:"content"`
					} `json:"messages"`
					ResponseFormat map[string]any   `json:"response_format"`
					Tools          []map[string]any `json:"tools"`
				}
				if err := json.Unmarshal(req.Payload, &request); err != nil {
					return native.Response{}, err
				}
				if request.ResponseFormat["type"] != "json_schema" || len(request.Tools) != 0 {
					t.Error("strict final output schema lost")
				}
				if rounds == 2 {
					if len(request.Messages) < 1 || !strings.Contains(request.Messages[0].Content, "create.pricing_mode") ||
						!strings.Contains(request.Messages[0].Content, "create.offers[0].availability_mode") {
						t.Error("model did not receive all field corrections")
					}
					if tc.parserError && !strings.Contains(request.Messages[0].Content, "attribute_definitions[0].color") {
						t.Error("model did not receive missing color enum diagnostic")
					}
				}
				answer := bad
				if rounds > 1 && !tc.secondInvalid {
					answer = marshalMerchantWireTestAnswer(`{"schema_version":3,"status":"needs_more_data","operation":"ask_merchant","response_text":"ما السعر والتوفر والخيارات؟"}`)
				}
				payload, _ := json.Marshal(map[string]any{
					"choices": []any{map[string]any{
						"finish_reason": "stop",
						"message":       map[string]any{"role": "assistant", "content": answer},
					}},
					"usage": map[string]any{"prompt_tokens": 60, "completion_tokens": 20},
				})
				return native.Response{Payload: payload, Model: "gemini", InputTokens: 60, OutputTokens: 20}, nil
			}
			adapter, err := NewNativeMerchantCatalogAuthoringAdapter(nativeContractClient(t, provider), "merchant", 2048, 300000)
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{
				BusinessID: "business-1", PrincipalID: "owner-1", SessionID: "session-1",
				Message: "أضف هاتف", DefaultCurrency: "YER",
			})
			if rounds != 2 {
				t.Fatalf("expected bounded one retry, got %d", rounds)
			}
			if tc.secondInvalid {
				if err == nil {
					t.Fatalf("untrusted proposal accepted: %+v", proposal)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if proposal.Operation != merchantcatalogai.OperationAskMerchant || proposal.Create != nil ||
					len(proposal.AttributeDefinitions) != 0 || proposal.Status != merchantcatalogai.StatusNeedsMoreData {
					t.Fatalf("expected safe ask_merchant: %+v", proposal)
				}
			}
		})
	}
}

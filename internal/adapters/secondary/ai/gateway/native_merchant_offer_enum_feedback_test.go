package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

func offerAvailabilityWire(t *testing.T, mode string) string {
	t.Helper()
	wire := emptyCatalogWireProposal()
	wire.Operation = merchantcatalogai.OperationCreate
	wire.Status = merchantcatalogai.StatusResolved
	wire.ResponseText = "اقتراح إضافة المنتج للمراجعة"
	wire.Update.ItemID = ""
	wire.Create.Fields = []catalogWireField{
		{Name: "name", ValueJSON: `"جلكسي اس 8"`},
		{Name: "item_type", ValueJSON: `"physical_good"`},
		{Name: "pricing_mode", ValueJSON: `"fixed"`},
		{Name: "availability_mode", ValueJSON: `"unknown"`},
		{Name: "fulfillment_mode", ValueJSON: `"manual"`},
		{Name: "requires_confirmation", ValueJSON: `true`},
	}
	availability, err := json.Marshal(mode)
	if err != nil {
		t.Fatal(err)
	}
	wire.Create.Offers = []catalogWireEntity{{Fields: []catalogWireField{
		{Name: "name", ValueJSON: `"سعر البيع"`},
		{Name: "name_source", ValueJSON: `"system_default"`},
		{Name: "pricing_mode", ValueJSON: `"fixed"`},
		{Name: "amount", ValueJSON: `"500.0000"`},
		{Name: "price_source", ValueJSON: `"merchant_stated"`},
		{Name: "currency", ValueJSON: `"YER"`},
		{Name: "availability_mode", ValueJSON: string(availability)},
		{Name: "availability_status", ValueJSON: `"unknown"`},
		{Name: "fulfillment_mode", ValueJSON: `"manual"`},
		{Name: "status", ValueJSON: `"draft"`},
	}}}
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestMerchantOfferAvailabilityValidationRemainsStrict(t *testing.T) {
	p, err := catalogWireParse(offerAvailabilityWire(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err == nil ||
		!strings.Contains(err.Error(), `offer.availability_mode has unsupported value ""`) {
		t.Fatalf("empty availability unexpectedly passed: %v", err)
	}
	p, err = catalogWireParse(offerAvailabilityWire(t, "unknown"))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("contractual unconfirmed availability should be accepted: %v", err)
	}
}

func TestMerchantCanonicalEnumRetryProvidesExactSafeFieldCorrection(t *testing.T) {
	for _, tc := range []struct {
		name        string
		second      string
		wantFailure bool
	}{
		{name: "valid_unknown_after_feedback", second: "unknown"},
		{name: "still_empty_fails_closed", second: "", wantFailure: true},
		{name: "ask_for_missing_details", second: "ask"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rounds := 0
			provider := &nativeContractProvider{name: "gemini"}
			provider.call = func(req native.Request) (native.Response, error) {
				rounds++
				var body struct {
					Messages []struct {
						Content string `json:"content"`
					} `json:"messages"`
					ResponseFormat map[string]any `json:"response_format"`
					Tools          []map[string]any `json:"tools"`
				}
				if err := json.Unmarshal(req.Payload, &body); err != nil {
					return native.Response{}, err
				}
				if len(body.Tools) != 0 || body.ResponseFormat["type"] != "json_schema" {
					t.Error("strict final contract was not retained")
				}
				if rounds == 2 && (len(body.Messages) == 0 ||
					!strings.Contains(body.Messages[0].Content, "Go rejected offer.availability_mode") ||
					!strings.Contains(body.Messages[0].Content, "stock") ||
					!strings.Contains(body.Messages[0].Content, "unknown") ||
					!strings.Contains(body.Messages[0].Content, "not an empty string")) {
					t.Error("model did not receive exact, contract-backed field feedback")
				}
				var answer string
				switch {
				case rounds == 1:
					answer = offerAvailabilityWire(t, "")
				case tc.second == "ask":
					answer = marshalMerchantWireTestAnswer(`{"schema_version":3,"status":"needs_more_data","operation":"ask_merchant","response_text":"كيف تحدد طريقة التوفر؟"}`)
				default:
					answer = offerAvailabilityWire(t, tc.second)
				}
				raw, _ := json.Marshal(map[string]any{
					"choices": []any{map[string]any{
						"finish_reason": "stop",
						"message":       map[string]any{"role": "assistant", "content": answer},
					}},
					"usage": map[string]any{"prompt_tokens": 60, "completion_tokens": 20},
				})
				return native.Response{Payload: raw, Model: "gemini", InputTokens: 60, OutputTokens: 20}, nil
			}
			adapter, err := NewNativeMerchantCatalogAuthoringAdapter(nativeContractClient(t, provider), "merchant", 2048, 300000)
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{
				BusinessID: "business-1", PrincipalID: "owner-1", SessionID: "session-1",
				Message: "أضف جلكسي اس 8 بسعر 500 ريال", DefaultCurrency: "YER",
			})
			if rounds != 2 {
				t.Fatalf("expected one retry, got rounds=%d", rounds)
			}
			if tc.wantFailure {
				if err == nil {
					t.Fatalf("unconfirmed malformed offer accepted: %+v", proposal)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.second == "ask" {
				if proposal.Operation != merchantcatalogai.OperationAskMerchant ||
					proposal.Status != merchantcatalogai.StatusNeedsMoreData || proposal.Create != nil {
					t.Fatalf("merchant clarification must not create offer: %+v", proposal)
				}
			} else if proposal.Create == nil || len(proposal.Create.Offers) != 1 ||
				proposal.Create.Offers[0].AvailabilityMode != "unknown" ||
				proposal.Create.Offers[0].AvailabilityStatus != "unknown" {
				t.Fatalf("unknown availability was transformed or dropped: %+v", proposal)
			}
		})
	}
}

func TestMerchantEnumFeedbackNeverEchoesUntrustedValidatorValues(t *testing.T) {
	sensitive := "private-merchant-secret"
	feedback := merchantCatalogValidationFeedback(errors.New(`offer.availability_mode has unsupported value "` + sensitive + `"`))
	if strings.Contains(feedback, sensitive) || !strings.Contains(feedback, "offer.availability_mode") {
		t.Fatalf("bad validation feedback: %s", feedback)
	}
	if strings.Contains(merchantCatalogValidationFeedback(errors.New(sensitive)), sensitive) {
		t.Fatal("untrusted validator error was inserted into the system prompt")
	}
}

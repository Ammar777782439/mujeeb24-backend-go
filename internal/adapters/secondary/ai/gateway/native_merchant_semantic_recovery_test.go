package gateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

func merchantMalformedAskWire(t *testing.T, invalidRules bool) string {
	t.Helper()
	wire := emptyCatalogWireProposal()
	wire.Status = merchantcatalogai.StatusNeedsMoreData
	wire.Operation = merchantcatalogai.OperationAskMerchant
	wire.Update.ItemID = ""
	wire.ResponseText = "كم سعر الجهاز؟"
	wire.AttributeSchemaName = "مواصفات هواتف سامسونج"
	rules := "{}"
	if invalidRules {
		rules = ""
	}
	wire.AttributeDefinitions = []catalogWireAttributeDefinition{{
		Key: "brand", Label: "العلامة التجارية", DataType: "text", ValidationRulesJSON: rules,
	}}
	wire.Create.Fields = []catalogWireField{
		{Name: "name", ValueJSON: `"جلكسي اس 8"`},
		{Name: "attributes", ValueJSON: `{"brand":"Samsung"}`},
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestMerchantInvalidAskWireMustNotBeSilentlyAccepted(t *testing.T) {
	for _, invalidRules := range []bool{true, false} {
		raw := merchantMalformedAskWire(t, invalidRules)
		if _, err := catalogWireParse(raw); err == nil {
			t.Fatalf("invalid ask_merchant wire accepted, invalidRules=%v", invalidRules)
		}
	}
}

func TestMerchantSemanticWireRetryIsBoundedAndPreservesStrictContract(t *testing.T) {
	for _, tc := range []struct {
		name         string
		invalidRules bool
		recover      bool
	}{
		{name: "empty_validation_rules_recovered", invalidRules: true, recover: true},
		{name: "inactive_draft_attributes_recovered", invalidRules: false, recover: true},
		{name: "two_invalid_semantic_wires_fail_closed", invalidRules: true, recover: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			bad := merchantMalformedAskWire(t, tc.invalidRules)
			provider := &nativeContractProvider{name: "gemini"}
			provider.call = func(req native.Request) (native.Response, error) {
				calls++
				var request struct {
					Messages []struct {
						Content string `json:"content"`
					} `json:"messages"`
					Tools          []map[string]any `json:"tools"`
					ResponseFormat map[string]any   `json:"response_format"`
				}
				if err := json.Unmarshal(req.Payload, &request); err != nil {
					return native.Response{}, err
				}
				if len(request.Tools) != 0 || request.ResponseFormat["type"] != "json_schema" {
					t.Error("merchant recovery lost strict final schema")
				}
				if calls == 2 && (len(request.Messages) == 0 ||
					!strings.Contains(request.Messages[0].Content, "Final proposal semantic recovery") ||
					!strings.Contains(request.Messages[0].Content, "validation_rules_json")) {
					t.Error("retry did not preserve strict repair instructions")
				}
				wire := bad
				if calls == 2 && tc.recover {
					valid := emptyCatalogWireProposal()
					valid.Update.ItemID = ""
					valid.Status = merchantcatalogai.StatusNeedsMoreData
					valid.Operation = merchantcatalogai.OperationAskMerchant
					valid.ResponseText = "كم سعر الجهاز؟"
					encoded, err := json.Marshal(valid)
					if err != nil {
						return native.Response{}, err
					}
					wire = string(encoded)
				}
				encoded, err := json.Marshal(map[string]any{
					"choices": []any{map[string]any{
						"finish_reason": "stop",
						"message":       map[string]any{"role": "assistant", "content": wire},
					}},
					"usage": map[string]any{"prompt_tokens": 100, "completion_tokens": 25},
				})
				if err != nil {
					return native.Response{}, err
				}
				return native.Response{
					Payload: encoded, Model: "gemini", InputTokens: 100, OutputTokens: 25,
				}, nil
			}
			adapter, err := NewNativeMerchantCatalogAuthoringAdapter(
				nativeContractClient(t, provider), "merchant", 2048, 300000,
			)
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{
				BusinessID: "business-1", PrincipalID: "owner-1", SessionID: "session-1",
				Message: "أضف جلكسي اس 8", DefaultCurrency: "YER",
			})
			if calls != 2 {
				t.Fatalf("expected exactly two inference attempts; got %d", calls)
			}
			if !tc.recover {
				if err == nil {
					t.Fatalf("second invalid wire was accepted: %+v", proposal)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if proposal.Status != merchantcatalogai.StatusNeedsMoreData ||
				proposal.Operation != merchantcatalogai.OperationAskMerchant ||
				proposal.Create != nil || len(proposal.AttributeDefinitions) != 0 ||
				proposal.AttributeSchemaName != "" {
				t.Fatalf("malformed attribute/template data was carried into the final proposal: %+v", proposal)
			}
		})
	}
}

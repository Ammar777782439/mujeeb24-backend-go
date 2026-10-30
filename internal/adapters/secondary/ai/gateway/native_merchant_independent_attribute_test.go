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

type independentDefinitionCapabilities struct{ checks int }

func (c *independentDefinitionCapabilities) Definitions() []merchantcatalogai.MerchantCatalogDiscoveryToolDefinition {
	return nil
}
func (c *independentDefinitionCapabilities) Execute(
	context.Context, merchantcatalogai.MerchantCatalogDiscoveryExecutionContext, string, []byte,
) (merchantcatalogai.MerchantCatalogDiscoveryResult, error) {
	return merchantcatalogai.MerchantCatalogDiscoveryResult{}, errors.New("unexpected discovery")
}
func (c *independentDefinitionCapabilities) ValidateProposalSchemaAttributes(
	_ context.Context, _ string, p merchantcatalogai.Proposal,
) error {
	c.checks++
	if len(p.AttributeDefinitions) != 1 {
		return errors.New("expected exactly one custom definition")
	}
	if p.AttributeDefinitions[0].Label == "المنطقة المستهدفة" {
		return &merchantcatalogai.SchemaDefinitionPreflightError{
			Index: 0, Reason: "existing_rules_conflict",
			ExpectedType: "multi_select", ProposedType: "multi_select",
		}
	}
	if p.AttributeDefinitions[0].Label != "الأعضاء المستهدفة" {
		return errors.New("custom definition was not disambiguated")
	}
	return nil
}

// The model gets one bounded retry and may propose a separate, clearly named
// field instead of being forced to abandon an unrelated merchant value.
func TestMerchantRuleConflictFeedbackCanPreserveIndependentCustomAttribute(t *testing.T) {
	caps := &independentDefinitionCapabilities{}
	rounds := 0
	provider := &nativeContractProvider{name: "gemini"}
	provider.call = func(req native.Request) (native.Response, error) {
		rounds++
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat map[string]any   `json:"response_format"`
			Tools          []map[string]any `json:"tools"`
		}
		if err := json.Unmarshal(req.Payload, &body); err != nil {
			return native.Response{}, err
		}
		if len(body.Tools) != 0 || body.ResponseFormat["type"] != "json_schema" {
			t.Error("strict merchant wire was removed")
		}
		if rounds == 2 {
			instructions := body.Messages[0].Content
			for _, expected := range []string{
				"existing_rules_conflict", "DIFFERENT concept",
				"precise Arabic label", "fresh snake_case alias",
			} {
				if !strings.Contains(instructions, expected) {
					t.Errorf("missing repair guidance %q", expected)
				}
			}
		}
		var wire catalogWireProposal
		if err := json.Unmarshal([]byte(offerAvailabilityWire(t, "unknown")), &wire); err != nil {
			return native.Response{}, err
		}
		label := "المنطقة المستهدفة"
		if rounds == 2 {
			label = "الأعضاء المستهدفة"
		}
		wire.AttributeDefinitions = []catalogWireAttributeDefinition{{
			Key: "target_anatomy", Label: label, DataType: "multi_select",
			ValidationRulesJSON: `{"enum":["الأسنان","اللثة","الفم"]}`,
		}}
		wire.Create.Fields = append(wire.Create.Fields, catalogWireField{
			Name:      "attributes",
			ValueJSON: `{"target_anatomy":["الأسنان","اللثة","الفم"]}`,
		})
		data, err := json.Marshal(wire)
		if err != nil {
			return native.Response{}, err
		}
		payload, err := json.Marshal(map[string]any{
			"choices": []any{map[string]any{
				"finish_reason": "stop",
				"message":       map[string]any{"role": "assistant", "content": string(data)},
			}},
			"usage": map[string]any{"prompt_tokens": 80, "completion_tokens": 30},
		})
		return native.Response{Payload: payload, Model: "gemini", InputTokens: 80, OutputTokens: 30}, err
	}
	adapter, err := NewNativeMerchantCatalogAuthoringAdapter(
		nativeContractClient(t, provider), "merchant", 2048, 300000,
	)
	if err != nil {
		t.Fatal(err)
	}
	p, err := adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{
		BusinessID: "business-1", PrincipalID: "merchant-1", SessionID: "session-1",
		Message: "أضف مواصفة مستقلة بدون تغيير القالب", DefaultCurrency: "YER",
		Capabilities: caps,
	})
	if err != nil {
		t.Fatalf("independent definition retry should succeed: %v", err)
	}
	if rounds != 2 || caps.checks != 2 || len(p.AttributeDefinitions) != 1 ||
		p.AttributeDefinitions[0].Label != "الأعضاء المستهدفة" ||
		p.Create == nil || p.Create.Attributes["target_anatomy"] == nil {
		t.Fatalf("merchant values or distinct definition were dropped: %+v, rounds=%d checks=%d", p, rounds, caps.checks)
	}
}

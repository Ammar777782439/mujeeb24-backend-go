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

type merchantSchemaPreflightStub struct{ calls int }

func (s *merchantSchemaPreflightStub) Definitions() []merchantcatalogai.MerchantCatalogDiscoveryToolDefinition {
	return nil
}
func (s *merchantSchemaPreflightStub) Execute(context.Context, merchantcatalogai.MerchantCatalogDiscoveryExecutionContext, string, []byte) (merchantcatalogai.MerchantCatalogDiscoveryResult, error) {
	return merchantcatalogai.MerchantCatalogDiscoveryResult{}, errors.New("discovery disabled in this fixture")
}
func (s *merchantSchemaPreflightStub) ValidateProposalSchemaAttributes(_ context.Context, business string, p merchantcatalogai.Proposal) error {
	s.calls++
	if business != "business-1" {
		return errors.New("tenant scope mismatch")
	}
	if p.Create != nil && p.Create.Attributes["attr_brand"] == "Samsung" {
		return nil
	}
	return &merchantcatalogai.SchemaAttributePreflightError{
		Path: "create.attributes.attr_brand", DataType: "text", Required: true,
	}
}

func TestMerchantSchemaPreflightRepairsMissingRequiredAttributeBeforeReview(t *testing.T) {
	caps := &merchantSchemaPreflightStub{}
	rounds := 0
	provider := &nativeContractProvider{name: "gemini"}
	provider.call = func(req native.Request) (native.Response, error) {
		rounds++
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat map[string]any   `json:"response_format"`
		}
		if err := json.Unmarshal(req.Payload, &body); err != nil {
			return native.Response{}, err
		}
		if body.ResponseFormat["type"] != "json_schema" {
			t.Error("strict proposal contract was dropped")
		}
		if rounds == 2 && (len(body.Messages) == 0 ||
			!strings.Contains(body.Messages[0].Content, "Existing catalog schema preflight correction") ||
			!strings.Contains(body.Messages[0].Content, "create.attributes.attr_brand")) {
			t.Error("model did not receive schema field repair guidance")
		}
		var wire catalogWireProposal
		if err := json.Unmarshal([]byte(offerAvailabilityWire(t, "unknown")), &wire); err != nil {
			t.Fatal(err)
		}
		if rounds == 2 {
			wire.Create.Fields = append(wire.Create.Fields, catalogWireField{
				Name: "attributes", ValueJSON: `{"attr_brand":"Samsung"}`,
			})
		}
		rawWire, err := json.Marshal(wire)
		if err != nil {
			return native.Response{}, err
		}
		raw, err := json.Marshal(map[string]any{
			"choices": []any{map[string]any{
				"finish_reason": "stop",
				"message":       map[string]any{"role": "assistant", "content": string(rawWire)},
			}},
			"usage": map[string]any{"prompt_tokens": 50, "completion_tokens": 20},
		})
		return native.Response{Payload: raw, Model: "gemini", InputTokens: 50, OutputTokens: 20}, err
	}
	adapter, err := NewNativeMerchantCatalogAuthoringAdapter(nativeContractClient(t, provider), "merchant", 2048, 300000)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{
		BusinessID: "business-1", PrincipalID: "owner-1", SessionID: "session-1",
		Message: "أضف هاتفًا بسعر 500 ريال", DefaultCurrency: "YER",
		Capabilities: caps,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rounds != 2 || caps.calls != 2 || proposal.Create == nil ||
		proposal.Create.Attributes["attr_brand"] != "Samsung" {
		t.Fatalf("schema preflight correction not applied: rounds=%d calls=%d result=%+v", rounds, caps.calls, proposal)
	}
}

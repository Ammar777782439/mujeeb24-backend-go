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

type definitionConflictCapabilities struct{ checks int }

func (c *definitionConflictCapabilities) Definitions() []merchantcatalogai.MerchantCatalogDiscoveryToolDefinition {
	return nil
}
func (c *definitionConflictCapabilities) Execute(
	context.Context, merchantcatalogai.MerchantCatalogDiscoveryExecutionContext, string, []byte,
) (merchantcatalogai.MerchantCatalogDiscoveryResult, error) {
	return merchantcatalogai.MerchantCatalogDiscoveryResult{}, errors.New("unexpected discovery")
}
func (c *definitionConflictCapabilities) ValidateProposalSchemaAttributes(
	_ context.Context, businessID string, proposal merchantcatalogai.Proposal,
) error {
	c.checks++
	if businessID != "business-1" {
		return errors.New("invalid tenant scope")
	}
	if proposal.Operation == merchantcatalogai.OperationAskMerchant {
		return nil
	}
	return &merchantcatalogai.SchemaDefinitionPreflightError{
		Index: 0, Reason: "existing_type_conflict",
		ExpectedType: "multi_select", ProposedType: "select",
	}
}

func TestMerchantDefinitionConflictIsRepairedBeforeApprovalOrFailsClosed(t *testing.T) {
	for _, test := range []struct {
		name   string
		repair bool
	}{
		{name: "corrected_to_ask", repair: true},
		{name: "second_conflict_rejected"},
	} {
		t.Run(test.name, func(t *testing.T) {
			caps := &definitionConflictCapabilities{}
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
					t.Error("strict final transport was removed")
				}
				if rounds == 2 && (len(body.Messages) == 0 ||
					!strings.Contains(body.Messages[0].Content, "existing_type_conflict") ||
					!strings.Contains(body.Messages[0].Content, "multi_select") ||
					!strings.Contains(body.Messages[0].Content, "attribute_definitions[0]") ||
					!strings.Contains(body.Messages[0].Content, "ask_merchant")) {
					t.Error("missing exact definition conflict feedback")
				}
				answer := offerAvailabilityWire(t, "unknown")
				if rounds == 2 && test.repair {
					answer = marshalMerchantWireTestAnswer(`{"schema_version":3,"status":"needs_more_data","operation":"ask_merchant","response_text":"اللون غير موجود في القالب؛ هل ترغب في تحديث خياراته؟"}`)
				}
				data, err := json.Marshal(map[string]any{
					"choices": []any{map[string]any{
						"finish_reason": "stop",
						"message":       map[string]any{"role": "assistant", "content": answer},
					}},
					"usage":   map[string]any{"prompt_tokens": 60, "completion_tokens": 20},
				})
				return native.Response{Payload: data, Model: "gemini", InputTokens: 60, OutputTokens: 20}, err
			}
			adapter, err := NewNativeMerchantCatalogAuthoringAdapter(
				nativeContractClient(t, provider), "merchant", 2048, 300000,
			)
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{
				BusinessID: "business-1", PrincipalID: "owner-1", SessionID: "session-1",
				Message: "أضف اللون الأصفر للمنتج", DefaultCurrency: "YER",
				Capabilities: caps,
			})
			if rounds != 2 {
				t.Fatalf("must attempt exactly one correction, got %d", rounds)
			}
			if !test.repair {
				if err == nil || caps.checks != 2 {
					t.Fatalf("second invalid schema should fail closed: %+v, err=%v", proposal, err)
				}
				return
			}
			if err != nil || proposal.Operation != merchantcatalogai.OperationAskMerchant ||
				proposal.Create != nil || proposal.Update != nil ||
				proposal.Status != merchantcatalogai.StatusNeedsMoreData {
				t.Fatalf("invalid schema was not converted to safe merchant question: %+v, err=%v", proposal, err)
			}
		})
	}
}

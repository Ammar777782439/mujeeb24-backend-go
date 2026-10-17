package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

// These tests use only local in-process providers. They validate the shared Go
// business boundary and do not imply real Gemini or DeepSeek qualification.
type nativeContractProvider struct {
	name string
	call func(native.Request) (native.Response, error)
}

func (p *nativeContractProvider) Name() string { return p.name }
func (p *nativeContractProvider) Generate(_ context.Context, req native.Request) (native.Response, error) {
	return p.call(req)
}
func nativeContractClient(t *testing.T, provider *nativeContractProvider) *native.CompletionClient {
	t.Helper()
	standby := &nativeContractProvider{name: "deepseek", call: func(native.Request) (native.Response, error) {
		return native.Response{}, errors.New("unexpected standby invocation")
	}}
	r, err := native.NewRouter(provider, standby, native.Policy{ConsecutiveFailures: 3, Cooldown: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	client, err := native.NewCompletionClient(r)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestNativeCatalogBatchPreservesCommercialEvidenceAndUsage(t *testing.T) {
	const itemID = "11111111-1111-4111-8111-111111111111"
	provider := &nativeContractProvider{name: "gemini"}
	provider.call = func(req native.Request) (native.Response, error) {
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat struct {
				Type       string `json:"type"`
				JSONSchema struct {
					Strict bool           `json:"strict"`
					Schema map[string]any `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.Unmarshal(req.Payload, &body); err != nil {
			t.Error(err)
		}
		if body.Model != "mujeeb-catalog" || len(body.Messages) != 2 ||
			body.Messages[0].Role != "system" || body.Messages[1].Role != "user" ||
			!strings.Contains(body.Messages[1].Content, "attribute_schemas") ||
			!strings.Contains(body.Messages[1].Content, "256GB") ||
			!strings.Contains(body.Messages[1].Content, "offers") ||
			body.ResponseFormat.Type != "json_schema" || !body.ResponseFormat.JSONSchema.Strict {
			t.Error("canonical item evidence or strict response contract was lost")
		}
		result := map[string]any{
			"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{
				"role": "assistant", "content": `{"candidates":[{"item_id":"11111111-1111-4111-8111-111111111111","variant_ids":["variant-1"],"offer_ids":["offer-1"],"reason":"model requested"}]}`,
			}}},
			"usage": map[string]any{"prompt_tokens": 220, "completion_tokens": 23},
		}
		raw, _ := json.Marshal(result)
		return native.Response{Payload: raw, Model: "gemini-2.5-flash", InputTokens: 220, OutputTokens: 23}, nil
	}
	adapter, err := NewNativeCatalogAdapter(nativeContractClient(t, provider), "mujeeb-catalog", 4096, 65536)
	if err != nil {
		t.Fatal(err)
	}
	schemaID := "schema-1"
	input := services.BatchEvaluationInput{BatchNumber: 1, CustomerMessage: "أريد هاتف 256",
		Batch: services.CatalogAIBatchPayload{BatchNumber: 1,
			AttributeSchemas: []services.CatalogAIAttributeSchema{{
				ID: schemaID, Name: "مواصفات الهواتف", Version: 1,
				Definitions: []services.CatalogAIAttributeDefinition{{
					ID: "definition-1", SchemaID: schemaID, AttributeKey: "capacity", Label: "الذاكرة", DataType: "select",
				}},
			}},
			Items: []services.CatalogAIItem{{
				ID: itemID, CatalogID: "catalog-1", Name: "هاتف", AttributeSchemaID: &schemaID,
				Attributes: map[string]any{"capacity": "256GB"},
				Variants:   []services.CatalogAIVariant{{ID: "variant-1", CatalogItemID: itemID}},
				Offers:     []services.CatalogAIOffer{{ID: "offer-1", CatalogItemID: itemID}},
			}},
		},
	}
	n, err := adapter.CountBatchTokens(context.Background(), input)
	if err != nil || n <= 1024 {
		t.Fatalf("unbounded catalog batch %d: %v", n, err)
	}
	got, err := adapter.EvaluateBatch(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if got.BatchNumber != 1 || len(got.Candidates) != 1 ||
		got.Candidates[0].ItemID != itemID ||
		len(got.Candidates[0].VariantIDs) != 1 || got.Candidates[0].VariantIDs[0] != "variant-1" ||
		len(got.Candidates[0].OfferIDs) != 1 || got.Candidates[0].OfferIDs[0] != "offer-1" ||
		got.Usage.Provider != "gemini" || got.Usage.Model != "gemini-2.5-flash" ||
		got.Usage.OutputTokens != 23 || got.Usage.ModelRequests != 1 {
		t.Fatalf("batch output or attribution lost: %+v", got)
	}
}

type nativeMerchantReadStub struct {
	calls                   int
	businessID, principalID string
}

func (s *nativeMerchantReadStub) Definitions() []merchantcatalogai.MerchantCatalogDiscoveryToolDefinition {
	return []merchantcatalogai.MerchantCatalogDiscoveryToolDefinition{{
		Name: "read_catalog", Description: "Read scoped catalog evidence",
		Parameters: map[string]any{
			"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}},
		},
	}}
}
func (s *nativeMerchantReadStub) Execute(_ context.Context, scope merchantcatalogai.MerchantCatalogDiscoveryExecutionContext, name string, args []byte) (merchantcatalogai.MerchantCatalogDiscoveryResult, error) {
	s.calls++
	s.businessID = scope.BusinessID
	s.principalID = scope.PrincipalID
	if name != "read_catalog" || !json.Valid(args) {
		return merchantcatalogai.MerchantCatalogDiscoveryResult{}, errors.New("unexpected tool")
	}
	return merchantcatalogai.MerchantCatalogDiscoveryResult{Data: map[string]any{"catalog_item": "Coffee", "currency": "YER"}}, nil
}

func TestNativeMerchantReadOnlyToolAndCanonicalFinalValidation(t *testing.T) {
	caps := &nativeMerchantReadStub{}
	round := 0
	provider := &nativeContractProvider{name: "gemini"}
	provider.call = func(req native.Request) (native.Response, error) {
		round++
		var body struct {
			Messages []struct {
				Role       string          `json:"role"`
				Content    json.RawMessage `json:"content"`
				ToolCallID string          `json:"tool_call_id"`
			} `json:"messages"`
			Tools             []map[string]any `json:"tools"`
			ResponseFormat    map[string]any   `json:"response_format"`
			ParallelToolCalls *bool            `json:"parallel_tool_calls"`
		}
		if err := json.Unmarshal(req.Payload, &body); err != nil {
			t.Fatal(err)
		}
		if body.ParallelToolCalls == nil || *body.ParallelToolCalls {
			t.Error("merchant allowed parallel model tool calls")
		}
		if len(body.Messages) < 2 || body.Messages[0].Role != "system" || body.Messages[1].Role != "user" {
			t.Error("merchant trusted and untrusted messages merged")
		}
		if round < 3 && (len(body.Tools) != 1 || body.ResponseFormat != nil) {
			t.Error("discovery tools must not silently combine with structured final output")
		}
		if round == 3 {
			if len(body.Tools) != 0 || body.ResponseFormat["type"] != "json_schema" ||
				!strings.Contains(string(body.Messages[0].Content), "Canonical merchant proposal JSON schema") {
				t.Error("strict final proposal transport is not isolated from discovery tools")
			}
		}
		if round >= 2 && (len(body.Messages) != 4 || body.Messages[2].Role != "assistant" ||
			body.Messages[3].Role != "tool" || body.Messages[3].ToolCallID != "call-1" ||
			!strings.Contains(string(body.Messages[3].Content), "Coffee")) {
			t.Error("read-only discovery result not replayed to model")
		}
		var choice map[string]any
		switch round {
		case 1:
			choice = map[string]any{"finish_reason": "tool_calls", "message": map[string]any{
				"role": "assistant", "content": nil,
				"tool_calls": []any{map[string]any{"id": "call-1", "type": "function",
					"function": map[string]any{"name": "read_catalog", "arguments": `{"query":"coffee"}`}}},
			}}
		case 2:
			// Read-only discovery can stop, but its plain text is NEVER
			// accepted as a commercial proposal.
			choice = map[string]any{"finish_reason": "stop", "message": map[string]any{
				"role": "assistant", "content": "I have enough evidence",
			}}
		case 3:
			proposalJSON := `{"schema_version":3,"status":"needs_more_data","operation":"ask_merchant","response_text":"ما السعر؟"}`
			envelope, _ := json.Marshal(map[string]string{"canonical_proposal_json": proposalJSON})
			choice = map[string]any{"finish_reason": "stop", "message": map[string]any{
				"role": "assistant", "content": string(envelope),
			}}
		default:
			return native.Response{}, errors.New("merchant model called beyond expected final step")
		}
		raw, _ := json.Marshal(map[string]any{
			"choices": []any{choice},
			"usage":   map[string]any{"prompt_tokens": 90, "completion_tokens": 12},
		})
		return native.Response{Payload: raw, Model: "gemini-2.5-flash", InputTokens: 90, OutputTokens: 12}, nil
	}
	adapter, err := NewNativeMerchantCatalogAuthoringAdapter(nativeContractClient(t, provider), "mujeeb-merchant", 2048, 300000)
	if err != nil {
		t.Fatal(err)
	}
	rounds := 0
	adapter.SetUsageReporter(func(_ context.Context, businessID, sessionID string, usage ports.CustomerSalesUsageTelemetry) error {
		rounds++
		if businessID != "business-42" || sessionID != "session-3" || usage.Provider != "gemini" ||
			usage.InputTokens != 90 || usage.ModelRequests != 1 {
			t.Errorf("invalid usage attribution %+v", usage)
		}
		return nil
	})
	proposal, err := adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{
		BusinessID: "business-42", PrincipalID: "owner-17", SessionID: "session-3",
		Message: "أريد تعديل الكتالوج", DefaultCurrency: "YER",
		EntityContract: []byte(`{"required":"item"}`), Capabilities: caps,
	})
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Operation != merchantcatalogai.OperationAskMerchant || proposal.ResponseText != "ما السعر؟" ||
		caps.calls != 1 || caps.businessID != "business-42" || caps.principalID != "owner-17" ||
		round != 3 || rounds != 3 {
		t.Fatalf("merchant scoped reads/strict final/usage drift: %+v calls=%d rounds=%d usage=%d", proposal, caps.calls, round, rounds)
	}
}

func TestNativeMerchantRejectsInvalidStrictProposalEnvelope(t *testing.T) {
	provider := &nativeContractProvider{name: "gemini"}
	provider.call = func(req native.Request) (native.Response, error) {
		var body struct {
			ResponseFormat    map[string]any `json:"response_format"`
			ParallelToolCalls *bool          `json:"parallel_tool_calls"`
		}
		if err := json.Unmarshal(req.Payload, &body); err != nil || body.ResponseFormat["type"] != "json_schema" {
			t.Fatal("merchant provider called without strict final schema")
		}
		response := map[string]any{"choices": []any{map[string]any{
			"finish_reason": "stop", "message": map[string]any{
				"role": "assistant", "content": `{"canonical_proposal_json":"{\"schema_version\":3}","unexpected":"must reject"}`,
			},
		}}, "usage": map[string]any{"prompt_tokens": 4, "completion_tokens": 10}}
		raw, _ := json.Marshal(response)
		return native.Response{Payload: raw, Model: "gemini-2.5-flash", InputTokens: 4, OutputTokens: 10}, nil
	}
	adapter, err := NewNativeMerchantCatalogAuthoringAdapter(nativeContractClient(t, provider), "mujeeb-merchant", 2048, 300000)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Propose(context.Background(), merchantcatalogai.MerchantCatalogAuthoringInput{
		BusinessID: "business-42", PrincipalID: "owner-17", SessionID: "session-3",
		Message: "أريد تعديل الكتالوج", DefaultCurrency: "YER",
	})
	if err == nil {
		t.Fatal("invalid strict merchant envelope was accepted")
	}
}

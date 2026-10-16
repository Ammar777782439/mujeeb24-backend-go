package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/litellm"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

func TestLiteLLMCatalogBatchPreservesSchemaAndAllCommercialRelations(t *testing.T) {
	const itemID = "11111111-1111-4111-8111-111111111111"
	var sawCount, sawEvaluation bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing gateway authentication")
		}
		switch r.URL.Path {
		case "/utils/token_counter":
			sawCount = true
			_, _ = w.Write([]byte(`{"total_tokens":210}`))
		case "/v1/chat/completions":
			sawEvaluation = true
			var req chatGatewayTestRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
			}
			if req.ResponseFormat != nil && req.ResponseFormat.JSONSchema.Schema != nil {
				props := req.ResponseFormat.JSONSchema.Schema["properties"].(map[string]any)["candidates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
				for key, want := range map[string]string{"item_id": itemID, "variant_ids": "v1", "offer_ids": "o1"} {
					prop := props[key].(map[string]any)
					if key != "item_id" {
						prop = prop["items"].(map[string]any)
					}
					values, ok := prop["enum"].([]any)
					if !ok || len(values) != 1 || values[0] != want {
						t.Errorf("%s not restricted to batch: %#v", key, prop["enum"])
					}
				}
			}
			if req.Model != "mujeeb-catalog" ||
				!strings.Contains(req.UserContent(), "attribute_schemas") ||
				!strings.Contains(req.UserContent(), "attr_1") ||
				!strings.Contains(req.UserContent(), "256GB") ||
				!strings.Contains(req.UserContent(), "variants") ||
				!strings.Contains(req.UserContent(), "offers") ||
				!strings.Contains(req.SystemContent(), "Catalog Entity Contract") ||
				req.ResponseFormat == nil || req.ResponseFormat.JSONSchema.Schema == nil {
				t.Errorf("catalog request lost schema/relations/structured response: %+v", req)
			}
			result := map[string]any{"status": "completed", "outputs": []any{map[string]any{
				"type": "message", "content": []any{map[string]any{"type": "output_text", "text": `{"candidates":[{"item_id":"11111111-1111-4111-8111-111111111111","variant_ids":["v1"],"offer_ids":["o1"]}]}`}},
			}}, "usage": map[string]any{"total_input_tokens": 300, "total_output_tokens": 22}}
			writeChatTestResponse(w, result)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := litellm.NewClient(litellm.Config{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewLiteLLMCatalogAdapter(client, "mujeeb-catalog", 4096, 65536)
	if err != nil {
		t.Fatal(err)
	}
	schemaID := "schema1"
	batch := services.CatalogAIBatchPayload{BatchNumber: 1,
		AttributeSchemas: []services.CatalogAIAttributeSchema{{
			ID: schemaID, Name: "مواصفات الهواتف", Version: 1,
			Definitions: []services.CatalogAIAttributeDefinition{{ID: "def1", SchemaID: schemaID, AttributeKey: "attr_1", Label: "سعة التخزين", DataType: "select"}},
		}},
		Items: []services.CatalogAIItem{{
			ID: itemID, CatalogID: "cat1", Name: "هاتف", AttributeSchemaID: &schemaID, Attributes: map[string]any{"attr_1": "256GB"},
			Variants: []services.CatalogAIVariant{{ID: "v1", CatalogItemID: itemID}},
			Offers:   []services.CatalogAIOffer{{ID: "o1", CatalogItemID: itemID}},
		}},
	}
	in := services.BatchEvaluationInput{BatchNumber: 1, CustomerMessage: "أريد هاتف 256", Batch: batch}
	size, err := adapter.CountBatchTokens(context.Background(), in)
	if err != nil || size <= 210 {
		t.Fatalf("unsafe token count: %d %v", size, err)
	}
	out, err := adapter.EvaluateBatch(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if !sawCount || !sawEvaluation || len(out.Candidates) != 1 || out.Candidates[0].ItemID != itemID ||
		len(out.Candidates[0].VariantIDs) != 1 || len(out.Candidates[0].OfferIDs) != 1 ||
		out.Usage.Provider != "litellm" || out.Usage.OutputTokens != 22 {
		t.Fatalf("candidate/evidence or usage lost: %+v", out)
	}
}

func TestLiteLLMFinalCatalogRequiresTerminalCapability(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/utils/token_counter" {
			_, _ = w.Write([]byte(`{"total_tokens":70}`))
			return
		}
		var req chatGatewayTestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if len(req.Tools) == 0 || req.ToolChoice != "required" {
			t.Error("final catalog capabilities were not offered")
		}
		writeChatTestResponse(w, map[string]any{
			"status": "requires_action",
			"outputs": []any{map[string]any{"type": "function_call", "id": "call-1",
				"name": "respond_without_action", "arguments": map[string]any{"response_text": "متوفر لدينا الهاتف"}}},
			"usage": map[string]any{"total_input_tokens": 90, "total_output_tokens": 16},
		})
	}))
	defer server.Close()
	client, err := litellm.NewClient(litellm.Config{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewLiteLLMCatalogAdapter(client, "mujeeb-catalog", 4096, 65536)
	if err != nil {
		t.Fatal(err)
	}
	out, usage, err := adapter.FinalEvaluateWithDetails(context.Background(), services.FinalEvaluationInput{}, "customer and verified candidates")
	if err != nil {
		t.Fatal(err)
	}
	if out.Capability != ports.CustomerSalesCapabilityRespondWithoutAction || out.ResponseText != "متوفر لدينا الهاتف" || usage.Provider != "litellm" {
		t.Fatalf("final capability contract drift: %+v %+v", out, usage)
	}
}

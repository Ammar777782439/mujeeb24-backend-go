package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/litellm"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

func TestLiteLLMOverviewReadsWholeBatchAndProducesOneFinalCustomerReply(t *testing.T) {
	var counted, batchCalls, finalCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("gateway virtual key missing")
		}
		switch r.URL.Path {
		case "/utils/token_counter":
			counted++
			_, _ = w.Write([]byte(`{"total_tokens":320}`))
		case "/v1beta/interactions":
			var body struct {
				Model          string                     `json:"model"`
				Input          string                     `json:"input"`
				ResponseFormat *interactionResponseFormat `json:"response_format"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			if body.Model != "mujeeb-catalog" || body.ResponseFormat == nil {
				t.Error("expected catalog model and JSON Schema")
				return
			}
			var input struct {
				Stage     string `json:"stage"`
				Batch     any    `json:"catalog_batch"`
				Summaries any    `json:"reviewed_summaries"`
			}
			if err := json.Unmarshal([]byte(body.Input), &input); err != nil {
				t.Error(err)
				return
			}
			reply := ""
			switch input.Stage {
			case "batch":
				batchCalls++
				if !strings.Contains(body.Input, "250.0000") || !strings.Contains(body.Input, "SAR") || !strings.Contains(body.Input, "variant-1") {
					t.Errorf("overview batch missing original commercial evidence: %s", body.Input)
				}
				properties := body.ResponseFormat.Schema["properties"].(map[string]any)
				ids := properties["covered_item_ids"].(map[string]any)["items"].(map[string]any)["enum"].([]any)
				if len(ids) != 1 || ids[0] != "item-1" {
					t.Errorf("batch IDs not confined to current evidence: %v", ids)
				}
				reply = `{"text":"المتجر يعرض هاتفًا وخيارات تقنية بسعر 250 ريال سعودي.","covered_item_ids":["item-1"]}`
			case "final":
				finalCalls++
				if input.Batch != nil || !strings.Contains(body.Input, "reviewed_summaries") {
					t.Error("Gemini final synthesis received raw catalog or no reviewed summary")
				}
				reply = `{"text":"أهلًا بك! لدينا أجهزة وخدمات متنوعة. أي قسم ترغب في استكشافه؟","covered_item_ids":[]}`
			default:
				t.Errorf("unexpected stage %q", input.Stage)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "completed",
				"outputs": []any{map[string]any{"type": "message", "content": []any{
					map[string]any{"type": "output_text", "text": reply},
				}}},
				"usage": map[string]any{"total_input_tokens": 320, "total_output_tokens": 70},
			})
		default:
			t.Errorf("unexpected endpoint %q", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := litellm.NewClient(litellm.Config{BaseURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewLiteLLMCatalogAdapter(client, "mujeeb-catalog", 4096, 32768)
	if err != nil {
		t.Fatal(err)
	}
	variant, price, currency := "variant-1", "250.0000", "SAR"
	batch := services.CatalogAIBatchPayload{
		Catalogs: []services.CatalogAICatalog{{ID: "catalog", Name: "الإلكترونيات", Status: "active"}},
		Items: []services.CatalogAIItem{{
			ID: "item-1", CatalogID: "catalog", Name: "هاتف",
			Variants: []services.CatalogAIVariant{{ID: variant, CatalogItemID: "item-1"}},
			Offers:   []services.CatalogAIOffer{{ID: "offer", CatalogItemID: "item-1", VariantID: &variant, Amount: &price, Currency: &currency}},
		}},
	}
	request := services.CatalogOverviewRequest{Stage: "batch", CustomerMessage: "ما المتوفر عندكم؟", Batch: &batch, RequestedPresentation: "overview"}
	count, err := adapter.CountOverviewTokens(context.Background(), request)
	if err != nil || count < 320 {
		t.Fatalf("exact overview token counter: %d %v", count, err)
	}
	result, err := adapter.ComposeOverview(context.Background(), request)
	if err != nil || len(result.CoveredItemIDs) != 1 || result.CoveredItemIDs[0] != "item-1" || result.Usage.OutputTokens != 70 {
		t.Fatalf("Gemini batch response invalid: %+v %v", result, err)
	}
	final, err := adapter.ComposeOverview(context.Background(), services.CatalogOverviewRequest{
		Stage: "final", CustomerMessage: "ما المتوفر عندكم؟",
		RequestedPresentation: "overview", Summaries: []string{result.Text},
	})
	if err != nil || len(final.CoveredItemIDs) != 0 || !strings.Contains(final.Text, "أي قسم") || counted < 3 || batchCalls != 1 || finalCalls != 1 {
		t.Fatalf("final AI customer summary missing: %+v err=%v counted=%d batch=%d final=%d", final, err, counted, batchCalls, finalCalls)
	}
}

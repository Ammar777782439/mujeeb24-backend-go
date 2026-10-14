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

func TestLiteLLMComposesCatalogReplyWithWholeCommercialEvidence(t *testing.T) {
	const itemID = "item-1"
	counted, composed := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("LiteLLM API key was not sent")
		}
		switch r.URL.Path {
		case "/utils/token_counter":
			counted++
			_, _ = w.Write([]byte(`{"total_tokens":230}`))
		case "/v1beta/interactions":
			composed++
			var req struct {
				Model             string                     `json:"model"`
				Input             string                     `json:"input"`
				SystemInstruction string                     `json:"system_instruction"`
				ResponseFormat    *interactionResponseFormat `json:"response_format"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Error(err)
				return
			}
			if req.Model != "mujeeb-catalog" || !strings.Contains(req.SystemInstruction, "اكتب جوابًا طبيعيًا") ||
				!strings.Contains(req.Input, "special-feature") || !strings.Contains(req.Input, "offer-price") ||
				!strings.Contains(req.Input, "variants") || !strings.Contains(req.Input, "السعر") ||
				req.ResponseFormat == nil || req.ResponseFormat.Schema == nil {
				t.Errorf("Gemini narration lost commercial facts or its grounded-output contract: %+v", req)
			}
			if req.ResponseFormat != nil {
				properties := req.ResponseFormat.Schema["properties"].(map[string]any)
				allowed := properties["covered_item_ids"].(map[string]any)["items"].(map[string]any)["enum"].([]any)
				if len(allowed) != 1 || allowed[0] != itemID {
					t.Errorf("Gemini coverage IDs not confined to supplied items: %+v", allowed)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "completed",
				"outputs": []any{map[string]any{
					"type": "message", "content": []any{map[string]any{
						"type": "output_text", "text": `{"response_text":"متوفر لدينا هاتف بميزة خاصة وسعر 250 ريال سعودي.","covered_item_ids":["item-1"]}`,
					}},
				}},
				"usage": map[string]any{"total_input_tokens": 321, "total_output_tokens": 43},
			})
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
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
	variant := "variant-1"
	amount, currency := "250.0000", "SAR"
	projection := services.CatalogAIProjection{
		Catalogs: []services.CatalogAICatalog{{ID: "catalog", Name: "منتجات", Status: "active"}},
		Items: []services.CatalogAIItem{{
			ID: itemID, CatalogID: "catalog", Name: "هاتف", Attributes: map[string]any{"special-feature": "مقاوم للماء"},
			Variants: []services.CatalogAIVariant{{ID: variant, CatalogItemID: itemID}},
			Offers: []services.CatalogAIOffer{{ID: "offer-price", CatalogItemID: itemID, VariantID: &variant, Amount: &amount, Currency: &currency}},
		}},
	}
	input := services.CatalogReplyInput{CustomerMessage: "أريد شرح المنتجات", Projection: projection}
	count, err := adapter.CountCatalogReplyTokens(context.Background(), input)
	if err != nil || count < 230 {
		t.Fatalf("token counter failed: %d %v", count, err)
	}
	out, err := adapter.ComposeCatalogReply(context.Background(), input)
	if err != nil || out.Text != "متوفر لدينا هاتف بميزة خاصة وسعر 250 ريال سعودي." ||
		len(out.CoveredItemIDs) != 1 || out.CoveredItemIDs[0] != itemID ||
		out.Usage.OutputTokens != 43 || out.Usage.Provider != "litellm" ||
		counted < 2 || composed != 1 {
		t.Fatalf("Gemini catalog composer failed: %+v, count=%d compose=%d err=%v", out, counted, composed, err)
	}
}

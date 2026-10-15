//go:build ignore

// One real catalog batch through the production LiteLLM Go adapter used by Mujeeb.
// All records are synthetic. No database writes, outbound messages or mocks.
// The encrypted virtual key is provided by GitHub Actions; no key or raw provider content is printed.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/gateway"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/litellm"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

// captureActualRequest inspects the exact provider-bound payload without changing
// its bytes or the production Go HTTP User-Agent. It never prints raw data.
type captureActualRequest struct {
	base     http.RoundTripper
	complete bool
	seen     bool
}

func (c *captureActualRequest) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/v1beta/interactions" {
		c.seen = true
		if req.GetBody != nil {
			reader, err := req.GetBody()
			if err == nil {
				var payload struct {
					Input string `json:"input"`
					Model string `json:"model"`
				}
				parseErr := json.NewDecoder(reader).Decode(&payload)
				reader.Close()
				required := []string{
					"11111111-1111-4111-8111-111111111111",
					"44444444-4444-4444-8444-444444444444",
					"55555555-5555-4555-8555-555555555555",
					"66666666-6666-4666-8666-666666666666",
					"77777777-7777-4777-8777-777777777777",
					"88888888-8888-4888-8888-888888888888",
					"12000.0000", "15000.0000", "10000.0000",
					"attribute_schemas", "definitions", "resolved_attributes",
					"effective_attributes", "item_level_offers",
				}
				c.complete = parseErr == nil && payload.Model != ""
				for _, item := range required {
					if !strings.Contains(payload.Input, item) {
						c.complete = false
					}
				}
			}
		}
		if !c.complete {
			return nil, fmt.Errorf("refusing to send incomplete commercial entity to provider")
		}
	}
	return c.base.RoundTrip(req)
}

func stringPtr(s string) *string { return &s }
func intPtr(v int) *int          { return &v }

func must(stage string, err error) {
	if err == nil {
		return
	}
	var httpErr *litellm.HTTPError
	if errors.As(err, &httpErr) {
		fmt.Fprintf(os.Stderr, "FAIL %s: gateway HTTP %d\n", stage, httpErr.StatusCode)
	} else {
		fmt.Fprintf(os.Stderr, "FAIL %s: %T (message withheld)\n", stage, err)
	}
	os.Exit(1)
}

func main() {
	key := os.Getenv("LITELLM_API_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "missing LITELLM_API_KEY GitHub secret")
		os.Exit(1)
	}
	model := os.Getenv("LITELLM_CATALOG_MODEL")
	if model == "" {
		model = "mujeeb-catalog"
	}
	base := os.Getenv("LITELLM_BASE_URL")
	if base != "https://webhook.mujeeb24.com" {
		fmt.Fprintln(os.Stderr, "unexpected LiteLLM endpoint")
		os.Exit(1)
	}

	capture := &captureActualRequest{base: http.DefaultTransport}
	client, err := litellm.NewClient(litellm.Config{
		BaseURL: base, APIKey: key, Timeout: 55 * time.Second,
		HTTPClient: &http.Client{Timeout: 55 * time.Second, Transport: capture,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	})
	must("create-client", err)
	adapter, err := gateway.NewLiteLLMCatalogAdapter(client, model, 1024, 32768)
	must("create-adapter", err)

	const itemID = "11111111-1111-4111-8111-111111111111"
	const catalogID = "22222222-2222-4222-8222-222222222222"
	const schemaID = "33333333-3333-4333-8333-333333333333"
	const vBlue = "44444444-4444-4444-8444-444444444444"
	const vBlack = "55555555-5555-4555-8555-555555555555"
	const oBlue = "66666666-6666-4666-8666-666666666666"
	const oBlack = "77777777-7777-4777-8777-777777777777"
	const oBase = "88888888-8888-4888-8888-888888888888"
	catalog := services.CatalogAICatalog{ID: catalogID, Name: "هواتف تجريبية", Status: "active"}
	schema := services.CatalogAIAttributeSchema{ID: schemaID, Name: "مواصفات الهواتف", Version: 2,
		Definitions: []services.CatalogAIAttributeDefinition{
			{ID: "definition-storage", SchemaID: schemaID, AttributeKey: "attr_storage", Label: "سعة التخزين", DataType: "text", IsRequired: true},
			{ID: "definition-color", SchemaID: schemaID, AttributeKey: "attr_color", Label: "اللون", DataType: "text", IsRequired: true},
		}}
	blue := services.CatalogAIOffer{ID: oBlue, CatalogItemID: itemID, VariantID: stringPtr(vBlue), Name: "العرض الأزرق", PricingMode: "fixed", Amount: stringPtr("12000.0000"), Currency: stringPtr("YER"), Status: "active"}
	black := services.CatalogAIOffer{ID: oBlack, CatalogItemID: itemID, VariantID: stringPtr(vBlack), Name: "العرض الأسود", PricingMode: "fixed", Amount: stringPtr("15000.0000"), Currency: stringPtr("YER"), Status: "active"}
	standalone := services.CatalogAIOffer{ID: oBase, CatalogItemID: itemID, Name: "العرض الأساسي", PricingMode: "fixed", Amount: stringPtr("10000.0000"), Currency: stringPtr("YER"), Status: "active"}
	item := services.CatalogAIItem{ID: itemID, CatalogID: catalogID, Catalog: &catalog, Schema: &schema,
		AttributeSchemaID: stringPtr(schemaID), AttributeSchemaVersion: intPtr(2),
		ItemType: "physical_good", Name: "هاتف تجريبي", Status: "active",
		PricingMode: "fixed", AvailabilityMode: "always_available", FulfillmentMode: "delivery",
		Attributes:         map[string]any{"attr_storage": "256GB"},
		ResolvedAttributes: []services.CatalogAIResolvedAttribute{{Key: "attr_storage", Label: "سعة التخزين", DataType: "text", Origin: "item", Value: "256GB"}},
		Variants: []services.CatalogAIVariant{
			{ID: vBlue, CatalogItemID: itemID, Name: "الأزرق", Status: "active", Attributes: map[string]any{"attr_color": "blue"},
				EffectiveAttributes: map[string]any{"attr_storage": "256GB", "attr_color": "blue"},
				ResolvedAttributes:  []services.CatalogAIResolvedAttribute{{Key: "attr_color", Label: "اللون", DataType: "text", Origin: "variant", Value: "blue"}, {Key: "attr_storage", Label: "سعة التخزين", DataType: "text", Origin: "item", Value: "256GB"}},
				Offers:              []services.CatalogAIOffer{blue}},
			{ID: vBlack, CatalogItemID: itemID, Name: "الأسود", Status: "active", Attributes: map[string]any{"attr_color": "black"},
				EffectiveAttributes: map[string]any{"attr_storage": "256GB", "attr_color": "black"},
				ResolvedAttributes:  []services.CatalogAIResolvedAttribute{{Key: "attr_color", Label: "اللون", DataType: "text", Origin: "variant", Value: "black"}, {Key: "attr_storage", Label: "سعة التخزين", DataType: "text", Origin: "item", Value: "256GB"}},
				Offers:              []services.CatalogAIOffer{black}},
		},
		Offers:          []services.CatalogAIOffer{blue, black, standalone},
		ItemLevelOffers: []services.CatalogAIOffer{standalone},
	}
	batch := services.CatalogAIBatchPayload{
		BatchNumber: 1, Catalogs: []services.CatalogAICatalog{catalog},
		AttributeSchemas: []services.CatalogAIAttributeSchema{schema}, Items: []services.CatalogAIItem{item},
	}
	input := services.BatchEvaluationInput{
		AIRunID: "synthetic-live-acceptance", BatchNumber: 1,
		CustomerMessage: "أريد مقارنة هذا الهاتف بجميع ألوانه وعروضه الثلاثة مع السعر، لا تسقط خيارًا أو عرضًا.",
		EntityContract:  services.BuildCatalogEntityContractPayload(),
		Batch:           batch,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	tokens, err := adapter.CountBatchTokens(ctx, input)
	must("real-token-counter", err)
	if tokens < 1 {
		fmt.Fprintln(os.Stderr, "FAIL: zero token estimate")
		os.Exit(1)
	}
	fmt.Printf("PASS: real gateway token-counter returned %d tokens for complete commercial batch\n", tokens)
	result, err := adapter.EvaluateBatch(ctx, input)
	must("real-batch-interaction", err)
	if !capture.seen || !capture.complete {
		fmt.Fprintln(os.Stderr, "FAIL: complete commercial entity was not sent")
		os.Exit(1)
	}
	fmt.Println("PASS: actual Go HTTP request included original item, both variants, all 3 linked offers, attribute definitions and precise prices")
	if result.BatchNumber != 1 || len(result.Candidates) == 0 {
		fmt.Fprintln(os.Stderr, "FAIL: provider returned no valid catalog candidate")
		os.Exit(1)
	}
	var selected bool
	for _, candidate := range result.Candidates {
		if candidate.ItemID != itemID {
			fmt.Fprintln(os.Stderr, "FAIL: model invented an item not in verified batch")
			os.Exit(1)
		}
		selected = true
		for _, id := range candidate.VariantIDs {
			if id != vBlue && id != vBlack {
				fmt.Fprintln(os.Stderr, "FAIL: model invented a variant ID")
				os.Exit(1)
			}
		}
		for _, id := range candidate.OfferIDs {
			if id != oBlue && id != oBlack && id != oBase {
				fmt.Fprintln(os.Stderr, "FAIL: model invented an offer ID")
				os.Exit(1)
			}
		}
		fmt.Printf("OBSERVED: optional candidate selection: %d variant IDs, %d offer IDs, item-level offer included=%t\n",
			len(candidate.VariantIDs), len(candidate.OfferIDs), contains(candidate.OfferIDs, oBase))
	}
	if !selected {
		fmt.Fprintln(os.Stderr, "FAIL: original item not selected")
		os.Exit(1)
	}
	fmt.Printf("PASS: real LiteLLM batch evaluated complete item; candidate selection is optional (provider=%s, model=%s, tokens_in=%d, tokens_out=%d)\n",
		result.Usage.Provider, result.Usage.Model, result.Usage.InputTokens, result.Usage.OutputTokens)
	// The production controller rehydrates every selected Item from the full
	// source projection. Offer IDs selected by the first LLM are relevance hints,
	// never authorization to remove offers from the final model request.
	fullProjection := services.CatalogAIProjection{
		Catalogs:         []services.CatalogAICatalog{catalog},
		AttributeSchemas: []services.CatalogAIAttributeSchema{schema},
		Items:            []services.CatalogAIItem{item},
	}
	candidateJSON, err := json.Marshal(result.Candidates)
	must("marshal-candidates", err)
	projectionJSON, err := json.Marshal(fullProjection)
	must("marshal-full-final-evidence", err)
	finalPrompt := fmt.Sprintf("Customer message: %s\n\nVerified conversation context:\n{}\n\nAggregated candidates:\n%s\n\nCandidate catalog projection:\n%s",
		input.CustomerMessage, string(candidateJSON), string(projectionJSON))
	finalInput := services.FinalEvaluationInput{
		AIRunID: input.AIRunID, CustomerMessage: input.CustomerMessage,
		EntityContract: input.EntityContract, CandidateResults: result.Candidates,
	}
	finalTokens, err := adapter.CountFinalTokens(ctx, finalInput, finalPrompt)
	must("real-final-token-counter", err)
	fmt.Printf("PASS: real final model request token-counter returned %d tokens\n", finalTokens)
	decision, finalUsage, err := adapter.FinalEvaluateWithDetails(ctx, finalInput, finalPrompt)
	must("real-final-capability", err)
	if !capture.seen || !capture.complete {
		fmt.Fprintln(os.Stderr, "FAIL: final model was not shown the original complete product")
		os.Exit(1)
	}
	fmt.Printf("PASS: final Go HTTP request retained all 3 offers, both variants, schemas and source prices (model=%s, tokens_in=%d, tokens_out=%d)\n",
		finalUsage.Model, finalUsage.InputTokens, finalUsage.OutputTokens)
	// A real completion can choose any allowed terminal capability, but for
	// this unambiguous price-comparison request it should answer with evidence.
	answer := strings.ReplaceAll(decision.ResponseText, ",", "")
	prices := []string{"10000", "12000", "15000"}
	included := 0
	for _, price := range prices {
		if strings.Contains(answer, price) {
			included++
		}
	}
	fmt.Printf("OBSERVED: final capability=%s, reply_chars=%d, source_prices_mentioned=%d/3\n",
		decision.Capability, len([]rune(decision.ResponseText)), included)
	if included != 3 {
		fmt.Fprintln(os.Stderr, "FAIL: model did not explicitly mention all three verified offer prices in its final answer")
		os.Exit(1)
	}
	fmt.Println("PASS: full real catalog evaluation returned all three prices without truncating commercial evidence")
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

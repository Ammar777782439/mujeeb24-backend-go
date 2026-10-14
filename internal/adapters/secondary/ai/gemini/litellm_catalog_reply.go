package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/prompts"
)

type catalogReplyJSON struct {
	ResponseText    string   `json:"response_text"`
	CoveredItemIDs []string `json:"covered_item_ids"`
}

// The schema restricts Gemini's item references to those in its own chunk.
// Go also checks exact item coverage and limits the response length.
func catalogCustomerReplySchema(projection services.CatalogAIProjection) map[string]any {
	ids := make([]string, 0, len(projection.Items))
	for _, item := range projection.Items {
		ids = append(ids, item.ID)
	}
	idSchema := map[string]any{"type": "string"}
	if len(ids) > 0 {
		idSchema["enum"] = ids
	}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"response_text": map[string]any{"type": "string"},
			"covered_item_ids": map[string]any{"type": "array", "items": idSchema},
		},
		"required": []string{"response_text", "covered_item_ids"},
	}
}

func (c *LiteLLMCatalogAdapter) catalogCustomerReplyRequest(input services.CatalogReplyInput) (interactionRequest, error) {
	if c == nil {
		return interactionRequest{}, errors.New("LiteLLM catalog adapter not configured")
	}
	payload := struct {
		CustomerMessage string                        `json:"customer_message"`
		Conversation    any                           `json:"conversation_context"`
		PartNumber      int                           `json:"part_number"`
		Catalog         services.CatalogAIProjection   `json:"catalog_projection"`
	}{
		CustomerMessage: input.CustomerMessage,
		Conversation: catalogBatchPromptContextFrom(&input.ConversationContext),
		PartNumber: input.PartNumber,
		Catalog: input.Projection,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return interactionRequest{}, fmt.Errorf("serialize customer catalog evidence: %w", err)
	}
	return interactionRequest{
		Model: c.model, Store: false,
		Input: string(body),
		SystemInstruction: prompts.CatalogCustomerReplySystemPrompt,
		ResponseFormat: &interactionResponseFormat{
			Type: "text", MimeType: "application/json",
			Schema: catalogCustomerReplySchema(input.Projection),
		},
		GenerationConfig: interactionGenerationConfig{MaxOutputTokens: c.maxOutputTokens},
	}, nil
}

// Count and send use the same serialized commercial evidence and output schema.
func (c *LiteLLMCatalogAdapter) CountCatalogReplyTokens(ctx context.Context, input services.CatalogReplyInput) (int, error) {
	request, err := c.catalogCustomerReplyRequest(input)
	if err != nil {
		return 0, err
	}
	return c.count(ctx, request)
}

func (c *LiteLLMCatalogAdapter) ComposeCatalogReply(ctx context.Context, input services.CatalogReplyInput) (services.CatalogReplyOutput, error) {
	req, err := c.catalogCustomerReplyRequest(input)
	if err != nil {
		return services.CatalogReplyOutput{}, err
	}
	estimate, err := c.count(ctx, req)
	if err != nil {
		return services.CatalogReplyOutput{}, err
	}
	if err := c.ensureModelCapacity(estimate); err != nil {
		return services.CatalogReplyOutput{}, err
	}
	started := time.Now()
	raw, err := c.client.SendInteraction(ctx, req)
	if err != nil {
		return services.CatalogReplyOutput{}, fmt.Errorf("compose customer catalog message via Gemini: %w", err)
	}
	normalized, err := normalizeLiteLLMResponse(raw)
	if err != nil {
		return services.CatalogReplyOutput{}, err
	}
	if normalized.Status != "completed" {
		return services.CatalogReplyOutput{}, fmt.Errorf("Gemini customer catalog response not completed: %s", normalized.Status)
	}
	if len(normalized.FunctionCalls) > 0 {
		return services.CatalogReplyOutput{}, errors.New("Gemini customer catalog composition must not request tools")
	}
	decoder := json.NewDecoder(strings.NewReader(normalized.Text))
	decoder.DisallowUnknownFields()
	var result catalogReplyJSON
	if err := decoder.Decode(&result); err != nil {
		return services.CatalogReplyOutput{}, fmt.Errorf("decode Gemini-authored catalog response: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return services.CatalogReplyOutput{}, errors.New("unexpected extra content in Gemini catalog response")
	}
	if strings.TrimSpace(result.ResponseText) == "" {
		return services.CatalogReplyOutput{}, errors.New("Gemini returned an empty catalog reply")
	}
	return services.CatalogReplyOutput{
		Text: result.ResponseText, CoveredItemIDs: result.CoveredItemIDs,
		Usage: ports.CustomerSalesUsageTelemetry{
			Provider: "litellm", Model: c.model,
			InputTokens: normalized.Usage.TotalInputTokens,
			CachedTokens: normalized.Usage.TotalCachedTokens,
			OutputTokens: normalized.Usage.TotalOutputTokens,
			ModelRequests: 1, LatencyMs: time.Since(started).Milliseconds(),
		},
	}, nil
}

var _ services.CatalogReplyComposer = (*LiteLLMCatalogAdapter)(nil)

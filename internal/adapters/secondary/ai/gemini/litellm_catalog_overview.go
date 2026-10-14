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

type catalogOverviewJSON struct {
	Text           string   `json:"text"`
	CoveredItemIDs []string `json:"covered_item_ids"`
}

func catalogOverviewResponseSchema(input services.CatalogOverviewRequest) map[string]any {
	ids := make([]string, 0)
	if input.Batch != nil {
		for _, item := range input.Batch.Items {
			ids = append(ids, item.ID)
		}
	}
	idSchema := map[string]any{"type": "string"}
	if len(ids) > 0 {
		idSchema["enum"] = ids
	}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"text":             map[string]any{"type": "string"},
			"covered_item_ids": map[string]any{"type": "array", "items": idSchema},
		},
		"required": []string{"text", "covered_item_ids"},
	}
}

func (c *LiteLLMCatalogAdapter) overviewRequest(input services.CatalogOverviewRequest) (interactionRequest, error) {
	if c == nil {
		return interactionRequest{}, errors.New("catalog AI adapter is not configured")
	}
	if input.Stage != "batch" && input.Stage != "merge" && input.Stage != "final" {
		return interactionRequest{}, errors.New("unsupported catalog overview task")
	}
	if input.Stage == "batch" && (input.Batch == nil || len(input.Batch.Items) == 0) {
		return interactionRequest{}, errors.New("batch overview requires original catalog evidence")
	}
	if input.Stage != "batch" && input.Batch != nil {
		return interactionRequest{}, errors.New("catalog overview merging cannot accept raw unverified batch evidence")
	}
	payload := struct {
		Stage        string                          `json:"stage"`
		Request      string                          `json:"customer_message"`
		Conversation any                             `json:"conversation_context"`
		Presentation string                          `json:"requested_presentation"`
		CatalogBatch *services.CatalogAIBatchPayload `json:"catalog_batch,omitempty"`
		Summaries    []string                        `json:"reviewed_summaries,omitempty"`
	}{
		Stage: input.Stage, Request: input.CustomerMessage,
		Conversation: catalogBatchPromptContextFrom(&input.ConversationContext),
		Presentation: input.RequestedPresentation, CatalogBatch: input.Batch,
		Summaries: input.Summaries,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return interactionRequest{}, fmt.Errorf("encode catalog overview input: %w", err)
	}
	return interactionRequest{
		Model: c.model, Store: false,
		Input:             string(data),
		SystemInstruction: prompts.CatalogOverviewSystemPrompt,
		ResponseFormat: &interactionResponseFormat{
			Type: "text", MimeType: "application/json",
			Schema: catalogOverviewResponseSchema(input),
		},
		GenerationConfig: interactionGenerationConfig{MaxOutputTokens: c.maxOutputTokens},
	}, nil
}

func (c *LiteLLMCatalogAdapter) CountOverviewTokens(ctx context.Context, input services.CatalogOverviewRequest) (int, error) {
	req, err := c.overviewRequest(input)
	if err != nil {
		return 0, err
	}
	return c.count(ctx, req)
}

func (c *LiteLLMCatalogAdapter) ComposeOverview(ctx context.Context, input services.CatalogOverviewRequest) (services.CatalogOverviewResponse, error) {
	req, err := c.overviewRequest(input)
	if err != nil {
		return services.CatalogOverviewResponse{}, err
	}
	count, err := c.count(ctx, req)
	if err != nil {
		return services.CatalogOverviewResponse{}, err
	}
	if err := c.ensureModelCapacity(count); err != nil {
		return services.CatalogOverviewResponse{}, err
	}
	started := time.Now()
	raw, err := c.client.SendInteraction(ctx, req)
	if err != nil {
		return services.CatalogOverviewResponse{}, fmt.Errorf("Gemini catalog overview: %w", err)
	}
	out, err := normalizeLiteLLMResponse(raw)
	if err != nil {
		return services.CatalogOverviewResponse{}, err
	}
	if out.Status != "completed" || len(out.FunctionCalls) != 0 {
		return services.CatalogOverviewResponse{}, fmt.Errorf("Gemini overview did not complete as structured text: %s", out.Status)
	}
	decoder := json.NewDecoder(strings.NewReader(out.Text))
	decoder.DisallowUnknownFields()
	var body catalogOverviewJSON
	if err := decoder.Decode(&body); err != nil {
		return services.CatalogOverviewResponse{}, fmt.Errorf("decode AI overview: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return services.CatalogOverviewResponse{}, errors.New("extra content following Gemini overview JSON")
	}
	if strings.TrimSpace(body.Text) == "" {
		return services.CatalogOverviewResponse{}, errors.New("empty Gemini overview text")
	}
	if input.Stage != "batch" && len(body.CoveredItemIDs) > 0 {
		return services.CatalogOverviewResponse{}, errors.New("Gemini invented item references during overview synthesis")
	}
	return services.CatalogOverviewResponse{
		Text: body.Text, CoveredItemIDs: body.CoveredItemIDs,
		Usage: ports.CustomerSalesUsageTelemetry{
			Provider: "litellm", Model: c.model,
			InputTokens: out.Usage.TotalInputTokens, CachedTokens: out.Usage.TotalCachedTokens,
			OutputTokens: out.Usage.TotalOutputTokens, ModelRequests: 1,
			LatencyMs: time.Since(started).Milliseconds(),
		},
	}, nil
}

var _ services.CatalogOverviewClient = (*LiteLLMCatalogAdapter)(nil)

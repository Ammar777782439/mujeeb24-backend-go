package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/litellm"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/prompts"
)

type catalogOverviewJSON struct {
	Text string `json:"text"`
}

func catalogOverviewResponseSchema() map[string]any {
	// Overview is a semantic digest, not an item-by-item customer listing.
	// Merchant item coverage is enforced by the Go controller from the
	// original projection and completed provider calls.
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
		},
		"required": []string{"text"},
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
			Schema: catalogOverviewResponseSchema(),
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
	ctx = litellm.WithSpendAttribution(ctx, input.ConversationContext.Business.Reference, "", "catalog_overview")
	raw, meta, err := sendGatewayChatWithMetadata(ctx, c.client, req)
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
	return services.CatalogOverviewResponse{
		Text: body.Text,
		Usage: ports.CustomerSalesUsageTelemetry{
			Provider: "litellm", Model: c.model,
			InputTokens: out.Usage.TotalInputTokens, CachedTokens: out.Usage.TotalCachedTokens,
			OutputTokens: out.Usage.TotalOutputTokens, ModelRequests: 1,
			LatencyMs: time.Since(started).Milliseconds(), GatewayCallID: meta.CallID, GatewayDeploymentID: meta.DeploymentID, GatewayCostUSD: meta.CostUSD, GatewayCostKnown: meta.HasCost,
		},
	}, nil
}

var _ services.CatalogOverviewClient = (*LiteLLMCatalogAdapter)(nil)

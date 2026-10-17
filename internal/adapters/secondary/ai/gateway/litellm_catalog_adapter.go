package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/litellm"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/prompts"
)

// LiteLLMCatalogAdapter reuses the existing COMPLETE catalog projection,
// system prompts, schemas, capability tools, and final evidence validation.
// The CatalogBatchController still owns paging, coverage and revision checks.
type LiteLLMCatalogAdapter struct {
	client          *litellm.Client
	model           string
	maxOutputTokens int
	maxInputTokens  int
}

func NewLiteLLMCatalogAdapter(client *litellm.Client, model string, maxOutputTokens, maxInputTokens int) (*LiteLLMCatalogAdapter, error) {
	if client == nil || strings.TrimSpace(model) == "" || maxOutputTokens <= 0 || maxInputTokens <= maxOutputTokens {
		return nil, errors.New("LiteLLM catalog adapter needs client, model and input context larger than reserved output")
	}
	return &LiteLLMCatalogAdapter{client: client, model: model, maxOutputTokens: maxOutputTokens, maxInputTokens: maxInputTokens}, nil
}

// Reuse the existing catalog payload builders rather than maintain an
// independently drifting catalog prompt/schema implementation.
func (c *LiteLLMCatalogAdapter) builder() *catalogRequestBuilder {
	return &catalogRequestBuilder{systemPrompt: prompts.BatchEvaluationSystemPrompt}
}

func (c *LiteLLMCatalogAdapter) batchRequest(input services.BatchEvaluationInput) (chatCompletionRequest, error) {
	rc := &catalogRequestConfig{model: c.model, maxOutputTokens: c.maxOutputTokens}
	request, err := c.builder().buildEvaluateBatchRequest(input, rc)
	if err != nil { return chatCompletionRequest{}, err }
	return batchChatRequest(request, c.model)
}

func (c *LiteLLMCatalogAdapter) finalRequest(input services.FinalEvaluationInput, userPrompt string) (chatCompletionRequest, error) {
	rc := &catalogRequestConfig{model: c.model, maxOutputTokens: c.maxOutputTokens}
	return batchChatRequest(c.builder().buildFinalEvaluateRequest(input, userPrompt, rc), c.model)
}

func (c *LiteLLMCatalogAdapter) count(ctx context.Context, req chatCompletionRequest) (int, error) {
	counter := map[string]any{"model": req.Model, "messages": req.Messages}
	if len(req.Tools) > 0 {
		counter["tools"] = req.Tools
		counter["tool_choice"] = req.ToolChoice
		counter["parallel_tool_calls"] = false
	}
	if req.ResponseFormat != nil { counter["response_format"] = req.ResponseFormat }
	return c.client.CountChatTokens(ctx, counter)
}

// BatchTokenBudget reserves output and a safety margin inside the smallest
// actual context window across every deployment and fallback in this route.
// The operator MUST configure maxInputTokens against that minimum capacity.
func (c *LiteLLMCatalogAdapter) BatchTokenBudget() (int, error) {
	if c == nil {
		return 0, errors.New("LiteLLM catalog adapter is nil")
	}
	return services.LiteLLMCatalogTokenBudget(c.maxInputTokens, c.maxOutputTokens)
}

// Splitting must receive the oversized count so the CatalogBatchController
// can divide the projection. Actual sends are separately blocked if unsafe.
func (c *LiteLLMCatalogAdapter) ensureModelCapacity(count int) error {
	if count+c.maxOutputTokens > c.maxInputTokens {
		return fmt.Errorf("LiteLLM catalog request exceeds configured minimum model context: estimate=%d reserved_output=%d context=%d", count, c.maxOutputTokens, c.maxInputTokens)
	}
	return nil
}

func (c *LiteLLMCatalogAdapter) CountBatchTokens(ctx context.Context, input services.BatchEvaluationInput) (int, error) {
	request, err := c.batchRequest(input)
	if err != nil {
		return 0, err
	}
	return c.count(ctx, request)
}

func (c *LiteLLMCatalogAdapter) CountFinalTokens(ctx context.Context, input services.FinalEvaluationInput, userPrompt string) (int, error) {
	req, err := c.finalRequest(input, userPrompt)
	if err != nil { return 0, err }
	return c.count(ctx, req)
}

func (c *LiteLLMCatalogAdapter) EvaluateBatch(ctx context.Context, input services.BatchEvaluationInput) (ports.CatalogBatchResult, error) {
	if len(input.Batch.Items) == 0 {
		return ports.CatalogBatchResult{BatchNumber: input.BatchNumber}, nil
	}
	req, err := c.batchRequest(input)
	if err != nil {
		return ports.CatalogBatchResult{}, err
	}
	tokens, err := c.count(ctx, req)
	if err != nil {
		return ports.CatalogBatchResult{}, err
	}
	if err := c.ensureModelCapacity(tokens); err != nil {
		return ports.CatalogBatchResult{}, err
	}
	started := time.Now()
	ctx = litellm.WithSpendAttribution(ctx, input.BusinessID, input.AIRunID, "catalog_batch")
	raw, meta, err := c.client.SendChatCompletionWithMetadata(ctx, req)
	if err != nil {
		return ports.CatalogBatchResult{}, fmt.Errorf("send catalog batch %d: %w", input.BatchNumber, err)
	}
	normalized, err := normalizeLiteLLMResponse(raw)
	if err != nil {
		return ports.CatalogBatchResult{}, err
	}

	if len(normalized.FunctionCalls) > 0 {
		return ports.CatalogBatchResult{}, errors.New("catalog batch returned unexpected function call")
	}
	if err := validateStrictArguments([]byte(normalized.Text), req.ResponseFormat.JSONSchema.Schema); err != nil {
		return ports.CatalogBatchResult{}, fmt.Errorf("invalid catalog candidates schema: %w", err)
	}
	candidates, err := parseBatchCandidates(catalogCandidateResponse{Candidates: []batchCandidate{{Content: batchContent{Parts: []batchPart{{Text: normalized.Text}}}}}})
	if err != nil {
		return ports.CatalogBatchResult{}, err
	}
	latency := time.Since(started).Milliseconds()
	return ports.CatalogBatchResult{
		BatchNumber: input.BatchNumber,
		Candidates:  candidates,
		Usage: ports.CustomerSalesUsageTelemetry{
			Provider: "litellm", Model: c.model, InputTokens: normalized.Usage.PromptTokens,
			CachedTokens: normalized.Usage.CachedTokens, OutputTokens: normalized.Usage.CompletionTokens,
			ModelRequests: 1, LatencyMs: latency, GatewayCallID: meta.CallID, GatewayDeploymentID: meta.DeploymentID, GatewayCostUSD: meta.CostUSD, GatewayCostKnown: meta.HasCost,
		},
		LatencyMs: latency,
	}, nil
}

func (c *LiteLLMCatalogAdapter) FinalEvaluateWithDetails(ctx context.Context, input services.FinalEvaluationInput, userPrompt string) (ports.CapabilityDecision, ports.CustomerSalesUsageTelemetry, error) {
	req, err := c.finalRequest(input, userPrompt)
	if err != nil { return ports.CapabilityDecision{}, ports.CustomerSalesUsageTelemetry{}, err }
	tokens, err := c.count(ctx, req)
	if err != nil {
		return ports.CapabilityDecision{}, ports.CustomerSalesUsageTelemetry{}, err
	}
	if err := c.ensureModelCapacity(tokens); err != nil {
		return ports.CapabilityDecision{}, ports.CustomerSalesUsageTelemetry{}, err
	}
	started := time.Now()
	ctx = litellm.WithSpendAttribution(ctx, input.BusinessID, input.AIRunID, "catalog_final")
	raw, meta, err := c.client.SendChatCompletionWithMetadata(ctx, req)
	if err != nil {
		return ports.CapabilityDecision{}, ports.CustomerSalesUsageTelemetry{}, fmt.Errorf("send final catalog evaluation: %w", err)
	}
	normalized, err := normalizeLiteLLMResponse(raw)
	if err != nil {
		return ports.CapabilityDecision{}, ports.CustomerSalesUsageTelemetry{}, err
	}

	tools := customerSalesTools(ports.CustomerSalesCapabilities(false, input.ConversationContext.Sales != nil && input.ConversationContext.Sales.Enabled && input.ConversationContext.Sales.AllowInteractions))
	if normalized.Text != "" {
		return ports.CapabilityDecision{}, ports.CustomerSalesUsageTelemetry{}, errors.New("final catalog returned plain text alongside capability")
	}
	decision, err := parseCustomerSalesCapability(capabilityResponse{Calls: normalized.FunctionCalls}, tools)
	if err != nil {
		return ports.CapabilityDecision{}, ports.CustomerSalesUsageTelemetry{}, err
	}
	def, ok := ports.CustomerSalesCapabilityDefinitionFor(decision.Capability)
	if !ok || !def.Terminal {
		return ports.CapabilityDecision{}, ports.CustomerSalesUsageTelemetry{}, errors.New("non-terminal capability after full catalog evaluation")
	}
	return decision, ports.CustomerSalesUsageTelemetry{
		Provider: "litellm", Model: c.model, InputTokens: normalized.Usage.PromptTokens,
		CachedTokens: normalized.Usage.CachedTokens, OutputTokens: normalized.Usage.CompletionTokens,
		ModelRequests: 1, LatencyMs: time.Since(started).Milliseconds(), GatewayCallID: meta.CallID, GatewayDeploymentID: meta.DeploymentID, GatewayCostUSD: meta.CostUSD, GatewayCostKnown: meta.HasCost,
	}, nil
}

var _ services.CatalogEvaluationClient = (*LiteLLMCatalogAdapter)(nil)
var _ services.BatchRequestTokenEstimator = (*LiteLLMCatalogAdapter)(nil)
var _ services.FinalRequestTokenEstimator = (*LiteLLMCatalogAdapter)(nil)

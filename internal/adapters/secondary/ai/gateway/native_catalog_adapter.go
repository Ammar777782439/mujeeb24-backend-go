package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/prompts"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/catalogdump"
)

// NativeCatalogAdapter reuses the existing COMPLETE catalog projection,
// system prompts, schemas, capability tools, and final evidence validation.
// The CatalogBatchController still owns paging, coverage and revision checks.
type NativeCatalogAdapter struct {
	client          *native.CompletionClient
	model           string
	maxOutputTokens int
	maxInputTokens  int
}

func NewNativeCatalogAdapter(client *native.CompletionClient, model string, maxOutputTokens, maxInputTokens int) (*NativeCatalogAdapter, error) {
	if client == nil || strings.TrimSpace(model) == "" || maxOutputTokens <= 0 || maxInputTokens <= maxOutputTokens {
		return nil, errors.New("native catalog adapter needs client, model and input context larger than reserved output")
	}
	return &NativeCatalogAdapter{client: client, model: model, maxOutputTokens: maxOutputTokens, maxInputTokens: maxInputTokens}, nil
}

// Reuse the existing catalog payload builders rather than maintain an
// independently drifting catalog prompt/schema implementation.
func (c *NativeCatalogAdapter) builder() *catalogRequestBuilder {
	return &catalogRequestBuilder{systemPrompt: prompts.BatchEvaluationSystemPrompt}
}

func (c *NativeCatalogAdapter) batchRequest(input services.BatchEvaluationInput) (chatCompletionRequest, error) {
	rc := &catalogRequestConfig{model: c.model, maxOutputTokens: c.maxOutputTokens}
	request, err := c.builder().buildEvaluateBatchRequest(input, rc)
	if err != nil {
		return chatCompletionRequest{}, err
	}
	return batchChatRequest(request, c.model)
}

func (c *NativeCatalogAdapter) finalRequest(input services.FinalEvaluationInput, userPrompt string) (chatCompletionRequest, error) {
	rc := &catalogRequestConfig{model: c.model, maxOutputTokens: c.maxOutputTokens}
	return batchChatRequest(c.builder().buildFinalEvaluateRequest(input, userPrompt, rc), c.model)
}

// Native providers do not share a tokenizer. Serialized UTF-8 byte count
// plus a fixed protocol overhead is a conservative admission estimate, not
// a claim of exact provider token usage. Never truncate catalog evidence.
func (c *NativeCatalogAdapter) count(ctx context.Context, req chatCompletionRequest) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		return 0, err
	}
	const protocolReserve = 4096
	if len(encoded) > int(^uint(0)>>1)-protocolReserve {
		return 0, errors.New("native catalog request size overflow")
	}
	return len(encoded) + protocolReserve, nil
}

// BatchTokenBudget reserves output and a safety margin inside the smallest
// actual context window across every deployment and fallback in this route.
// The operator MUST configure maxInputTokens against that minimum capacity.
func (c *NativeCatalogAdapter) BatchTokenBudget() (int, error) {
	if c == nil {
		return 0, errors.New("native catalog adapter is nil")
	}
	return services.NativeCatalogTokenBudget(c.maxInputTokens, c.maxOutputTokens)
}

// Splitting must receive the oversized count so the CatalogBatchController
// can divide the projection. Actual sends are separately blocked if unsafe.
func (c *NativeCatalogAdapter) ensureModelCapacity(count int) error {
	if count+c.maxOutputTokens > c.maxInputTokens {
		return fmt.Errorf("native catalog request exceeds configured minimum model context: estimate=%d reserved_output=%d context=%d", count, c.maxOutputTokens, c.maxInputTokens)
	}
	return nil
}

func (c *NativeCatalogAdapter) CountBatchTokens(ctx context.Context, input services.BatchEvaluationInput) (int, error) {
	request, err := c.batchRequest(input)
	if err != nil {
		return 0, err
	}
	return c.count(ctx, request)
}

func (c *NativeCatalogAdapter) CountFinalTokens(ctx context.Context, input services.FinalEvaluationInput, userPrompt string) (int, error) {
	req, err := c.finalRequest(input, userPrompt)
	if err != nil {
		return 0, err
	}
	return c.count(ctx, req)
}

func (c *NativeCatalogAdapter) EvaluateBatch(ctx context.Context, input services.BatchEvaluationInput) (ports.CatalogBatchResult, error) {
	ctx = catalogdump.WithRun(ctx, input.AIRunID)
	catalogdump.Record(ctx, "batch_catalog_input", input.Batch)
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
	catalogdump.Record(ctx, "catalog_canonical_request", req)
	started := time.Now()
	raw, info, err := c.client.Complete(ctx, req)
	if err != nil {
		return ports.CatalogBatchResult{}, fmt.Errorf("send catalog batch %d: %w", input.BatchNumber, err)
	}
	catalogdump.Record(ctx, "catalog_normalized_response", catalogdump.Raw(raw))
	normalized, err := normalizeNativeResponse(raw)
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
	if int64(normalized.Usage.PromptTokens) != info.InputTokens ||
		int64(normalized.Usage.CompletionTokens) != info.OutputTokens ||
		int64(normalized.Usage.CachedTokens) != info.CachedTokens ||
		len(info.Trace.Attempts) == 0 {
		return ports.CatalogBatchResult{}, errors.New("native catalog usage mismatch")
	}
	catalogdump.Record(ctx, "batch_parsed_candidates", candidates)
	latency := time.Since(started).Milliseconds()
	return ports.CatalogBatchResult{
		BatchNumber: input.BatchNumber,
		Candidates:  candidates,
		Usage: ports.CustomerSalesUsageTelemetry{
			Provider: info.Provider, Model: info.Model, InputTokens: normalized.Usage.PromptTokens,
			CachedTokens: normalized.Usage.CachedTokens, OutputTokens: normalized.Usage.CompletionTokens,
			ModelRequests: len(info.Trace.Attempts), LatencyMs: latency,
		},
		LatencyMs: latency,
	}, nil
}

func (c *NativeCatalogAdapter) FinalEvaluateWithDetails(ctx context.Context, input services.FinalEvaluationInput, userPrompt string) (ports.CapabilityDecision, ports.CustomerSalesUsageTelemetry, error) {
	ctx = catalogdump.WithRun(ctx, input.AIRunID)
	catalogdump.Record(ctx, "final_catalog_input", input)
	req, err := c.finalRequest(input, userPrompt)
	if err != nil {
		return ports.CapabilityDecision{}, ports.CustomerSalesUsageTelemetry{}, err
	}
	tokens, err := c.count(ctx, req)
	if err != nil {
		return ports.CapabilityDecision{}, ports.CustomerSalesUsageTelemetry{}, err
	}
	if err := c.ensureModelCapacity(tokens); err != nil {
		return ports.CapabilityDecision{}, ports.CustomerSalesUsageTelemetry{}, err
	}
	catalogdump.Record(ctx, "catalog_canonical_request", req)
	started := time.Now()
	raw, info, err := c.client.Complete(ctx, req)
	if err != nil {
		return ports.CapabilityDecision{}, ports.CustomerSalesUsageTelemetry{}, fmt.Errorf("send final catalog evaluation: %w", err)
	}
	catalogdump.Record(ctx, "catalog_normalized_response", catalogdump.Raw(raw))
	normalized, err := normalizeNativeResponse(raw)
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
	if int64(normalized.Usage.PromptTokens) != info.InputTokens ||
		int64(normalized.Usage.CompletionTokens) != info.OutputTokens ||
		int64(normalized.Usage.CachedTokens) != info.CachedTokens ||
		len(info.Trace.Attempts) == 0 {
		return ports.CapabilityDecision{}, ports.CustomerSalesUsageTelemetry{}, errors.New("native final catalog usage mismatch")
	}
	catalogdump.Record(ctx, "final_parsed_decision", decision)
	return decision, ports.CustomerSalesUsageTelemetry{
		Provider: info.Provider, Model: info.Model, InputTokens: normalized.Usage.PromptTokens,
		CachedTokens: normalized.Usage.CachedTokens, OutputTokens: normalized.Usage.CompletionTokens,
		ModelRequests: len(info.Trace.Attempts), LatencyMs: time.Since(started).Milliseconds(),
	}, nil
}

var _ services.CatalogEvaluationClient = (*NativeCatalogAdapter)(nil)
var _ services.BatchRequestTokenEstimator = (*NativeCatalogAdapter)(nil)
var _ services.FinalRequestTokenEstimator = (*NativeCatalogAdapter)(nil)

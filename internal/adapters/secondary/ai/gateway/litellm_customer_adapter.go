package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/litellm"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/prompts"
)

// LiteLLMCustomerSalesAdapter reuses the EXACT B2C tools, prompt and strict
// capability parser shared by all gateway models. Models cannot execute Mujeeb capabilities.
// All execution uses the provider-neutral gateway contracts.
type LiteLLMCustomerSalesAdapter struct {
	client             *litellm.Client
	model              string
	maxOutputTokens    int
	maxInputCharacters int
}

func NewLiteLLMCustomerSalesAdapter(client *litellm.Client, model string, maxOutputTokens, maxInputCharacters int) (*LiteLLMCustomerSalesAdapter, error) {
	if client == nil || strings.TrimSpace(model) == "" || maxOutputTokens <= 0 || maxInputCharacters <= 0 {
		return nil, errors.New("LiteLLM customer sales requires client, model and positive limits")
	}
	return &LiteLLMCustomerSalesAdapter{
		client: client, model: model, maxOutputTokens: maxOutputTokens, maxInputCharacters: maxInputCharacters,
	}, nil
}

func (c *LiteLLMCustomerSalesAdapter) Decide(ctx context.Context, input ports.CustomerSalesDecisionInput) (ports.CustomerSalesDecisionOutput, error) {
	if err := ctx.Err(); err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}
	if strings.TrimSpace(input.Request.Text) == "" {
		return ports.CustomerSalesDecisionOutput{}, errors.New("AI input text is required")
	}
	if len([]rune(input.Request.Text)) > c.maxInputCharacters {
		return ports.CustomerSalesDecisionOutput{}, errors.New("LiteLLM customer input exceeds configured limit")
	}
	capabilities := input.Capabilities
	if capabilities == nil {
		capabilities = ports.CustomerSalesCapabilities(false)
	}
	tools := customerSalesTools(capabilities)
	if len(tools) == 0 {
		return ports.CustomerSalesDecisionOutput{}, errors.New("no customer sales capabilities available")
	}
	systemInstruction := prompts.CustomerSalesSystemPrompt
	if len(input.EntityContractPayload) > 0 {
		systemInstruction += "\n\n# Catalog Entity Contract\n" + string(input.EntityContractPayload)
	}
	userPrompt := buildUserPrompt(input.Request)
	encodedTools, err := json.Marshal(tools)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}
	if len([]rune(systemInstruction))+len([]rune(userPrompt))+len([]rune(string(encodedTools))) > c.maxInputCharacters {
		return ports.CustomerSalesDecisionOutput{}, errors.New("LiteLLM customer sales request exceeds configured context limit")
	}
	// A proposed tool call never constitutes evidence of execution.
	// System context is isolated from user-controlled text in the request.
	req, err := capabilityChatRequest(c.model, systemInstruction, userPrompt, tools, c.maxOutputTokens)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}
	started := time.Now()
	ctx = litellm.WithSpendAttribution(ctx, input.Request.BusinessID, input.AIRunID, "customer_decision")
	raw, meta, err := c.client.SendChatCompletionWithMetadata(ctx, req)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}
	normalized, err := normalizeLiteLLMResponse(raw)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}
	if normalized.Text != "" {
		return ports.CustomerSalesDecisionOutput{}, errors.New("customer decision returned plain text instead of one capability")
	}
	decision, err := parseCustomerSalesCapability(capabilityResponse{Calls: normalized.FunctionCalls}, tools)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, fmt.Errorf("%w; LiteLLM response shape: %s", err, summarizeDecisionResponseShape(normalized))
	}

	latencyMs := time.Since(started).Milliseconds()
	return ports.CustomerSalesDecisionOutput{
		Decision: decision,
		Usage: ports.CustomerSalesUsageTelemetry{
			Provider: "litellm", InputTokens: normalized.Usage.PromptTokens,
			CachedTokens: normalized.Usage.CachedTokens, OutputTokens: normalized.Usage.CompletionTokens,
			Model: c.model, LatencyMs: latencyMs, ModelRequests: 1,
			GatewayCallID: meta.CallID, GatewayDeploymentID: meta.DeploymentID,
			GatewayCostUSD: meta.CostUSD, GatewayCostKnown: meta.HasCost,
		},
		LatencyMs: latencyMs,
	}, nil
}

// Only structural diagnostics are emitted. Arguments and customer text are private.
func summarizeDecisionResponseShape(resp normalizedLiteLLMResponse) string {
	parts := []string{fmt.Sprintf("tool_calls=%d text_present=%t", len(resp.FunctionCalls), resp.Text != "")}
	for i, call := range resp.FunctionCalls {
		parts = append(parts, fmt.Sprintf("call[%d]={id_present=%t name=%q arguments_bytes=%d}",
			i, strings.TrimSpace(call.ID) != "", call.Name, len(call.Arguments)))
	}
	return strings.Join(parts, " ")
}

var _ ports.CustomerSalesDecisionPort = (*LiteLLMCustomerSalesAdapter)(nil)

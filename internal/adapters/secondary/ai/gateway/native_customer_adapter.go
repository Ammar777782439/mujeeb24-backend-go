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
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/prompts"
)

// NativeCustomerSalesAdapter reuses the EXACT B2C tools, prompt and strict
// capability parser shared by all gateway models. Models cannot execute Mujeeb capabilities.
// All execution uses the provider-neutral gateway contracts.
type NativeCustomerSalesAdapter struct {
	client             *native.CompletionClient
	model              string
	maxOutputTokens    int
	maxInputCharacters int
}

func NewNativeCustomerSalesAdapter(client *native.CompletionClient, model string, maxOutputTokens, maxInputCharacters int) (*NativeCustomerSalesAdapter, error) {
	if client == nil || strings.TrimSpace(model) == "" || maxOutputTokens <= 0 || maxInputCharacters <= 0 {
		return nil, errors.New("native customer sales requires client, model and positive limits")
	}
	return &NativeCustomerSalesAdapter{
		client: client, model: model, maxOutputTokens: maxOutputTokens, maxInputCharacters: maxInputCharacters,
	}, nil
}

func (c *NativeCustomerSalesAdapter) Decide(ctx context.Context, input ports.CustomerSalesDecisionInput) (ports.CustomerSalesDecisionOutput, error) {
	if err := ctx.Err(); err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}
	if strings.TrimSpace(input.Request.Text) == "" {
		return ports.CustomerSalesDecisionOutput{}, errors.New("AI input text is required")
	}
	if len([]rune(input.Request.Text)) > c.maxInputCharacters {
		return ports.CustomerSalesDecisionOutput{}, errors.New("native customer input exceeds configured limit")
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
	if input.Request.Context != nil && input.Request.Context.SalesStartContinuation != nil {
		// This is verified server state, not a synthetic customer utterance.
		systemInstruction += "\n\n# Verified sales-start continuation\nThe customer already accepted starting preparation of the existing sales workflow. The customer message shown is the ORIGINAL earlier customer message for context, NOT a new request. Do not offer sales-start confirmation again, do not claim the final transaction was confirmed, and do not repeat information the customer already provided. Ask only what is still missing or answer from verified catalog evidence; hand off when necessary."
	}
	if len(input.EntityContractPayload) > 0 {
		systemInstruction += "\n\n# Catalog Entity Contract\n" + string(input.EntityContractPayload)
	}
	userPrompt := buildUserPrompt(input.Request)
	encodedTools, err := json.Marshal(tools)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}
	if len([]rune(systemInstruction))+len([]rune(userPrompt))+len([]rune(string(encodedTools))) > c.maxInputCharacters {
		return ports.CustomerSalesDecisionOutput{}, errors.New("native customer sales request exceeds configured context limit")
	}
	// A proposed tool call never constitutes evidence of execution.
	// System context is isolated from user-controlled text in the request.
	req, err := capabilityChatRequest(c.model, systemInstruction, userPrompt, tools, c.maxOutputTokens)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}
	started := time.Now()
	raw, modelInfo, err := c.client.Complete(ctx, req)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}
	normalized, err := normalizeNativeResponse(raw)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}
	if normalized.Text != "" {
		return ports.CustomerSalesDecisionOutput{}, errors.New("customer decision returned plain text instead of one capability")
	}
	decision, err := parseCustomerSalesCapability(capabilityResponse{Calls: normalized.FunctionCalls}, tools)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, fmt.Errorf("%w; native response shape: %s", err, summarizeDecisionResponseShape(normalized))
	}

	// The native boundary cannot invent usage or count provider attempts as
	// separate commercial operations. Preserve the actual model/provider.
	if int64(normalized.Usage.PromptTokens) != modelInfo.InputTokens ||
		int64(normalized.Usage.CompletionTokens) != modelInfo.OutputTokens ||
		int64(normalized.Usage.CachedTokens) != modelInfo.CachedTokens ||
		len(modelInfo.Trace.Attempts) < 1 {
		return ports.CustomerSalesDecisionOutput{}, errors.New("native customer usage or attempt trace disagrees with model response")
	}
	latencyMs := time.Since(started).Milliseconds()
	return ports.CustomerSalesDecisionOutput{
		Decision: decision,
		Usage: ports.CustomerSalesUsageTelemetry{
			Provider:      modelInfo.Provider,
			Model:         modelInfo.Model,
			InputTokens:   normalized.Usage.PromptTokens,
			CachedTokens:  normalized.Usage.CachedTokens,
			OutputTokens:  normalized.Usage.CompletionTokens,
			LatencyMs:     latencyMs,
			ModelRequests: len(modelInfo.Trace.Attempts),
		},
		LatencyMs: latencyMs,
	}, nil
}

// Summarize is a separate non-tool call to the same configured gateway.
// No capability can be selected or executed during memory compression.
func (c *NativeCustomerSalesAdapter) Summarize(ctx context.Context, systemInstruction, transcript string) (string, ports.CustomerSalesUsageTelemetry, error) {
	if c == nil || c.client == nil || strings.TrimSpace(systemInstruction) == "" || strings.TrimSpace(transcript) == "" {
		return "", ports.CustomerSalesUsageTelemetry{}, errors.New("summary requires a configured model and content")
	}
	if len([]rune(systemInstruction))+len([]rune(transcript)) > c.maxInputCharacters {
		return "", ports.CustomerSalesUsageTelemetry{}, errors.New("summary exceeds provider context budget")
	}
	outputLimit := c.maxOutputTokens
	if outputLimit > 1200 {
		outputLimit = 1200
	}
	request := chatCompletionRequest{Model: c.model, Messages: []chatCompletionMessage{
		{Role: "system", Content: systemInstruction}, {Role: "user", Content: transcript},
	}, MaxTokens: outputLimit, Stream: false}
	started := time.Now()
	raw, modelInfo, err := c.client.Complete(ctx, request)
	if err != nil {
		return "", ports.CustomerSalesUsageTelemetry{}, err
	}
	normalized, err := normalizeNativeResponse(raw)
	if err != nil {
		return "", ports.CustomerSalesUsageTelemetry{}, err
	}
	if len(normalized.FunctionCalls) != 0 || strings.TrimSpace(normalized.Text) == "" {
		return "", ports.CustomerSalesUsageTelemetry{}, errors.New("summary must be plain text without tool calls")
	}
	if int64(normalized.Usage.PromptTokens) != modelInfo.InputTokens ||
		int64(normalized.Usage.CompletionTokens) != modelInfo.OutputTokens ||
		int64(normalized.Usage.CachedTokens) != modelInfo.CachedTokens ||
		len(modelInfo.Trace.Attempts) < 1 {
		return "", ports.CustomerSalesUsageTelemetry{}, errors.New("summary provider usage mismatch")
	}
	return strings.TrimSpace(normalized.Text), ports.CustomerSalesUsageTelemetry{
		Provider: modelInfo.Provider, Model: modelInfo.Model, InputTokens: normalized.Usage.PromptTokens,
		OutputTokens: normalized.Usage.CompletionTokens, CachedTokens: normalized.Usage.CachedTokens,
		ModelRequests: len(modelInfo.Trace.Attempts), LatencyMs: time.Since(started).Milliseconds(),
	}, nil
}

var _ ports.ConversationSummaryPort = (*NativeCustomerSalesAdapter)(nil)
var _ ports.CustomerSalesDecisionPort = (*NativeCustomerSalesAdapter)(nil)

// summarizeDecisionResponseShape exposes only structural diagnostics.
func summarizeDecisionResponseShape(resp normalizedNativeResponse) string {
	parts := []string{fmt.Sprintf("tool_calls=%d text_present=%t", len(resp.FunctionCalls), resp.Text != "")}
	for i, call := range resp.FunctionCalls {
		parts = append(parts, fmt.Sprintf("call[%d]={id_present=%t name=%q arguments_bytes=%d}", i, strings.TrimSpace(call.ID) != "", call.Name, len(call.Arguments)))
	}
	return strings.Join(parts, " ")
}

package gemini

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
// capability parser from Gemini. No provider may execute Mujeeb capabilities.
// This lives beside Gemini until the rest of the AI execution ports are migrated.
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
	// This is a stateless, non-executing tool selection request. Neither model
	// response nor tool-call ID is used as proof of a commercial operation.
	req := interactionRequest{
		Model: c.model, Input: userPrompt, SystemInstruction: systemInstruction,
		Store: false, Tools: tools,
		GenerationConfig: interactionGenerationConfig{MaxOutputTokens: c.maxOutputTokens, ToolChoice: "any"},
	}
	started := time.Now()
	raw, meta, err := c.client.SendInteractionWithMetadata(ctx, req)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}

	var resp liteLLMDecisionResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return ports.CustomerSalesDecisionOutput{}, fmt.Errorf("decode LiteLLM interaction: %w", err)
	}
	if resp.Status != "completed" && resp.Status != "requires_action" {
		return ports.CustomerSalesDecisionOutput{}, fmt.Errorf("LiteLLM interaction not complete: %s", resp.Status)
	}
	// LiteLLM may return outputs[] (documented bridge) or steps[] (native).
	// Never allow a second tool call to disappear during normalization.
	steps := append([]interactionStep(nil), resp.Steps...)
	for _, output := range resp.Outputs {
		if output.Type != "function_call" {
			continue
		}
		callID := output.ID
		if callID == "" {
			callID = output.CallID
		}
		args := output.Arguments
		if len(args) > 0 && args[0] == '"' {
			var argumentsString string
			if err := json.Unmarshal(args, &argumentsString); err != nil {
				return ports.CustomerSalesDecisionOutput{}, fmt.Errorf("decode LiteLLM tool arguments: %w", err)
			}
			args = json.RawMessage(argumentsString)
		}
		steps = append(steps, interactionStep{
			ID: callID, Name: output.Name, Type: output.Type, Arguments: args,
		})
	}
	decision, err := parseCustomerSalesCapability(interactionResponse{Steps: steps}, tools)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}

	latencyMs := time.Since(started).Milliseconds()
	return ports.CustomerSalesDecisionOutput{
		Decision:          decision,
		GeminiInteraction: ports.GeminiInteractionContext{Store: false},
		Usage: ports.CustomerSalesUsageTelemetry{
			Provider: "litellm", InputTokens: resp.Usage.TotalInputTokens,
			CachedTokens: resp.Usage.TotalCachedTokens, OutputTokens: resp.Usage.TotalOutputTokens,
			Model: c.model, LatencyMs: latencyMs, ModelRequests: 1,
			GatewayCallID: meta.CallID, GatewayDeploymentID: meta.DeploymentID,
			GatewayCostUSD: meta.CostUSD, GatewayCostKnown: meta.HasCost,
		},
		LatencyMs: latencyMs,
	}, nil
}

type liteLLMDecisionResponse struct {
	Status  string            `json:"status"`
	Steps   []interactionStep `json:"steps"`
	Outputs []struct {
		Type      string          `json:"type"`
		ID        string          `json:"id"`
		CallID    string          `json:"call_id"`
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"outputs"`
	Usage interactionUsage `json:"usage"`
}

var _ ports.CustomerSalesDecisionPort = (*LiteLLMCustomerSalesAdapter)(nil)

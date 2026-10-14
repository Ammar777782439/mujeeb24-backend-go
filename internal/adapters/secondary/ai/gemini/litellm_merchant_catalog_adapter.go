package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/litellm"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// LiteLLMMerchantCatalogAuthoringAdapter uses stateless Chat Completions through
// the gateway. Every assistant tool call and server-executed tool result is
// replayed explicitly; no Google previous_interaction_id is ever reused.
type LiteLLMMerchantCatalogAuthoringAdapter struct {
	client          *litellm.Client
	model           string
	maxOutputTokens int
	maxRequestBytes int

	usageReporter func(context.Context, string, string, ports.CustomerSalesUsageTelemetry) error
}

const (
	maxMerchantToolRounds    = 12
	maxMerchantToolCalls     = 32
	maxMerchantCallsPerRound = 8
)

func NewLiteLLMMerchantCatalogAuthoringAdapter(client *litellm.Client, model string, maxOutputTokens, maxRequestBytes int) (*LiteLLMMerchantCatalogAuthoringAdapter, error) {
	if client == nil || strings.TrimSpace(model) == "" || maxOutputTokens <= 0 || maxRequestBytes <= 0 {
		return nil, errors.New("LiteLLM merchant catalog requires gateway, model, and positive limits")
	}
	return &LiteLLMMerchantCatalogAuthoringAdapter{client: client, model: model, maxOutputTokens: maxOutputTokens, maxRequestBytes: maxRequestBytes}, nil
}

// SetUsageReporter persists one native gateway charge for every successful
// Chat Completions round, including tool rounds that precede the proposal.
func (a *LiteLLMMerchantCatalogAuthoringAdapter) SetUsageReporter(report func(context.Context, string, string, ports.CustomerSalesUsageTelemetry) error) {
	if a != nil {
		a.usageReporter = report
	}
}

type merchantChatFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type merchantChatCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function merchantChatFunction `json:"function"`
}
type merchantChatMessage struct {
	Role       string             `json:"role"`
	Content    any                `json:"content,omitempty"`
	ToolCalls  []merchantChatCall `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
}
type merchantChatRequest struct {
	Model          string                `json:"model"`
	Messages       []merchantChatMessage `json:"messages"`
	Tools          []map[string]any      `json:"tools,omitempty"`
	ToolChoice     string                `json:"tool_choice,omitempty"`
	ResponseFormat any                   `json:"response_format"`
	MaxTokens      int                   `json:"max_tokens"`
	Stream         bool                  `json:"stream"`
}
type merchantChatResponse struct {
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role      string             `json:"role"`
			Content   json.RawMessage    `json:"content"`
			ToolCalls []merchantChatCall `json:"tool_calls"`
			Refusal   *string            `json:"refusal"`
		} `json:"message"`
	} `json:"choices"`
}

func (a *LiteLLMMerchantCatalogAuthoringAdapter) Propose(ctx context.Context, input merchantcatalogai.MerchantCatalogAuthoringInput) (merchantcatalogai.Proposal, error) {
	if a == nil || a.client == nil || strings.TrimSpace(input.Message) == "" ||
		strings.TrimSpace(input.BusinessID) == "" || strings.TrimSpace(input.PrincipalID) == "" {
		return merchantcatalogai.Proposal{}, errors.New("merchant catalog gateway input is incomplete")
	}
	system, contextualInput, err := buildMerchantCatalogAuthoringPrompt(input)
	if err != nil {
		return merchantcatalogai.Proposal{}, err
	}

	req := merchantChatRequest{
		Model:    a.model,
		Messages: []merchantChatMessage{{Role: "system", Content: system}, {Role: "user", Content: contextualInput}},
		ResponseFormat: map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": "merchant_catalog_proposal", "strict": true, "schema": merchantCatalogProposalSchema()},
		},
		MaxTokens: a.maxOutputTokens,
		Stream:    false,
	}
	allowed := map[string]struct{}{}
	if input.Capabilities != nil {
		for _, def := range merchantCatalogInteractionTools(input.Capabilities) {
			if def.Name == "" {
				return merchantcatalogai.Proposal{}, errors.New("unnamed merchant discovery tool")
			}
			if _, exists := allowed[def.Name]; exists {
				return merchantcatalogai.Proposal{}, errors.New("duplicate merchant discovery tool")
			}
			allowed[def.Name] = struct{}{}
			req.Tools = append(req.Tools, map[string]any{
				"type":     "function",
				"function": map[string]any{"name": def.Name, "description": def.Description, "parameters": def.Parameters},
			})
		}
	}
	if len(req.Tools) > 0 {
		req.ToolChoice = "auto"
	}

	totalCalls := 0
	seenCalls := map[string]struct{}{}
	for round := 0; round < maxMerchantToolRounds; round++ {
		if err := ctx.Err(); err != nil {
			return merchantcatalogai.Proposal{}, err
		}
		body, err := json.Marshal(req)
		if err != nil {
			return merchantcatalogai.Proposal{}, fmt.Errorf("marshal merchant catalog request: %w", err)
		}
		if len(body) > a.maxRequestBytes {
			return merchantcatalogai.Proposal{}, fmt.Errorf("merchant catalog request exceeds context byte limit: %d > %d", len(body), a.maxRequestBytes)
		}
		roundCtx := litellm.WithSpendAttribution(ctx, input.BusinessID, input.SessionID, "merchant_authoring")
		raw, meta, err := a.client.SendChatCompletionWithMetadata(roundCtx, req)
		if err != nil {
			return merchantcatalogai.Proposal{}, fmt.Errorf("merchant catalog gateway request: %w", err)
		}
		var response merchantChatResponse
		if err := json.Unmarshal(raw, &response); err != nil {
			return merchantcatalogai.Proposal{}, errors.New("merchant catalog gateway returned invalid chat schema")
		}
		if a.usageReporter != nil {
			tokens := response.Usage.PromptTokensDetails.CachedTokens
			if tokens > response.Usage.PromptTokens {
				tokens = response.Usage.PromptTokens
			}
			usage := ports.CustomerSalesUsageTelemetry{
				Provider: "litellm", Model: a.model,
				InputTokens:  response.Usage.PromptTokens,
				CachedTokens: tokens, OutputTokens: response.Usage.CompletionTokens,
				ModelRequests: 1,
				GatewayCallID: meta.CallID, GatewayDeploymentID: meta.DeploymentID,
				GatewayCostUSD: meta.CostUSD, GatewayCostKnown: meta.HasCost,
			}
			if err := a.usageReporter(ctx, input.BusinessID, input.SessionID, usage); err != nil {
				return merchantcatalogai.Proposal{}, fmt.Errorf("record merchant gateway spend: %w", err)
			}
		}
		if len(response.Choices) != 1 {
			return merchantcatalogai.Proposal{}, errors.New("merchant catalog requires exactly one model choice")
		}
		choice := response.Choices[0]
		if choice.Message.Role != "assistant" || choice.Message.Refusal != nil {
			return merchantcatalogai.Proposal{}, errors.New("merchant catalog model returned refusal or unexpected role")
		}
		if len(choice.Message.ToolCalls) > 0 {
			if choice.FinishReason != "tool_calls" {
				return merchantcatalogai.Proposal{}, errors.New("merchant tool calls had invalid completion reason")
			}
			if len(choice.Message.ToolCalls) > maxMerchantCallsPerRound || totalCalls+len(choice.Message.ToolCalls) > maxMerchantToolCalls {
				return merchantcatalogai.Proposal{}, errors.New("merchant discovery call budget exceeded")
			}
			if input.Capabilities == nil {
				return merchantcatalogai.Proposal{}, errors.New("merchant catalog read-only capabilities are unavailable")
			}
			var text string
			if len(choice.Message.Content) > 0 && string(choice.Message.Content) != "null" {
				if err := json.Unmarshal(choice.Message.Content, &text); err != nil || strings.TrimSpace(text) != "" {
					return merchantcatalogai.Proposal{}, errors.New("merchant model mixed tool calls with unverified content")
				}
			}
			// Verify the entire batch before executing its first read.
			for _, call := range choice.Message.ToolCalls {
				if call.Type != "function" || call.ID == "" || call.Function.Name == "" {
					return merchantcatalogai.Proposal{}, errors.New("invalid merchant catalog tool call")
				}
				if _, ok := allowed[call.Function.Name]; !ok {
					return merchantcatalogai.Proposal{}, errors.New("merchant model requested an unauthorized tool")
				}
				if _, duplicate := seenCalls[call.ID]; duplicate {
					return merchantcatalogai.Proposal{}, errors.New("duplicate merchant tool call ID")
				}
				seenCalls[call.ID] = struct{}{}
				var args map[string]json.RawMessage
				if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil || args == nil {
					return merchantcatalogai.Proposal{}, errors.New("merchant tool arguments must be a JSON object")
				}
			}
			req.Messages = append(req.Messages, merchantChatMessage{Role: "assistant", ToolCalls: choice.Message.ToolCalls})
			for _, call := range choice.Message.ToolCalls {
				result, err := input.Capabilities.Execute(ctx, merchantcatalogai.MerchantCatalogDiscoveryExecutionContext{
					BusinessID: input.BusinessID, ConversationID: input.SessionID, PrincipalID: input.PrincipalID,
				}, call.Function.Name, []byte(call.Function.Arguments))
				if err != nil {
					return merchantcatalogai.Proposal{}, fmt.Errorf("merchant catalog read tool %s: %w", call.Function.Name, err)
				}
				encoded, err := json.Marshal(result.Data)
				if err != nil {
					return merchantcatalogai.Proposal{}, errors.New("unable to serialize merchant read-only tool result")
				}
				req.Messages = append(req.Messages, merchantChatMessage{Role: "tool", ToolCallID: call.ID, Content: string(encoded)})
			}
			totalCalls += len(choice.Message.ToolCalls)
			continue
		}
		if choice.FinishReason != "stop" {
			return merchantcatalogai.Proposal{}, errors.New("merchant catalog model did not finish proposal")
		}
		var content string
		if err := json.Unmarshal(choice.Message.Content, &content); err != nil || strings.TrimSpace(content) == "" {
			return merchantcatalogai.Proposal{}, errors.New("merchant catalog model returned no structured proposal")
		}
		// The unchanged canonical parser rejects unknown fields and trailing data.
		proposal, err := parseMerchantCatalogProposal(content)
		if err != nil {
			return merchantcatalogai.Proposal{}, err
		}
		proposal = proposal.Normalize()
		if validator, ok := input.Capabilities.(interface {
			ValidateProposalReferences(merchantcatalogai.Proposal) error
		}); ok {
			if err := validator.ValidateProposalReferences(proposal); err != nil {
				return merchantcatalogai.Proposal{}, fmt.Errorf("validate merchant catalog proposal references: %w", err)
			}
		}
		if err := proposal.Validate(); err != nil {
			return merchantcatalogai.Proposal{}, fmt.Errorf("validate merchant catalog proposal: %w", err)
		}
		return proposal, nil
	}
	return merchantcatalogai.Proposal{}, errors.New("merchant catalog maximum model rounds exceeded")
}

var _ merchantcatalogai.MerchantCatalogAuthoringPort = (*LiteLLMMerchantCatalogAuthoringAdapter)(nil)

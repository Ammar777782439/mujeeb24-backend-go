package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/litellm"
)

// chatCompletionTools translates the existing bounded Mujeeb capabilities into
// OpenAI-compatible function definitions without changing their schemas.
func chatCompletionTools(tools []customerSalesTool) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": tool.Name, "description": tool.Description, "parameters": tool.Parameters,
			},
		})
	}
	return out
}

// chatCompletionPayload is stateless by design: Mujeeb supplies the full
// trusted context per request. No provider-side conversation ID is required.
func chatCompletionPayload(req interactionRequest) (map[string]any, error) {
	if strings.TrimSpace(req.Model) == "" || strings.TrimSpace(req.Input) == "" {
		return nil, errors.New("Chat Completions requires model and user input")
	}
	messages := []map[string]string{}
	if strings.TrimSpace(req.SystemInstruction) != "" {
		messages = append(messages, map[string]string{"role": "system", "content": req.SystemInstruction})
	}
	messages = append(messages, map[string]string{"role": "user", "content": req.Input})
	out := map[string]any{
		"model": req.Model, "messages": messages, "stream": false,
		"max_tokens": req.GenerationConfig.MaxOutputTokens,
	}
	if req.GenerationConfig.MaxOutputTokens <= 0 {
		return nil, errors.New("Chat Completions output token limit is required")
	}
	if len(req.Tools) > 0 {
		out["tools"] = chatCompletionTools(req.Tools)
		out["parallel_tool_calls"] = false
		switch req.GenerationConfig.ToolChoice {
		case "any", "required":
			out["tool_choice"] = "required"
		case "auto", "":
			out["tool_choice"] = "auto"
		default:
			return nil, errors.New("unsupported Chat Completions tool choice")
		}
	} else if req.GenerationConfig.ToolChoice != "" {
		return nil, errors.New("Chat Completions cannot require tools without declarations")
	}
	if req.ResponseFormat != nil {
		if req.ResponseFormat.Type != "text" || req.ResponseFormat.MimeType != "application/json" || req.ResponseFormat.Schema == nil {
			return nil, errors.New("unsupported Chat Completions structured response")
		}
		out["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name": "mujeeb_catalog_result",
				"strict": false,
				"schema": req.ResponseFormat.Schema,
			},
		}
	}
	return out, nil
}

// parseChatCompletionResponse maps an OpenAI-compatible response to the
// existing provider-neutral decision boundary. Reject unsafe or ambiguous
// replies instead of silently selecting a choice or dropping extra calls.
func parseChatCompletionResponse(raw []byte) (json.RawMessage, error) {
	var response struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message struct {
				Role string `json:"role"`
				Content json.RawMessage `json:"content"`
				Refusal *string `json:"refusal"`
				ToolCalls []struct {
					ID string `json:"id"`
					Type string `json:"type"`
					Function struct {
						Name string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens int `json:"total_tokens"`
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, errors.New("invalid LiteLLM Chat Completions response")
	}
	if len(response.Choices) != 1 {
		return nil, errors.New("LiteLLM must return exactly one chat choice")
	}
	choice := response.Choices[0]
	if choice.Message.Role != "assistant" || choice.Message.Refusal != nil {
		return nil, errors.New("LiteLLM chat refused or returned unexpected role")
	}
	var content string
	if len(choice.Message.Content) > 0 && string(choice.Message.Content) != "null" {
		if err := json.Unmarshal(choice.Message.Content, &content); err != nil {
			return nil, errors.New("LiteLLM chat message content must be text")
		}
	}
	steps := make([]interactionStep, 0, len(choice.Message.ToolCalls)+1)
	if len(choice.Message.ToolCalls) > 0 {
		if choice.FinishReason != "tool_calls" || strings.TrimSpace(content) != "" {
			return nil, errors.New("LiteLLM returned mixed text or invalid tool finish")
		}
		for _, call := range choice.Message.ToolCalls {
			if call.Type != "function" || strings.TrimSpace(call.ID) == "" || strings.TrimSpace(call.Function.Name) == "" || !json.Valid([]byte(call.Function.Arguments)) {
				return nil, errors.New("LiteLLM returned invalid tool call")
			}
			steps = append(steps, interactionStep{
				ID: call.ID, Type: "function_call", Name: call.Function.Name,
				Arguments: json.RawMessage(call.Function.Arguments),
			})
		}
	} else {
		if choice.FinishReason != "stop" || strings.TrimSpace(content) == "" {
			return nil, errors.New("LiteLLM chat did not complete a text response")
		}
		steps = append(steps, interactionStep{Type: "model_output", Content: []interactionContent{{Type: "text", Text: content}}})
	}
	cached := response.Usage.PromptTokensDetails.CachedTokens
	if cached < 0 || cached > response.Usage.PromptTokens || response.Usage.PromptTokens < 0 || response.Usage.CompletionTokens < 0 {
		return nil, errors.New("invalid LiteLLM chat token usage")
	}
	normalized := interactionResponse{
		Status: "completed", Steps: steps,
		Usage: interactionUsage{
			TotalInputTokens: response.Usage.PromptTokens,
			TotalCachedTokens: cached,
			TotalOutputTokens: response.Usage.CompletionTokens,
			TotalTokens: response.Usage.TotalTokens,
		},
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode normalized LiteLLM chat: %w", err)
	}
	return data, nil
}

// sendGatewayChatWithMetadata is the only B2C/catalog transport. All actual
// generation requests go to /v1/chat/completions; LiteLLM handles routing,
// retries and fallbacks. Execution of a capability remains inside Mujeeb Go.
func sendGatewayChatWithMetadata(ctx context.Context, client *litellm.Client, req interactionRequest) (json.RawMessage, litellm.ResponseMetadata, error) {
	payload, err := chatCompletionPayload(req)
	if err != nil {
		return nil, litellm.ResponseMetadata{}, err
	}
	raw, meta, err := client.SendChatCompletionWithMetadata(ctx, payload)
	if err != nil {
		return nil, litellm.ResponseMetadata{}, err
	}
	normalized, err := parseChatCompletionResponse(raw)
	if err != nil {
		return nil, litellm.ResponseMetadata{}, err
	}
	return normalized, meta, nil
}

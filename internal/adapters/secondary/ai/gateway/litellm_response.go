package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// The gateway's Chat Completions choice is not an execution result.
// Validate the full envelope before exposing one proposed capability or text.
type normalizedLiteLLMResponse struct {
	Text string
	FunctionCalls []capabilityCall
	Usage chatUsage
}

func normalizeLiteLLMResponse(raw []byte) (normalizedLiteLLMResponse, error) {
	var resp struct {
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
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return normalizedLiteLLMResponse{}, errors.New("invalid LiteLLM chat response")
	}
	if len(resp.Choices) != 1 {
		return normalizedLiteLLMResponse{}, errors.New("LiteLLM requires exactly one choice")
	}
	choice := resp.Choices[0]
	if choice.Message.Role != "assistant" || choice.Message.Refusal != nil {
		return normalizedLiteLLMResponse{}, errors.New("LiteLLM returned refusal or unexpected role")
	}
	if choice.FinishReason != "stop" && choice.FinishReason != "tool_calls" {
		return normalizedLiteLLMResponse{}, fmt.Errorf("LiteLLM incomplete chat choice: %s", choice.FinishReason)
	}
	out := normalizedLiteLLMResponse{Usage: chatUsage{
		PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens,
		CachedTokens: resp.Usage.PromptTokensDetails.CachedTokens,
	}}
	if out.Usage.PromptTokens < 0 || out.Usage.CompletionTokens < 0 ||
		out.Usage.CachedTokens < 0 || out.Usage.CachedTokens > out.Usage.PromptTokens {
		return normalizedLiteLLMResponse{}, errors.New("LiteLLM returned invalid token usage")
	}
	if len(choice.Message.Content) > 0 && !bytes.Equal(bytes.TrimSpace(choice.Message.Content), []byte("null")) {
		if err := json.Unmarshal(choice.Message.Content, &out.Text); err != nil {
			return normalizedLiteLLMResponse{}, errors.New("LiteLLM returned non-text message content")
		}
	}
	out.Text = strings.TrimSpace(out.Text)
	if choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) == 0 ||
		choice.FinishReason == "stop" && len(choice.Message.ToolCalls) != 0 {
		return normalizedLiteLLMResponse{}, errors.New("LiteLLM finish reason disagrees with tool calls")
	}
	if len(choice.Message.ToolCalls) > 1 {
		return normalizedLiteLLMResponse{}, errors.New("LiteLLM returned multiple capability calls")
	}
	if len(choice.Message.ToolCalls) != 0 && out.Text != "" {
		return normalizedLiteLLMResponse{}, errors.New("LiteLLM mixed tool call and plain text")
	}
	for _, call := range choice.Message.ToolCalls {
		if call.Type != "function" || strings.TrimSpace(call.ID) == "" ||
			strings.TrimSpace(call.Function.Name) == "" || !json.Valid([]byte(call.Function.Arguments)) {
			return normalizedLiteLLMResponse{}, errors.New("invalid LiteLLM function call")
		}
		out.FunctionCalls = append(out.FunctionCalls, capabilityCall{ID: call.ID, Name: call.Function.Name, Arguments: json.RawMessage(call.Function.Arguments)})
	}
	return out, nil
}

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantassist"
)

// MerchantAssistNativeTools uses the existing native provider router and
// standard Chat Completions function calls, not JSON-planning prompts.
// Merchant Catalog AI's authoring route remains fully independent.
type MerchantAssistNativeTools struct {
	Client *native.CompletionClient
	Model string
}

type merchantAssistNativeCall struct {
	ID string `json:"id"`
	Type string `json:"type"`
	Function struct {
		Name string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
	ThoughtSignature string `json:"thought_signature,omitempty"`
}
type merchantAssistNativeMessage struct {
	Role string `json:"role"`
	Content any `json:"content,omitempty"`
	ToolCalls []merchantAssistNativeCall `json:"tool_calls,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}
type merchantAssistNativeResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message struct {
			Role string `json:"role"`
			Content json.RawMessage `json:"content"`
			ToolCalls []merchantAssistNativeCall `json:"tool_calls"`
			Refusal *string `json:"refusal"`
		} `json:"message"`
	} `json:"choices"`
}

func (m *MerchantAssistNativeTools) CompleteTools(ctx context.Context, history []merchantassist.ToolMessage, defs []merchantassist.ToolDefinition, pinned string) (merchantassist.ToolCompletion, error) {
	var zero merchantassist.ToolCompletion
	if m == nil || m.Client == nil || strings.TrimSpace(m.Model) == "" {
		return zero, errors.New("merchant assist native completion is not configured")
	}
	messages := make([]merchantAssistNativeMessage, 0, len(history))
	for _, h := range history {
		if h.Role != "user" && h.Role != "assistant" && h.Role != "tool" {
			return zero, errors.New("unsupported merchant assistant role")
		}
		entry := merchantAssistNativeMessage{Role: h.Role}
		if h.Role == "tool" {
			if h.ToolCallID == "" { return zero, errors.New("tool result missing call ID") }
			entry.ToolCallID, entry.Content = h.ToolCallID, h.Content
		} else if h.Role == "user" {
			entry.Content = h.Content
		} else if len(h.ToolCalls) > 0 {
			entry.ToolCalls = make([]merchantAssistNativeCall, 0, len(h.ToolCalls))
			for _, t := range h.ToolCalls {
				var call merchantAssistNativeCall
				call.ID, call.Type, call.Function.Name = t.ID, "function", t.Name
				call.Function.Arguments = string(t.Arguments)
				call.ThoughtSignature = t.ThoughtSignature
				entry.ToolCalls = append(entry.ToolCalls, call)
			}
		} else {
			entry.Content = h.Content
		}
		messages = append(messages, entry)
	}
	request := map[string]any{"model": m.Model, "messages": messages, "max_tokens": 1000, "stream": false}
	if len(defs) > 0 {
		tools := make([]map[string]any, 0, len(defs))
		for _, def := range defs {
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
				"name": def.Name, "description": def.Description, "parameters": def.Parameters, "strict": true,
			}})
		}
		request["tools"] = tools
		request["tool_choice"] = "auto"
		request["parallel_tool_calls"] = false
	}
	body, err := json.Marshal(request)
	if err != nil || len(body) > 90000 { return zero, errors.New("merchant assist request exceeds context limit") }
	raw, info, err := m.Client.CompletePinned(ctx, request, pinned)
	if err != nil { return zero, err }
	var answer merchantAssistNativeResponse
	if err := json.Unmarshal(raw, &answer); err != nil || len(answer.Choices) != 1 {
		return zero, errors.New("invalid merchant assistant native response")
	}
	choice := answer.Choices[0]
	if choice.Message.Role != "assistant" || choice.Message.Refusal != nil {
		return zero, errors.New("merchant assistant returned refusal or invalid role")
	}
	reply := merchantassist.ToolCompletion{FinishReason: choice.FinishReason, Provider: info.Provider}
	if len(choice.Message.ToolCalls) > 0 {
		if choice.FinishReason != "tool_calls" || len(choice.Message.ToolCalls) != 1 {
			return zero, errors.New("invalid merchant assistant tool call batch")
		}
		for _, call := range choice.Message.ToolCalls {
			if call.ID == "" || call.Type != "function" || call.Function.Name == "" || call.Function.Arguments == "" {
				return zero, errors.New("invalid merchant assistant function call")
			}
			reply.Calls = append(reply.Calls, merchantassist.ToolCall{
				ID: call.ID, Name: call.Function.Name, Arguments: json.RawMessage(call.Function.Arguments), ThoughtSignature: call.ThoughtSignature,
			})
		}
		return reply, nil
	}
	if choice.FinishReason != "stop" { return zero, errors.New("merchant assistant did not complete") }
	if err := json.Unmarshal(choice.Message.Content, &reply.Content); err != nil {
		return zero, errors.New("merchant assistant final content must be text")
	}
	reply.Content = strings.TrimSpace(reply.Content)
	if reply.Content == "" || len(reply.Content) > 4000 { return zero, errors.New("merchant assistant final text is empty or too long") }
	return reply, nil
}

var _ merchantassist.ToolModel = (*MerchantAssistNativeTools)(nil)

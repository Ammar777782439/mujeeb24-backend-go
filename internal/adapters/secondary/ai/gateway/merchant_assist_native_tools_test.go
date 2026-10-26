package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantassist"
)

func TestMerchantAssistNativeToolsUsesCanonicalToolCallsAndReplaysSignature(t *testing.T) {
	provider := &nativeContractProvider{name: "gemini"}
	var attempts int
	provider.call = func(req native.Request) (native.Response, error) {
		attempts++
		var body struct {
			Model             string `json:"model"`
			ToolChoice        string `json:"tool_choice"`
			ParallelToolCalls bool   `json:"parallel_tool_calls"`
			Tools             []struct {
				Type     string `json:"type"`
				Function struct {
					Name       string         `json:"name"`
					Strict     bool           `json:"strict"`
					Parameters map[string]any `json:"parameters"`
				} `json:"function"`
			} `json:"tools"`
			Messages []struct {
				Role       string `json:"role"`
				Content    string `json:"content"`
				ToolCallID string `json:"tool_call_id"`
				ToolCalls  []struct {
					ID               string `json:"id"`
					ThoughtSignature string `json:"thought_signature"`
					Function         struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(req.Payload, &body); err != nil {
			return native.Response{}, err
		}
		if body.Model != "mujeeb-merchant" || body.ToolChoice != "auto" ||
			body.ParallelToolCalls || len(body.Tools) != 1 ||
			body.Tools[0].Type != "function" || body.Tools[0].Function.Name != "find_customers" ||
			!body.Tools[0].Function.Strict || body.Tools[0].Function.Parameters["additionalProperties"] != false {
			t.Fatalf("native read tool contract missing or unsafe: %+v", body)
		}
		if attempts == 1 {
			if len(body.Messages) != 1 || body.Messages[0].Role != "user" {
				t.Fatalf("unexpected initial messages: %+v", body.Messages)
			}
			raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
				"finish_reason": "tool_calls", "message": map[string]any{
					"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{
						"id": "read-1", "type": "function", "thought_signature": "provider-signature",
						"function": map[string]any{"name": "find_customers", "arguments": `{"search":""}`},
					}},
				},
			}}})
			return native.Response{Payload: raw, Model: "gemini-test", InputTokens: 12, OutputTokens: 9}, nil
		}
		if attempts != 2 || len(body.Messages) != 3 || body.Messages[1].Role != "assistant" ||
			len(body.Messages[1].ToolCalls) != 1 || body.Messages[1].ToolCalls[0].ThoughtSignature != "provider-signature" ||
			body.Messages[2].Role != "tool" || body.Messages[2].ToolCallID != "read-1" ||
			!strings.Contains(body.Messages[2].Content, "total") || req.PinnedProvider != "gemini" {
			t.Fatalf("tool history or pinned provider was lost: %+v", body.Messages)
		}
		raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
			"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": "إجمالي العملاء 25 عميلًا."},
		}}})
		return native.Response{Payload: raw, Model: "gemini-test", InputTokens: 52, OutputTokens: 20}, nil
	}
	model := &MerchantAssistNativeTools{Client: nativeContractClient(t, provider), Model: "mujeeb-merchant"}
	definitions := []merchantassist.ToolDefinition{{
		Name: "find_customers", Description: "List business-scoped customers",
		Parameters: map[string]any{
			"type": "object", "additionalProperties": false,
			"required":   []string{"search"},
			"properties": map[string]any{"search": map[string]any{"type": "string"}},
		},
	}}
	history := []merchantassist.ToolMessage{{Role: "user", Content: "كم إجمالي العملاء؟"}}
	first, err := model.CompleteTools(context.Background(), history, definitions, "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Provider != "gemini" || first.FinishReason != "tool_calls" || len(first.Calls) != 1 ||
		first.Calls[0].Name != "find_customers" || first.Calls[0].ThoughtSignature != "provider-signature" {
		t.Fatalf("lost structured model tool call: %+v", first)
	}
	history = append(history, merchantassist.ToolMessage{Role: "assistant", ToolCalls: first.Calls})
	history = append(history, merchantassist.ToolMessage{Role: "tool", ToolCallID: first.Calls[0].ID, Content: `{"total":25,"pagination_exhausted":true}`})
	second, err := model.CompleteTools(context.Background(), history, definitions, first.Provider)
	if err != nil {
		t.Fatal(err)
	}
	if second.Content != "إجمالي العملاء 25 عميلًا." || second.FinishReason != "stop" || attempts != 2 {
		t.Fatalf("unexpected completion: %+v attempts=%d", second, attempts)
	}
}

func TestMerchantAssistNativeToolsKeepsElevenValidCalls(t *testing.T) {
	provider := &nativeContractProvider{name: "deepseek"}
	provider.call = func(req native.Request) (native.Response, error) {
		calls := make([]map[string]any, 0, 11)
		for i := 0; i < 11; i++ {
			calls = append(calls, map[string]any{
				"id":   fmt.Sprintf("id-%d", i),
				"type": "function",
				"function": map[string]any{
					"name":      "find_customers",
					"arguments": fmt.Sprintf(`{"search":"customer-%d"}`, i),
				},
			})
		}
		payload, err := json.Marshal(map[string]any{"choices": []any{
			map[string]any{"finish_reason": "tool_calls", "message": map[string]any{
				"role": "assistant", "content": nil, "tool_calls": calls,
			}},
		}})
		if err != nil {
			return native.Response{}, err
		}
		return native.Response{Payload: payload, Model: "deepseek", InputTokens: 20, OutputTokens: 55}, nil
	}
	model := &MerchantAssistNativeTools{Client: nativeContractClient(t, provider), Model: "mujeeb-merchant"}
	definitions := []merchantassist.ToolDefinition{{
		Name:        "find_customers",
		Description: "Read customers",
		Parameters: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           map[string]any{"search": map[string]any{"type": "string"}},
			"required":             []string{"search"},
		},
	}}
	reply, err := model.CompleteTools(context.Background(), []merchantassist.ToolMessage{
		{Role: "user", Content: "ابحث عن العملاء"},
	}, definitions, "")
	if err != nil {
		t.Fatalf("provider's eleven valid calls were rejected: %v", err)
	}
	if len(reply.Calls) != 11 || reply.FinishReason != "tool_calls" {
		t.Fatalf("native adapter dropped tool calls: got %d, finish=%s", len(reply.Calls), reply.FinishReason)
	}
	for i, call := range reply.Calls {
		if call.ID != fmt.Sprintf("id-%d", i) || call.Name != "find_customers" {
			t.Fatalf("call order or identity lost at index %d: %+v", i, call)
		}
	}
}

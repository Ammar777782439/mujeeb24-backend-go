package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type chatGatewayTestRequest struct {
	Model string `json:"model"`
	Messages []struct {
		Role string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Tools []struct {
		Type string `json:"type"`
		Function struct {
			Name string `json:"name"`
			Description string `json:"description"`
			Parameters map[string]any `json:"parameters"`
		} `json:"function"`
	} `json:"tools"`
	ToolChoice string `json:"tool_choice"`
	ParallelToolCalls bool `json:"parallel_tool_calls"`
	ResponseFormat *struct {
		Type string `json:"type"`
		JSONSchema struct {
			Name string `json:"name"`
			Schema map[string]any `json:"schema"`
		} `json:"json_schema"`
	} `json:"response_format"`
	MaxTokens int `json:"max_tokens"`
}

func (r chatGatewayTestRequest) UserContent() string {
	for _, m := range r.Messages {
		if m.Role == "user" {
			return m.Content
		}
	}
	return ""
}

func (r chatGatewayTestRequest) SystemContent() string {
	for _, m := range r.Messages {
		if m.Role == "system" {
			return m.Content
		}
	}
	return ""
}

// Legacy test fixtures express provider-independent model decisions. This
// converts those fixtures to the real Chat Completions response envelope.
func writeChatTestResponse(w http.ResponseWriter, fixture any) {
	raw, err := json.Marshal(fixture)
	if err != nil { panic(err) }
	var legacy struct {
		Status string `json:"status"`
		Outputs []struct {
			Type string `json:"type"`
			ID string `json:"id"`
			Name string `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
			Text string `json:"text"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"outputs"`
		Usage struct {
			Input int `json:"total_input_tokens"`
			Output int `json:"total_output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &legacy); err != nil { panic(err) }
	calls := []map[string]any{}
	textContent := ""
	for _, output := range legacy.Outputs {
		if output.Type == "function_call" {
			args := string(output.Arguments)
			calls = append(calls, map[string]any{
				"id": output.ID, "type": "function",
				"function": map[string]any{"name": output.Name, "arguments": args},
			})
		} else {
			textContent += output.Text
			for _, part := range output.Content { textContent += part.Text }
		}
	}
	finish := "stop"
	if len(calls) > 0 { finish = "tool_calls" }
	if legacy.Status == "incomplete" { finish = "length" }
	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []any{map[string]any{
			"finish_reason": finish,
			"message": map[string]any{
				"role": "assistant", "content": textContent,
				"tool_calls": calls,
			},
		}},
		"usage": map[string]any{
			"prompt_tokens": legacy.Usage.Input,
			"completion_tokens": legacy.Usage.Output,
		},
	})
}

func TestChatCompletionTransportAndStrictResponses(t *testing.T) {
	req := interactionRequest{
		Model: "mujeeb-customer", Input: "أريد حجزًا",
		SystemInstruction: "Mujeeb customer contract",
		Tools: []customerSalesTool{{Type:"function",Name:"ask_customer",Description:"Ask missing information",Parameters:map[string]any{"type":"object","properties":map[string]any{"response_text":map[string]any{"type":"string"}}}}},
		GenerationConfig: interactionGenerationConfig{MaxOutputTokens:512,ToolChoice:"any"},
	}
	payload, err := chatCompletionPayload(req)
	if err != nil { t.Fatal(err) }
	if payload["tool_choice"] != "required" || payload["parallel_tool_calls"] != false || payload["model"] != "mujeeb-customer" {
		t.Fatalf("unsafe chat request: %v",payload)
	}
	tools := payload["tools"].([]map[string]any)
	if tools[0]["function"].(map[string]any)["name"] != "ask_customer" {
		t.Fatal("lost capability identity")
	}
	for name, data := range map[string]string{
		"parallel": `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"ask_customer","arguments":"{}"}},{"id":"b","type":"function","function":{"name":"ask_customer","arguments":"{}"}}]}}]}`,
		"mixed": `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"Done","tool_calls":[{"id":"a","type":"function","function":{"name":"ask_customer","arguments":"{}"}}]}}]}`,
		"length": `{"choices":[{"finish_reason":"length","message":{"role":"assistant","content":"truncated"}}]}`,
		"refusal": `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","refusal":"no","content":""}}]}`,
	} {
		t.Run(name,func(t *testing.T){
			result, e := parseChatCompletionResponse([]byte(data))
			if name == "parallel" {
				if e != nil {t.Fatal(e)}
				var normalized interactionResponse
				if json.Unmarshal(result,&normalized) != nil || len(normalized.Steps) != 2 {t.Fatal("dropped parallel calls")}
				if _,e = parseCustomerSalesCapability(normalized,req.Tools);e==nil {t.Fatal("parallel calls accepted")}
				return
			}
			if e == nil {t.Fatal("unsafe chat response accepted")}
		})
	}
	// Ensure JSON schema stays in response_format, not in the system prompt.
	catalog := interactionRequest{Model:"mujeeb-catalog",Input:"batch", ResponseFormat:&interactionResponseFormat{Type:"text",MimeType:"application/json",Schema:map[string]any{"type":"object"}},GenerationConfig:interactionGenerationConfig{MaxOutputTokens:256}}
	structured,e := chatCompletionPayload(catalog)
	if e != nil || structured["response_format"] == nil || strings.Contains(structured["messages"].([]map[string]string)[0]["content"],"response_format") {
		t.Fatalf("lost catalog JSON schema: %v %v",structured,e)
	}
}

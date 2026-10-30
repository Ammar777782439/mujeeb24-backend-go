package native

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func merchantFinalRequestWithToolHistory(t *testing.T, toolID string) Request {
	t.Helper()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{"type": "string"},
		},
		"required":             []string{"operation"},
		"additionalProperties": false,
	}
	canonical := map[string]any{
		"model": "merchant-catalog",
		"messages": []any{
			map[string]any{"role": "system", "content": "trusted merchant rules"},
			map[string]any{"role": "user", "content": "أضف جلكسي اس 8"},
			map[string]any{"role": "assistant", "tool_calls": []any{
				map[string]any{
					"id": "read-1", "type": "function",
					"thought_signature": "provider-private",
					"function": map[string]any{
						"name": "merchant_catalog_list_items", "arguments": `{"limit":50}`,
					},
				},
			}},
			map[string]any{
				"role": "tool", "tool_call_id": toolID,
				"content": `{"data":{"item":"Coffee"},"has_more":true,"next_cursor":"next-1"}`,
			},
		},
		"max_tokens": 2048,
		"stream":     false,
		"response_format": map[string]any{
			"type": "json_schema", "json_schema": map[string]any{
				"name": "merchant_catalog_wire", "strict": true, "schema": schema,
			},
		},
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	return Request{Payload: payload}
}

func TestDeepSeekFinalTransportDropsStaleToolRolesButRetainsReadEvidence(t *testing.T) {
	count := 0
	api, done := newFakeDeepSeek(t, func(w http.ResponseWriter, r *http.Request) {
		count++
		var wire struct {
			Messages []struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []any  `json:"tool_calls"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
			ToolChoice struct {
				Type     string `json:"type"`
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tool_choice"`
		}
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			t.Error(err)
		}
		if len(wire.Messages) != 2 || wire.Messages[0].Role != "system" ||
			wire.Messages[1].Role != "user" || len(wire.Messages[1].ToolCalls) != 0 {
			t.Errorf("stale assistant/tool call roles reached final request: %+v", wire.Messages)
		} else {
			for _, expected := range []string{
				"جلكسي اس 8", "Coffee", "read-1", "next-1", "has_more",
			} {
				if !strings.Contains(wire.Messages[1].Content, expected) {
					t.Errorf("catalog evidence lost %q", expected)
				}
			}
			if strings.Contains(wire.Messages[1].Content, "provider-private") {
				t.Error("provider-only thought signature leaked into evidence")
			}
		}
		if len(wire.Tools) != 1 ||
			wire.Tools[0].Function.Name != "submit_merchant_catalog_proposal" ||
			wire.ToolChoice.Function.Name != "submit_merchant_catalog_proposal" ||
			wire.ToolChoice.Type != "function" {
			t.Errorf("strict output tool not forced: %+v", wire.ToolChoice)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"finish_reason": "tool_calls",
				"message": map[string]any{
					"role": "assistant", "content": "",
					"tool_calls": []any{map[string]any{
						"id": "final-1", "type": "function",
						"function": map[string]any{
							"name": "submit_merchant_catalog_proposal",
							"arguments": `{"operation":"ask_merchant"}`,
						},
					}},
				},
			}},
			"usage": map[string]any{"prompt_tokens": 50, "completion_tokens": 20},
		})
	})
	defer done()
	_, err := api.Generate(context.Background(), merchantFinalRequestWithToolHistory(t, "read-1"))
	if err != nil || count != 1 {
		t.Fatalf("final read evidence isolation failed requests=%d err=%v", count, err)
	}
}

func TestDeepSeekFinalTransportRejectsUnmatchedToolHistoryBeforeHTTP(t *testing.T) {
	count := 0
	api, done := newFakeDeepSeek(t, func(w http.ResponseWriter, r *http.Request) {
		count++
		w.WriteHeader(http.StatusOK)
	})
	defer done()
	_, err := api.Generate(context.Background(), merchantFinalRequestWithToolHistory(t, "wrong-id"))
	if err == nil || count != 0 {
		t.Fatalf("malformed tool transcript reached the provider requests=%d err=%v", count, err)
	}
}

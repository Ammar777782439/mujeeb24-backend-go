package native

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestDeepSeekMerchantFinalWrongFunctionIsRejectedAndClassified(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{"type": "string"},
		},
		"required":             []string{"operation"},
		"additionalProperties": false,
	}
	format, err := json.Marshal(map[string]any{
		"type": "json_schema", "json_schema": map[string]any{
			"name": "merchant_catalog_wire", "strict": true, "schema": schema,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	api, done := newFakeDeepSeek(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req struct {
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if len(req.Tools) != 1 || req.Tools[0].Function.Name != "submit_merchant_catalog_proposal" ||
			req.ToolChoice.Type != "function" ||
			req.ToolChoice.Function.Name != "submit_merchant_catalog_proposal" {
			t.Errorf("provider final tool name not forced: %+v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"finish_reason": "tool_calls",
				"message": map[string]any{
					"role": "assistant", "content": "",
					"tool_calls": []any{map[string]any{
						"id": "call-1", "type": "function",
						"function": map[string]any{
							"name":      "merchant_catalog_list_items",
							"arguments": `{"limit":20}`,
						},
					}},
				},
			}},
			"usage": map[string]any{"prompt_tokens": 30, "completion_tokens": 15},
		})
	})
	defer done()
	_, err = api.Generate(context.Background(), deepSeekReq("", string(format)))
	var wrongName *MerchantCatalogToolNameError
	if !errors.As(err, &wrongName) || calls != 1 {
		t.Fatalf("wrong final function must be rejected and classified, calls=%d err=%v", calls, err)
	}
}

package native

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestDeepSeekMalformedMerchantFinalIsClassifiedWithoutAcceptingIt(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{"type": "string"},
			"create": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"fields":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"variants": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"offers":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required":             []string{"fields", "variants", "offers"},
				"additionalProperties": false,
			},
		},
		"required":             []string{"operation", "create"},
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
	api, done := newFakeDeepSeek(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"finish_reason": "tool_calls",
				"message": map[string]any{
					"role": "assistant", "content": "",
					"tool_calls": []any{map[string]any{
						"id": "call-1", "type": "function",
						"function": map[string]any{
							"name":      "submit_merchant_catalog_proposal",
							"arguments": `{"operation":"ask_merchant","create":{"fields":[]}}`,
						},
					}},
				},
			}},
			"usage": map[string]any{"prompt_tokens": 20, "completion_tokens": 10},
		})
	})
	defer done()
	_, err = api.Generate(context.Background(), deepSeekReq("", string(format)))
	var malformed *MerchantCatalogSchemaError
	if !errors.As(err, &malformed) {
		t.Fatalf("malformed merchant transport was not classified: %v", err)
	}
	if malformed.Cause == nil {
		t.Fatal("classified error lost its validation cause")
	}
}

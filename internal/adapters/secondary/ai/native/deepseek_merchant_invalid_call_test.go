package native

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestDeepSeekMerchantFinalClassifiesMalformedCallEnvelope(t *testing.T) {
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
	for _, tc := range []struct {
		name    string
		id      string
		kind    string
		fn      string
		args    string
		reason  string
	}{
		{name: "missing_id", kind: "function", fn: "submit_merchant_catalog_proposal", args: `{"operation":"ask_merchant"}`, reason: "missing_call_id"},
		{name: "wrong_type", id: "call-1", kind: "not_function", fn: "submit_merchant_catalog_proposal", args: `{"operation":"ask_merchant"}`, reason: "invalid_call_type"},
		{name: "missing_name", id: "call-1", kind: "function", args: `{"operation":"ask_merchant"}`, reason: "missing_function_name"},
		{name: "invalid_json", id: "call-1", kind: "function", fn: "submit_merchant_catalog_proposal", args: "{", reason: "invalid_arguments_json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			api, done := newFakeDeepSeek(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				var body struct {
					Tools []struct {
						Function struct {
							Name string `json:"name"`
						} `json:"function"`
					} `json:"tools"`
					ToolChoice struct {
						Function struct {
							Name string `json:"name"`
						} `json:"function"`
					} `json:"tool_choice"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if len(body.Tools) != 1 ||
					body.Tools[0].Function.Name != "submit_merchant_catalog_proposal" ||
					body.ToolChoice.Function.Name != "submit_merchant_catalog_proposal" {
					t.Errorf("final output tool was not forced: %+v", body)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"choices": []any{map[string]any{
						"finish_reason": "tool_calls",
						"message": map[string]any{
							"role": "assistant", "content": "",
							"tool_calls": []any{map[string]any{
								"id": tc.id, "type": tc.kind,
								"function": map[string]any{
									"name": tc.fn, "arguments": tc.args,
								},
							}},
						},
					}},
					"usage": map[string]any{"prompt_tokens": 30, "completion_tokens": 15},
				})
			})
			defer done()
			_, err := api.Generate(context.Background(), deepSeekReq("", string(format)))
			var malformed *MerchantCatalogCallEnvelopeError
			if requests != 1 || !errors.As(err, &malformed) || malformed.Reason != tc.reason {
				t.Fatalf("incorrect classification requests=%d err=%v", requests, err)
			}
		})
	}
}

func TestDeepSeekMerchantCallEnvelopeReasonDoesNotContainUserData(t *testing.T) {
	if got := merchantCatalogCallEnvelopeReason(
		"call-1", "function", "submit_merchant_catalog_proposal", "{bad-private-data",
	); got != "invalid_arguments_json" {
		t.Fatalf("unsafe diagnostic reason: %q", got)
	}
	if got := merchantCatalogCallEnvelopeReason(
		"call-1", "function", "submit_merchant_catalog_proposal", `{"operation":"ask_merchant"}`,
	); got != "" {
		t.Fatalf("valid call incorrectly rejected: %s", got)
	}
}

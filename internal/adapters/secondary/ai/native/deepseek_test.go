package native

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newFakeDeepSeek(t *testing.T, handler http.HandlerFunc) (*DeepSeek, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	api, err := NewDeepSeek(DeepSeekConfig{Model: "deepseek-flash", APIKey: "secret-key", BaseURL: srv.URL + "/beta", Timeout: time.Second})
	if err != nil {
		srv.Close()
		t.Fatal(err)
	}
	return api, srv.Close
}
func deepSeekReq(tools, format string) Request {
	body := `{"model":"mujeeb-sales","messages":[{"role":"system","content":"trusted system rules"},{"role":"user","content":"untrusted customer text"}],"max_tokens":123,"tool_choice":"required","parallel_tool_calls":false,"tools":` + tools + `}`
	if format != "" {
		body = `{"model":"mujeeb-catalog","messages":[{"role":"system","content":"trusted"},{"role":"user","content":"payload"}],"max_tokens":123,"response_format":` + format + `}`
	}
	return Request{Payload: json.RawMessage(body)}
}
func TestDeepSeekForcedToolsUseBetaNonThinkingWithoutLosingSchema(t *testing.T) {
	client, closeFn := newFakeDeepSeek(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/beta/chat/completions" || r.Header.Get("Authorization") != "Bearer secret-key" {
			t.Error("invalid direct DeepSeek endpoint/auth")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["tool_choice"] != "required" || body["thinking"].(map[string]any)["type"] != "disabled" {
			t.Error("forced non-thinking not preserved")
		}
		tools := body["tools"].([]any)
		fn := tools[0].(map[string]any)["function"].(map[string]any)
		if fn["strict"] != true || fn["name"] != "evaluate_full_catalog" {
			t.Error("strict function erased")
		}
		messages := body["messages"].([]any)
		if messages[0].(map[string]any)["role"] != "system" || messages[1].(map[string]any)["role"] != "user" {
			t.Error("trust boundaries merged")
		}
		_, _ = fmt.Fprint(w, `{"model":"deepseek-flash","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"call-a","type":"function","function":{"name":"evaluate_full_catalog","arguments":"{}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":4}}`)
	})
	defer closeFn()
	tools := `[{"type":"function","function":{"name":"evaluate_full_catalog","strict":true,"description":"fetch evidence","parameters":{"type":"object","properties":{},"required":[],"additionalProperties":false}}}]`
	got, err := client.Generate(context.Background(), deepSeekReq(tools, ""))
	if err != nil || got.Model != "deepseek-flash" || got.InputTokens != 10 || !strings.Contains(string(got.Payload), "evaluate_full_catalog") {
		t.Fatalf("result %+v error %v", got, err)
	}
}
func TestDeepSeekCatalogSchemaUsesStrictToolRatherThanWeakJSONObject(t *testing.T) {
	api, done := newFakeDeepSeek(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, bad := body["response_format"]; bad {
			t.Error("plain JSON mode must not pretend to satisfy strict schema")
		}
		fn := body["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)
		schema := fn["parameters"].(map[string]any)
		if fn["strict"] != true || schema["additionalProperties"] != false || schema["required"].([]any)[0] != "candidates" {
			t.Fatal("catalog schema weakened")
		}
		_, _ = fmt.Fprint(w, `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"x","type":"function","function":{"name":"submit_catalog_batch","arguments":"{\"candidates\":[]}"}}]}}],"usage":{"prompt_tokens":4,"completion_tokens":8}}`)
	})
	defer done()
	schema := `{"type":"json_schema","json_schema":{"name":"catalog_batch_candidates","strict":true,"schema":{"type":"object","properties":{"candidates":{"type":"array","items":{"type":"object","properties":{},"required":[],"additionalProperties":false}}},"required":["candidates"],"additionalProperties":false}}}`
	got, err := api.Generate(context.Background(), deepSeekReq("", schema))
	if err != nil || !strings.Contains(string(got.Payload), `{\"candidates\":[]}`) {
		t.Fatalf("strict batch: %s %v", got.Payload, err)
	}
}
func TestDeepSeekFailsClosedOnUnsupportedMerchantAndProviderFailures(t *testing.T) {
	called := 0
	api, done := newFakeDeepSeek(t, func(w http.ResponseWriter, r *http.Request) { called++; w.WriteHeader(503) })
	defer done()
	cases := []Request{
		{Payload: []byte(`{"model":"x","messages":[{"role":"user","content":"x"}],"max_tokens":8,"tools":[{"type":"function","function":{"name":"read","strict":true}}],"tool_choice":"auto","response_format":{"type":"json_schema","json_schema":{"name":"merchant","strict":true,"schema":{}}}}`)},
		{Payload: []byte(`{"model":"x","messages":[{"role":"user","content":"x"}],"max_tokens":8,"tools":[{"type":"function","function":{"name":"read","strict":false}}],"tool_choice":"required"}`)},
	}
	for _, req := range cases {
		if _, err := api.Generate(context.Background(), req); err == nil {
			t.Error("unsafe request accepted")
		}
	}
	if called != 0 {
		t.Fatal("unsafe requests sent to provider")
	}
	_, err := api.Generate(context.Background(), deepSeekReq(`[{"type":"function","function":{"name":"safe","strict":true,"parameters":{"type":"object","properties":{},"required":[],"additionalProperties":false}}}]`, ""))
	if !IsUnavailable(err) {
		t.Fatalf("503 not classified: %v", err)
	}
}
func TestDeepSeekRejectsTruncatedResponse(t *testing.T) {
	api, done := newFakeDeepSeek(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"choices":[{"finish_reason":"length","message":{"role":"assistant","content":"partial"}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)
	})
	defer done()
	if _, err := api.Generate(context.Background(), deepSeekReq(`[{"type":"function","function":{"name":"safe","strict":true,"parameters":{}}}]`, "")); err == nil {
		t.Fatal("truncated response accepted")
	}
}

func TestDeepSeekRejectsParallelUnknownOrMixedForcedCalls(t *testing.T) {
	tool := `[{"type":"function","function":{"name":"evaluate_full_catalog","strict":true,"parameters":{"type":"object","properties":{},"required":[],"additionalProperties":false}}}]`
	responses := []string{
		`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"evaluate_full_catalog","arguments":"{}"}},{"id":"b","type":"function","function":{"name":"evaluate_full_catalog","arguments":"{}"}}]}}]}`,
		`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"not_offered","arguments":"{}"}}]}}]}`,
		`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":"already done","tool_calls":[{"id":"a","type":"function","function":{"name":"evaluate_full_catalog","arguments":"{}"}}]}}]}`,
		`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"evaluate_full_catalog","arguments":"not JSON"}}]}}]}`,
	}
	for i, body := range responses {
		api, done := newFakeDeepSeek(t, func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, body) })
		_, err := api.Generate(context.Background(), deepSeekReq(tool, ""))
		done()
		if err == nil {
			t.Fatalf("unsafe forced function response %d accepted", i)
		}
	}
}

func TestDeepSeekRejectsSchemaViolationsBeforeAndAfterHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, argument string
		wantHTTP       int
	}{
		{name: "unexpected offer ref", argument: `{"candidates":[{"item_id":"item-1","variant_ids":[],"offer_ids":["fabricated"],"reason":"fake"}]}`, wantHTTP: 1},
		{name: "missing required", argument: `{"candidates":[{"item_id":"item-1","variant_ids":[],"offer_ids":[]}]}`, wantHTTP: 1},
		{name: "valid empty batch", argument: `{"candidates":[]}`, wantHTTP: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			api, done := newFakeDeepSeek(t, func(w http.ResponseWriter, r *http.Request) {
				count++
				resp := map[string]any{"choices": []any{map[string]any{"finish_reason": "tool_calls", "message": map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "one", "type": "function", "function": map[string]any{"name": "submit_catalog_batch", "arguments": tc.argument}}}}}}}
				_ = json.NewEncoder(w).Encode(resp)
			})
			defer done()
			format := `{"type":"json_schema","json_schema":{"name":"batch","strict":true,"schema":` + strictCatalogExample + `}}`
			_, err := api.Generate(context.Background(), deepSeekReq("", format))
			wantOK := tc.name == "valid empty batch"
			if (err == nil) != wantOK || count != tc.wantHTTP {
				t.Fatalf("success=%t http=%d err=%v", wantOK, count, err)
			}
		})
	}
}
func TestDeepSeekRefusesUnsupportedSchemaWithoutCallingProvider(t *testing.T) {
	calls := 0
	api, done := newFakeDeepSeek(t, func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(200) })
	defer done()
	unsupported := `{"type":"json_schema","json_schema":{"name":"merchant","strict":true,"schema":{"type":"object","properties":{},"required":[],"additionalProperties":false,"propertyNames":{"pattern":"^x$"}}}}`
	_, err := api.Generate(context.Background(), deepSeekReq("", unsupported))
	if err == nil || calls != 0 {
		t.Fatalf("unsupported schema silently downgraded calls=%d err=%v", calls, err)
	}
}

func TestDeepSeekToolRequestRoundTripsNullableSelectedReferences(t *testing.T) {
	api, closeFn := newFakeDeepSeek(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["tool_choice"] != "required" || body["thinking"].(map[string]any)["type"] != "disabled" {
			t.Fatal("DeepSeek forced non-thinking contract lost")
		}
		schema := body["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any)
		selected := schema["properties"].(map[string]any)["selected"].(map[string]any)
		variant := selected["items"].(map[string]any)["properties"].(map[string]any)["variant_id"].(map[string]any)
		if variant["anyOf"] == nil {
			t.Fatalf("DeepSeek nullable field not translated: %+v", variant)
		}
		args := map[string]any{
			"response_text": "available",
			"selected": []any{map[string]any{
				"item_id":    "item-1",
				"offer_id":   map[string]any{"__mujeeb_null_v1": true},
				"variant_id": map[string]any{"__mujeeb_null_v1": true},
			}},
		}
		encodedArgs, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		reply := map[string]any{"choices": []any{map[string]any{
			"finish_reason": "tool_calls",
			"message": map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{
				"id": "call-1", "type": "function",
				"function": map[string]any{"name": "respond_without_action", "arguments": string(encodedArgs)},
			}}},
		}}, "usage": map[string]any{"prompt_tokens": 20, "completion_tokens": 10}}
		if err := json.NewEncoder(w).Encode(reply); err != nil {
			t.Fatal(err)
		}
	})
	defer closeFn()
	tool := `[{"type":"function","function":{"name":"respond_without_action","strict":true,"parameters":{"type":"object","properties":{"response_text":{"type":"string"},"selected":{"type":"array","items":{"type":"object","properties":{"item_id":{"type":"string"},"variant_id":{"type":["string","null"]},"offer_id":{"type":["string","null"]}},"required":["item_id","variant_id","offer_id"],"additionalProperties":false}}},"required":["response_text","selected"],"additionalProperties":false}}}]`
	resp, err := api.Generate(context.Background(), deepSeekReq(tool, ""))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct {
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(resp.Payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Choices) != 1 || len(decoded.Choices[0].Message.ToolCalls) != 1 {
		t.Fatal("unexpected normalized call count")
	}
	var args struct {
		Selected []struct {
			ItemID    string  `json:"item_id"`
			OfferID   *string `json:"offer_id"`
			VariantID *string `json:"variant_id"`
		} `json:"selected"`
	}
	if err := json.Unmarshal([]byte(decoded.Choices[0].Message.ToolCalls[0].Function.Arguments), &args); err != nil {
		t.Fatal(err)
	}
	if len(args.Selected) != 1 || args.Selected[0].ItemID != "item-1" || args.Selected[0].VariantID != nil || args.Selected[0].OfferID != nil {
		t.Fatalf("nullable canonical round trip failed: %+v", args)
	}
}

func TestDeepSeekDropsOnlyGeminiTransportThoughtSignatures(t *testing.T) {
	input := []json.RawMessage{
		json.RawMessage(`{"role":"system","content":"trusted"}`),
		json.RawMessage(`{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","thought_signature":"private-opaque==","function":{"name":"read_catalog","arguments":"{}"}}]}`),
		json.RawMessage(`{"role":"tool","tool_call_id":"call_1","content":"{\"records\":[1]}"}`),
	}
	out, err := deepSeekMessagesWithoutGeminiSignatures(input)
	if err != nil || len(out) != len(input) {
		t.Fatalf("cannot sanitize provider-only metadata: %v", err)
	}
	if strings.Contains(string(out[1]), "thought_signature") || strings.Contains(string(out[1]), "private-opaque") ||
		!strings.Contains(string(out[1]), "read_catalog") ||
		!strings.Contains(string(out[1]), "call_1") ||
		string(out[2]) != string(input[2]) {
		t.Fatalf("DeepSeek transport changed commercial tool history: %s", out[1])
	}
	if _, err := deepSeekMessagesWithoutGeminiSignatures([]json.RawMessage{json.RawMessage(`{"role":"assistant","tool_calls":{"bad":true}}`)}); err == nil {
		t.Fatal("malformed provider-neutral tool history accepted")
	}
}

func TestDeepSeekAutoAllowsValidatedToolWithCommentaryButDropsCommentary(t *testing.T) {
	tools := `[{"type":"function","function":{"name":"merchant_catalog_list_items","strict":true,"parameters":{"type":"object","properties":{"cursor":{"type":["string","null"]},"limit":{"type":"integer"},"status":{"type":["string","null"]}},"required":["cursor","limit","status"],"additionalProperties":false}}}]`
	cases := []struct {
		name    string
		content string
		wantErr bool
	}{
		{name: "text", content: `"I'll read the products."`},
		{name: "null", content: "null"},
		{name: "empty", content: `""`},
		{name: "bad_object_content", content: `{"approved":true}`, wantErr: true},
		{name: "bad_numeric_content", content: "123", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api, done := newFakeDeepSeek(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = fmt.Fprintf(w, `{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":%s,"tool_calls":[{"id":"read-1","type":"function","function":{"name":"merchant_catalog_list_items","arguments":"{\"cursor\":null,\"limit\":50,\"status\":null}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":8}}`, tc.content)
			})
			defer done()
			req := deepSeekReq(tools, "")
			req.Payload = json.RawMessage(strings.Replace(string(req.Payload), `"tool_choice":"required"`, `"tool_choice":"auto"`, 1))
			out, err := api.Generate(context.Background(), req)
			if tc.wantErr {
				if err == nil {
					t.Fatal("invalid tool commentary accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var normalized struct {
				Choices []struct {
					FinishReason string `json:"finish_reason"`
					Message struct {
						Content json.RawMessage `json:"content"`
						ToolCalls []struct {
							Function struct {
								Name string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
					} `json:"message"`
				} `json:"choices"`
			}
			if err := json.Unmarshal(out.Payload, &normalized); err != nil {
				t.Fatal(err)
			}
			if len(normalized.Choices) != 1 || normalized.Choices[0].FinishReason != "tool_calls" {
				t.Fatal("tool call lost after normalizing")
			}
			msg := normalized.Choices[0].Message
			if string(msg.Content) != "null" || len(msg.ToolCalls) != 1 ||
				msg.ToolCalls[0].Function.Name != "merchant_catalog_list_items" ||
				!strings.Contains(msg.ToolCalls[0].Function.Arguments, `"limit":50`) {
				t.Fatalf("tool data changed or commentary leaked: %s", out.Payload)
			}
		})
	}
}

func TestDeepSeekAutoStillRejectsInvalidToolCalls(t *testing.T) {
	tool := `[{"type":"function","function":{"name":"merchant_catalog_list_items","strict":true,"parameters":{"type":"object","properties":{"cursor":{"type":"string"},"limit":{"type":"integer"},"status":{"type":"string"}},"required":[],"additionalProperties":false}}}]`
	cases := []struct {
		name  string
		reply string
	}{
		{"unauthorized_tool", `{"finish_reason":"tool_calls","message":{"role":"assistant","content":"Reading","tool_calls":[{"id":"a","type":"function","function":{"name":"delete_everything","arguments":"{}"}}]}}`},
		{"bad_arguments", `{"finish_reason":"tool_calls","message":{"role":"assistant","content":"Reading","tool_calls":[{"id":"a","type":"function","function":{"name":"merchant_catalog_list_items","arguments":"{\"limit\":\"wrong\"}"}}]}}`},
		{"multiple_calls", `{"finish_reason":"tool_calls","message":{"role":"assistant","content":"Reading","tool_calls":[{"id":"a","type":"function","function":{"name":"merchant_catalog_list_items","arguments":"{}"}},{"id":"b","type":"function","function":{"name":"merchant_catalog_list_items","arguments":"{}"}}]}}`},
		{"wrong_finish", `{"finish_reason":"stop","message":{"role":"assistant","content":"Reading","tool_calls":[{"id":"a","type":"function","function":{"name":"merchant_catalog_list_items","arguments":"{}"}}]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api, done := newFakeDeepSeek(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = fmt.Fprintf(w, `{"choices":[%s],"usage":{"prompt_tokens":10,"completion_tokens":8}}`, tc.reply)
			})
			defer done()
			req := deepSeekReq(tool, "")
			req.Payload = json.RawMessage(strings.Replace(string(req.Payload), `"tool_choice":"required"`, `"tool_choice":"auto"`, 1))
			if _, err := api.Generate(context.Background(), req); err == nil {
				t.Fatal("unsafe auto tool response accepted")
			}
		})
	}
}

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

func fakeGemini(t *testing.T, handle http.HandlerFunc) (*Gemini, func()) {
	t.Helper()
	srv := httptest.NewServer(handle)
	client, err := NewGemini(GeminiConfig{Model: "gemini-2.5-flash", APIKey: "key-private", BaseURL: srv.URL + "/v1beta", Timeout: time.Second})
	if err != nil {
		srv.Close()
		t.Fatal(err)
	}
	return client, srv.Close
}
func TestGeminiTrustBoundariesAndForcedFunction(t *testing.T) {
	api, done := fakeGemini(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-2.5-flash:generateContent" || r.Header.Get("x-goog-api-key") != "key-private" {
			t.Error("unexpected direct Gemini endpoint")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		system := body["systemInstruction"].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"]
		user := body["contents"].([]any)[0].(map[string]any)["parts"].([]any)[0].(map[string]any)["text"]
		if system != "trusted" || user != "hello" {
			t.Error("system and user were merged")
		}
		mode := body["toolConfig"].(map[string]any)["functionCallingConfig"].(map[string]any)["mode"]
		decl := body["tools"].([]any)[0].(map[string]any)["functionDeclarations"].([]any)[0].(map[string]any)
		if mode != "ANY" || decl["name"] != "respond_without_action" || decl["parametersJsonSchema"] == nil {
			t.Error("tool was weakened")
		}
		_, _ = fmt.Fprint(w, `{"candidates":[{"finishReason":"STOP","content":{"parts":[{"functionCall":{"name":"respond_without_action","args":{"response_text":"hello","selected":[]}}}]}}],"usageMetadata":{"promptTokenCount":16,"candidatesTokenCount":7}}`)
	})
	defer done()
	payload := `{"model":"mujeeb-sales","messages":[{"role":"system","content":"trusted"},{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"respond_without_action","description":"answer","strict":true,"parameters":{"type":"object","properties":{"response_text":{"type":"string"},"selected":{"type":"array","items":{"type":"object","properties":{},"required":[],"additionalProperties":false}}},"required":["response_text","selected"],"additionalProperties":false}}}],"tool_choice":"required","parallel_tool_calls":false,"max_tokens":512,"stream":false}`
	got, err := api.Generate(context.Background(), Request{Payload: []byte(payload)})
	if err != nil || got.InputTokens != 16 || !strings.Contains(string(got.Payload), "respond_without_action") {
		t.Fatalf("Gemini tool: %+v %v", got, err)
	}
}
func TestGeminiStructuredBatchKeepsSchema(t *testing.T) {
	api, done := fakeGemini(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		cfg := body["generationConfig"].(map[string]any)
		if cfg["responseMimeType"] != "application/json" || cfg["responseJsonSchema"] == nil {
			t.Error("missing strict catalog schema")
		}
		if _, ok := body["tools"]; ok {
			t.Error("unexpected tool in structured catalog request")
		}
		_, _ = fmt.Fprint(w, `{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"{\"candidates\":[]}"}]}}],"usageMetadata":{"promptTokenCount":17,"candidatesTokenCount":4}}`)
	})
	defer done()
	p := `{"model":"mujeeb-catalog","messages":[{"role":"system","content":"trusted"},{"role":"user","content":"batch"}],"response_format":{"type":"json_schema","json_schema":{"name":"batch","strict":true,"schema":{"type":"object","properties":{"candidates":{"type":"array","items":{"type":"string"}}},"required":["candidates"],"additionalProperties":false}}},"max_tokens":512,"stream":false}`
	got, err := api.Generate(context.Background(), Request{Payload: []byte(p)})
	if err != nil || !strings.Contains(string(got.Payload), `candidates`) {
		t.Fatalf("Gemini structured: %s %v", got.Payload, err)
	}
}
func TestGemini503ClassifiedForDeepSeekFallback(t *testing.T) {
	api, done := fakeGemini(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	defer done()
	req := Request{Payload: []byte(`{"model":"sales","messages":[{"role":"user","content":"hello"}],"max_tokens":128}`)}
	if _, err := api.Generate(context.Background(), req); !IsUnavailable(err) {
		t.Fatalf("missing 503 classification: %v", err)
	}
}
func TestGeminiRejectsUnsafeOutput(t *testing.T) {
	for _, body := range []string{
		`{"candidates":[{"finishReason":"MAX_TOKENS","content":{"parts":[{"text":"partial"}]}}]}`,
		`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"not a tool"}]}}]}`,
		`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"functionCall":{"name":"a","args":{}}},{"functionCall":{"name":"b","args":{}}}]}}]}`,
	} {
		api, done := fakeGemini(t, func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, body) })
		_, err := api.Generate(context.Background(), Request{Payload: []byte(`{"model":"sales","messages":[{"role":"user","content":"hello"}],"max_tokens":123,"tool_choice":"required","parallel_tool_calls":false,"tools":[{"type":"function","function":{"name":"a","strict":true,"parameters":{"type":"object","properties":{},"required":[],"additionalProperties":false}}}]}`)})
		done()
		if err == nil {
			t.Errorf("unsafe Gemini response accepted: %s", body)
		}
	}
}

func TestGemini25AcceptsOptionalThoughtSignatureForSingleCapability(t *testing.T) {
	api, done := fakeGemini(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"candidates":[{"finishReason":"STOP","content":{"parts":[{"functionCall":{"name":"respond_without_action","args":{"response_text":"ok","selected":[]}},"thoughtSignature":"opaque-signature"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":4}}`)
	})
	defer done()
	req := Request{Payload: []byte(`{"model":"mujeeb-sales","messages":[{"role":"system","content":"trusted"},{"role":"user","content":"hello"}],"max_tokens":128,"tool_choice":"required","parallel_tool_calls":false,"tools":[{"type":"function","function":{"name":"respond_without_action","strict":true,"parameters":{"type":"object","additionalProperties":false,"required":[],"properties":{}}}}]}`)}
	if _, err := api.Generate(context.Background(), req); err != nil {
		t.Fatalf("Gemini 2.5 optional thought signature rejected: %v", err)
	}
}


func TestGeminiRejectsInvalidCatalogCandidates(t *testing.T) {
	api,done:=fakeGemini(t,func(w http.ResponseWriter,r *http.Request){
		catalogText := `{"candidates":[{"item_id":"invented","variant_ids":[],"offer_ids":[],"reason":"fake"}]}`
		response := map[string]any{"candidates": []any{map[string]any{
			"finishReason":"STOP","content":map[string]any{"parts":[]any{map[string]any{"text":catalogText}}},
		}}}
		_ = json.NewEncoder(w).Encode(response)
	})
	defer done()
	req:=`{"model":"mujeeb-catalog","messages":[{"role":"system","content":"trusted"},{"role":"user","content":"batch"}],"response_format":{"type":"json_schema","json_schema":{"name":"batch","strict":true,"schema":`+strictCatalogExample+`}},"max_tokens":512,"stream":false}`
	if _,err:=api.Generate(context.Background(),Request{Payload:json.RawMessage(req)});err==nil{
		t.Fatal("Gemini invalid catalog reference accepted")
	}
}
func TestGeminiRejectsUnknownToolsAndMalformedParameters(t *testing.T) {
	calls:=0
	api,done:=fakeGemini(t,func(w http.ResponseWriter,r *http.Request){
		calls++
		_,_=fmt.Fprint(w,`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"functionCall":{"name":"unauthorized","args":{}}}]}}]}`)
	})
	defer done()
	req:=`{"model":"sales","messages":[{"role":"system","content":"system"},{"role":"user","content":"user"}],"max_tokens":512,"tool_choice":"required","parallel_tool_calls":false,"tools":[{"type":"function","function":{"name":"safe","strict":true,"parameters":{"type":"object","properties":{},"required":[],"additionalProperties":false}}}]}`
	if _,err:=api.Generate(context.Background(),Request{Payload:json.RawMessage(req)});err==nil||calls!=1{
		t.Fatalf("unknown function should be rejected: calls=%d err=%v",calls,err)
	}
	calls=0
	invalid:=strings.Replace(req,`"required":[],"additionalProperties":false`,`"required":[],"additionalProperties":true`,1)
	if _,err:=api.Generate(context.Background(),Request{Payload:json.RawMessage(invalid)});err==nil||calls!=0{
		t.Fatalf("invalid outbound tool schema should be denied: calls=%d err=%v",calls,err)
	}
}

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
	req := Request{Payload: []byte(`{"model":"mujeeb-sales","messages":[{"role":"system","content":"trusted"},{"role":"user","content":"hello"}],"max_tokens":128,"tool_choice":"required","parallel_tool_calls":false,"tools":[{"type":"function","function":{"name":"respond_without_action","strict":true,"parameters":{"type":"object","additionalProperties":false,"required":["response_text","selected"],"properties":{"response_text":{"type":"string"},"selected":{"type":"array","items":{"type":"object","additionalProperties":false,"required":[],"properties":{}}}}}}}]}`)}
	if _, err := api.Generate(context.Background(), req); err != nil {
		t.Fatalf("Gemini 2.5 optional thought signature rejected: %v", err)
	}
}

func TestGeminiRejectsInvalidCatalogCandidates(t *testing.T) {
	api, done := fakeGemini(t, func(w http.ResponseWriter, r *http.Request) {
		catalogText := `{"candidates":[{"item_id":"invented","variant_ids":[],"offer_ids":[],"reason":"fake"}]}`
		response := map[string]any{"candidates": []any{map[string]any{
			"finishReason": "STOP", "content": map[string]any{"parts": []any{map[string]any{"text": catalogText}}},
		}}}
		_ = json.NewEncoder(w).Encode(response)
	})
	defer done()
	req := `{"model":"mujeeb-catalog","messages":[{"role":"system","content":"trusted"},{"role":"user","content":"batch"}],"response_format":{"type":"json_schema","json_schema":{"name":"batch","strict":true,"schema":` + strictCatalogExample + `}},"max_tokens":512,"stream":false}`
	if _, err := api.Generate(context.Background(), Request{Payload: json.RawMessage(req)}); err == nil {
		t.Fatal("Gemini invalid catalog reference accepted")
	}
}
func TestGeminiRejectsUnknownToolsAndMalformedParameters(t *testing.T) {
	calls := 0
	api, done := fakeGemini(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = fmt.Fprint(w, `{"candidates":[{"finishReason":"STOP","content":{"parts":[{"functionCall":{"name":"unauthorized","args":{}}}]}}]}`)
	})
	defer done()
	req := `{"model":"sales","messages":[{"role":"system","content":"system"},{"role":"user","content":"user"}],"max_tokens":512,"tool_choice":"required","parallel_tool_calls":false,"tools":[{"type":"function","function":{"name":"safe","strict":true,"parameters":{"type":"object","properties":{},"required":[],"additionalProperties":false}}}]}`
	if _, err := api.Generate(context.Background(), Request{Payload: json.RawMessage(req)}); err == nil || calls != 1 {
		t.Fatalf("unknown function should be rejected: calls=%d err=%v", calls, err)
	}
	calls = 0
	invalid := strings.Replace(req, `"required":[],"additionalProperties":false`, `"required":[],"additionalProperties":true`, 1)
	if _, err := api.Generate(context.Background(), Request{Payload: json.RawMessage(invalid)}); err == nil || calls != 0 {
		t.Fatalf("invalid outbound tool schema should be denied: calls=%d err=%v", calls, err)
	}
}

func TestGemini31SalesCapabilityPreservesSignedFunctionCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-3.1-flash-lite:generateContent" {
			t.Errorf("wrong Gemini model URL: %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, `{"candidates":[{"finishReason":"STOP","content":{"parts":[{"functionCall":{"name":"answer_customer","args":{"text":"متوفر"}},"thoughtSignature":"exact-opaque-signature=="}]}}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":6}}`)
	}))
	defer srv.Close()
	api, err := NewGemini(GeminiConfig{Model:"gemini-3.1-flash-lite",APIKey:"fake",BaseURL:srv.URL+"/v1beta",Timeout:time.Second})
	if err != nil {t.Fatal(err)}
	req := Request{Payload:json.RawMessage(`{"model":"mujeeb-sales","messages":[{"role":"system","content":"trusted"},{"role":"user","content":"مرحبا"}],"tools":[{"type":"function","function":{"name":"answer_customer","strict":true,"parameters":{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}}}],"tool_choice":"required","parallel_tool_calls":false,"max_tokens":128}`)}
	result, err := api.Generate(context.Background(),req)
	if err != nil {t.Fatalf("Gemini 3.1 signed capability rejected: %v",err)}
	var parsed struct{
		Choices []struct{
			FinishReason string `json:"finish_reason"`
			Message struct{ToolCalls []struct{
				ThoughtSignature string `json:"thought_signature"`
				Function struct{Name string `json:"name"`} `json:"function"`
			} `json:"tool_calls"`} `json:"message"`
		} `json:"choices"`
	}
	if err:=json.Unmarshal(result.Payload,&parsed);err!=nil{t.Fatal(err)}
	if len(parsed.Choices)!=1||parsed.Choices[0].FinishReason!="tool_calls"||
		len(parsed.Choices[0].Message.ToolCalls)!=1||
		parsed.Choices[0].Message.ToolCalls[0].ThoughtSignature!="exact-opaque-signature=="||
		parsed.Choices[0].Message.ToolCalls[0].Function.Name!="answer_customer"{
		t.Fatalf("Gemini function signature or canonical capability lost: %s",result.Payload)
	}
}

func TestGemini31SequentialFunctionCallsReplayAllOriginalSignatures(t *testing.T) {
	callCount:=0
	srv:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		var wire struct{
			Contents []struct{
				Role string `json:"role"`
				Parts []struct {
					Text string `json:"text"`
					ThoughtSignature string `json:"thoughtSignature"`
					FunctionCall *struct{
						Name string `json:"name"`
						ID string `json:"id"`
					} `json:"functionCall"`
					FunctionResponse *struct{Name string `json:"name"`;ID string `json:"id"`} `json:"functionResponse"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if err:=json.NewDecoder(r.Body).Decode(&wire);err!=nil{t.Error(err)}
		if len(wire.Contents)!=1+2*callCount {t.Errorf("incomplete replay count=%d messages=%d",callCount,len(wire.Contents))}
		for step:=0;step<callCount;step++{
			model:=wire.Contents[1+2*step]
			tool:=wire.Contents[2+2*step]
			wantSignature:=fmt.Sprintf("opaque-thought-%d==",step+1)
			wantName:=fmt.Sprintf("read_%d",step+1)
			if model.Role!="model"||len(model.Parts)!=1||model.Parts[0].ThoughtSignature!=wantSignature||
				model.Parts[0].FunctionCall==nil||model.Parts[0].FunctionCall.Name!=wantName||
				tool.Role!="user"||len(tool.Parts)!=1||tool.Parts[0].FunctionResponse==nil||
				tool.Parts[0].FunctionResponse.Name!=wantName{
				t.Errorf("original signature/function response not replayed for step %d: %+v %+v",step,model,tool)
			}
		}
		callCount++
		if callCount<=2{
			_,_=fmt.Fprintf(w,`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"functionCall":{"name":"read_%d","id":"call_%d","args":{}},"thoughtSignature":"opaque-thought-%d=="}]}}]}`,callCount,callCount,callCount)
		} else {
			_,_=fmt.Fprint(w,`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"All tools complete"}]}}]}`)
		}
	}))
	defer srv.Close()
	api,err:=NewGemini(GeminiConfig{Model:"gemini-3.1-flash-lite",APIKey:"fake",BaseURL:srv.URL+"/v1beta",Timeout:time.Second})
	if err!=nil{t.Fatal(err)}
	history:=[]any{
		map[string]any{"role":"system","content":"Only read scoped catalog evidence"},
		map[string]any{"role":"user","content":"Read information before answering"},
	}
	for step:=0;step<3;step++ {
		body:=map[string]any{
			"model":"mujeeb-merchant","messages":history,"max_tokens":128,"tool_choice":"auto","parallel_tool_calls":false,
			"tools":[]any{
				map[string]any{"type":"function","function":map[string]any{"name":"read_1","strict":true,
					"parameters":map[string]any{"type":"object","properties":map[string]any{},"required":[]string{},"additionalProperties":false}}},
				map[string]any{"type":"function","function":map[string]any{"name":"read_2","strict":true,
					"parameters":map[string]any{"type":"object","properties":map[string]any{},"required":[]string{},"additionalProperties":false}}},
			},
		}
		raw,_:=json.Marshal(body)
		response,err:=api.Generate(context.Background(),Request{Payload:raw})
		if err!=nil{t.Fatalf("Gemini 3.1 round %d: %v",step+1,err)}
		var parsed struct{Choices []struct{
			FinishReason string `json:"finish_reason"`
			Message struct{
				Content *string `json:"content"`
				ToolCalls []map[string]any `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`}
		if err:=json.Unmarshal(response.Payload,&parsed);err!=nil{t.Fatal(err)}
		if len(parsed.Choices)!=1{t.Fatalf("no Gemini choice: %s",response.Payload)}
		if step==2 {
			if parsed.Choices[0].FinishReason!="stop"||parsed.Choices[0].Message.Content==nil||
				*parsed.Choices[0].Message.Content!="All tools complete" {t.Fatalf("final: %s",response.Payload)}
			break
		}
		calls:=parsed.Choices[0].Message.ToolCalls
		if len(calls)!=1 {t.Fatalf("missing tool call at step %d",step)}
		wantSig:=fmt.Sprintf("opaque-thought-%d==",step+1)
		if calls[0]["thought_signature"]!=wantSig {t.Fatalf("signature was mutated: %#v",calls[0])}
		history=append(history,map[string]any{"role":"assistant","tool_calls":calls})
		history=append(history,map[string]any{"role":"tool","tool_call_id":calls[0]["id"],
			"content":`{"read_only_result":true}`})
	}
	if callCount!=3 {t.Fatalf("expected three inference calls, got %d",callCount)}
}

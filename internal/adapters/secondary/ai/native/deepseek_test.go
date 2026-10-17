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

func newFakeDeepSeek(t *testing.T, handler http.HandlerFunc) (*DeepSeek,func()){
	t.Helper()
	srv:=httptest.NewServer(handler)
	api,err:=NewDeepSeek(DeepSeekConfig{Model:"deepseek-flash",APIKey:"secret-key",BaseURL:srv.URL+"/beta",Timeout:time.Second})
	if err!=nil {srv.Close();t.Fatal(err)}
	return api,srv.Close
}
func deepSeekReq(tools,format string) Request {
	body:=`{"model":"mujeeb-sales","messages":[{"role":"system","content":"trusted system rules"},{"role":"user","content":"untrusted customer text"}],"max_tokens":123,"tool_choice":"required","parallel_tool_calls":false,"tools":`+tools+`}`
	if format!="" {body=`{"model":"mujeeb-catalog","messages":[{"role":"system","content":"trusted"},{"role":"user","content":"payload"}],"max_tokens":123,"response_format":`+format+`}`}
	return Request{Payload:json.RawMessage(body)}
}
func TestDeepSeekForcedToolsUseBetaNonThinkingWithoutLosingSchema(t *testing.T){
	client,closeFn:=newFakeDeepSeek(t,func(w http.ResponseWriter,r *http.Request){
		if r.URL.Path!="/beta/chat/completions"||r.Header.Get("Authorization")!="Bearer secret-key" {t.Error("invalid direct DeepSeek endpoint/auth")}
		var body map[string]any
		if err:=json.NewDecoder(r.Body).Decode(&body);err!=nil {t.Error(err)}
		if body["tool_choice"]!="required"||body["thinking"].(map[string]any)["type"]!="disabled" {t.Error("forced non-thinking not preserved")}
		tools:=body["tools"].([]any);fn:=tools[0].(map[string]any)["function"].(map[string]any)
		if fn["strict"]!=true||fn["name"]!="evaluate_full_catalog" {t.Error("strict function erased")}
		messages:=body["messages"].([]any);if messages[0].(map[string]any)["role"]!="system"||messages[1].(map[string]any)["role"]!="user" {t.Error("trust boundaries merged")}
		_,_=fmt.Fprint(w,`{"model":"deepseek-flash","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"call-a","type":"function","function":{"name":"evaluate_full_catalog","arguments":"{}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":4}}`)
	});defer closeFn()
	tools:=`[{"type":"function","function":{"name":"evaluate_full_catalog","strict":true,"description":"fetch evidence","parameters":{"type":"object","properties":{},"required":[],"additionalProperties":false}}}]`
	got,err:=client.Generate(context.Background(),deepSeekReq(tools,""))
	if err!=nil||got.Model!="deepseek-flash"||got.InputTokens!=10||!strings.Contains(string(got.Payload),"evaluate_full_catalog"){t.Fatalf("result %+v error %v",got,err)}
}
func TestDeepSeekCatalogSchemaUsesStrictToolRatherThanWeakJSONObject(t *testing.T){
	api,done:=newFakeDeepSeek(t,func(w http.ResponseWriter,r *http.Request){
		var body map[string]any
		_=json.NewDecoder(r.Body).Decode(&body)
		if _,bad:=body["response_format"];bad {t.Error("plain JSON mode must not pretend to satisfy strict schema")}
		fn:=body["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)
		schema:=fn["parameters"].(map[string]any)
		if fn["strict"]!=true||schema["additionalProperties"]!=false||schema["required"].([]any)[0]!="candidates" {t.Fatal("catalog schema weakened")}
		_,_=fmt.Fprint(w,`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"x","type":"function","function":{"name":"submit_catalog_batch","arguments":"{\"candidates\":[]}"}}]}}],"usage":{"prompt_tokens":4,"completion_tokens":8}}`)
	});defer done()
	schema:=`{"type":"json_schema","json_schema":{"name":"catalog_batch_candidates","strict":true,"schema":{"type":"object","properties":{"candidates":{"type":"array","items":{"type":"object","properties":{},"required":[],"additionalProperties":false}}},"required":["candidates"],"additionalProperties":false}}}`
	got,err:=api.Generate(context.Background(),deepSeekReq("",schema))
	if err!=nil||!strings.Contains(string(got.Payload),`{\"candidates\":[]}`){t.Fatalf("strict batch: %s %v",got.Payload,err)}
}
func TestDeepSeekFailsClosedOnUnsupportedMerchantAndProviderFailures(t *testing.T){
	called:=0
	api,done:=newFakeDeepSeek(t,func(w http.ResponseWriter,r *http.Request){called++;w.WriteHeader(503)})
	defer done()
	cases:=[]Request{
		{Payload:[]byte(`{"model":"x","messages":[{"role":"user","content":"x"}],"max_tokens":8,"tools":[{"type":"function","function":{"name":"read","strict":true}}],"tool_choice":"auto","response_format":{"type":"json_schema","json_schema":{"name":"merchant","strict":true,"schema":{}}}}`)},
		{Payload:[]byte(`{"model":"x","messages":[{"role":"user","content":"x"}],"max_tokens":8,"tools":[{"type":"function","function":{"name":"read","strict":false}}],"tool_choice":"required"}`)},
	}
	for _,req:=range cases {if _,err:=api.Generate(context.Background(),req);err==nil {t.Error("unsafe request accepted")}}
	if called!=0 {t.Fatal("unsafe requests sent to provider")}
	_,err:=api.Generate(context.Background(),deepSeekReq(`[{"type":"function","function":{"name":"safe","strict":true,"parameters":{"type":"object","properties":{},"required":[],"additionalProperties":false}}}]`,""))
	if !IsUnavailable(err) {t.Fatalf("503 not classified: %v",err)}
}
func TestDeepSeekRejectsTruncatedResponse(t *testing.T){
	api,done:=newFakeDeepSeek(t,func(w http.ResponseWriter,r *http.Request){
		_,_=fmt.Fprint(w,`{"choices":[{"finish_reason":"length","message":{"role":"assistant","content":"partial"}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)
	});defer done()
	if _,err:=api.Generate(context.Background(),deepSeekReq(`[{"type":"function","function":{"name":"safe","strict":true,"parameters":{}}}]`,""));err==nil {t.Fatal("truncated response accepted")}
}

package native

import (
 "context"
 "encoding/json"
 "fmt"
 "net/http"
 "net/http/httptest"
 "sync/atomic"
 "testing"
 "time"
)

// This exercises the REAL native transport adapters + router, with a local HTTP
// provider simulation. It is not a claim of live Gemini/DeepSeek compatibility.
func TestNativeFallbackGemini503ToDeepSeekStrictCapability(t *testing.T) {
 var geminiCount,deepSeekCount atomic.Int32
 geminiHTTP:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  geminiCount.Add(1)
  if r.URL.Path!="/v1beta/models/gemini-2.5-flash:generateContent"{
   t.Errorf("wrong Gemini URL %q",r.URL.Path)
  }
  w.WriteHeader(http.StatusServiceUnavailable)
 }))
 defer geminiHTTP.Close()
 deepSeekHTTP:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  deepSeekCount.Add(1)
  if r.URL.Path!="/beta/chat/completions"{
   t.Errorf("wrong DeepSeek URL %q",r.URL.Path)
  }
  var body map[string]any
  if err:=json.NewDecoder(r.Body).Decode(&body);err!=nil{t.Error(err)}
  if body["tool_choice"]!="required"{t.Error("mandatory capability downgraded")}
  mode,ok:=body["thinking"].(map[string]any)
  if !ok||mode["type"]!="disabled"{t.Error("DeepSeek thinking not disabled for forced tool")}
  _,_=fmt.Fprint(w,`{"choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"evaluate_full_catalog","arguments":"{}"}}]}}],"usage":{"prompt_tokens":100,"completion_tokens":5}}`)
 }))
 defer deepSeekHTTP.Close()
 g,err:=NewGemini(GeminiConfig{Model:"gemini-2.5-flash",APIKey:"mock",BaseURL:geminiHTTP.URL+"/v1beta",Timeout:time.Second})
 if err!=nil{t.Fatal(err)}
 d,err:=NewDeepSeek(DeepSeekConfig{Model:"deepseek-flash",APIKey:"mock",BaseURL:deepSeekHTTP.URL+"/beta",Timeout:time.Second})
 if err!=nil{t.Fatal(err)}
 router,err:=NewRouter(g,d,Policy{ConsecutiveFailures:1,Cooldown:time.Minute})
 if err!=nil{t.Fatal(err)}
 request:=Request{Operation:"customer_decision",Payload:json.RawMessage(`{
  "model":"mujeeb-sales",
  "messages":[{"role":"system","content":"Trusted rules"},{"role":"user","content":"عندكم آيفون؟"}],
  "tools":[{"type":"function","function":{"name":"evaluate_full_catalog",
   "description":"Request complete merchant evidence","strict":true,
   "parameters":{"type":"object","properties":{},"required":[],"additionalProperties":false}}}],
  "tool_choice":"required","parallel_tool_calls":false,"max_tokens":512,"stream":false
 }`)}
 response,trace,err:=router.Generate(context.Background(),request)
 if err!=nil{t.Fatal(err)}
 if geminiCount.Load()!=1||deepSeekCount.Load()!=1||!trace.FallbackUsed||len(trace.Attempts)!=2||
  trace.Attempts[0].HTTPStatus!=http.StatusServiceUnavailable||response.Model!="deepseek-flash"{
  t.Fatalf("incorrect failover: trace=%+v response=%+v",trace,response)
 }
 var parsed struct {
  Choices []struct {
   FinishReason string `json:"finish_reason"`
   Message struct{
    ToolCalls []struct{
     Function struct{Name string `json:"name"`;Arguments string `json:"arguments"`} `json:"function"`
    } `json:"tool_calls"`
   } `json:"message"`
  } `json:"choices"`
 }
 if err:=json.Unmarshal(response.Payload,&parsed);err!=nil{t.Fatal(err)}
 if len(parsed.Choices)!=1||parsed.Choices[0].FinishReason!="tool_calls"||
  len(parsed.Choices[0].Message.ToolCalls)!=1||
  parsed.Choices[0].Message.ToolCalls[0].Function.Name!="evaluate_full_catalog"||
  parsed.Choices[0].Message.ToolCalls[0].Function.Arguments!="{}"{
  t.Fatalf("capability was lost or malformed: %+v",parsed)
 }
 // The circuit suppresses another Gemini attempt while the 503 is active.
 _,trace2,err:=router.Generate(context.Background(),request)
 if err!=nil||!trace2.FallbackUsed||geminiCount.Load()!=1||deepSeekCount.Load()!=2{
  t.Fatalf("breaker did not honor cooldown: %+v err=%v",trace2,err)
 }
}

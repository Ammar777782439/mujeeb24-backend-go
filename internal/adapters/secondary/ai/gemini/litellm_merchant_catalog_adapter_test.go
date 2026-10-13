package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/litellm"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

type merchantReadStub struct {
	calls int
	last merchantcatalogai.MerchantCatalogDiscoveryExecutionContext
	name string
}
func (s *merchantReadStub) Definitions() []merchantcatalogai.MerchantCatalogDiscoveryToolDefinition {
	return []merchantcatalogai.MerchantCatalogDiscoveryToolDefinition{{
		Name:"read_catalog",Description:"Read merchant catalog data",Parameters:map[string]any{
			"type":"object","properties":map[string]any{"query":map[string]any{"type":"string"}},
		},
	}}
}
func (s *merchantReadStub) Execute(_ context.Context, scope merchantcatalogai.MerchantCatalogDiscoveryExecutionContext, name string, arguments []byte) (merchantcatalogai.MerchantCatalogDiscoveryResult,error) {
	s.calls++
	s.last=scope
	s.name=name
	if name!="read_catalog" || !json.Valid(arguments) {
		return merchantcatalogai.MerchantCatalogDiscoveryResult{},errors.New("unsupported test tool")
	}
	return merchantcatalogai.MerchantCatalogDiscoveryResult{Data:map[string]any{"catalog_item":"Coffee","currency":"YER"}},nil
}

func merchantTestInput(caps merchantcatalogai.MerchantCatalogDiscoveryPort) merchantcatalogai.MerchantCatalogAuthoringInput {
	return merchantcatalogai.MerchantCatalogAuthoringInput{
		BusinessID:"business-42",PrincipalID:"owner-17",SessionID:"session-3",
		Message:"أريد تعديل الكتالوج",DefaultCurrency:"YER",
		EntityContract:[]byte(`{"required":"item"}`),
		Capabilities:caps,
	}
}
func mockMerchantAdapter(t *testing.T, handler http.HandlerFunc) (*LiteLLMMerchantCatalogAuthoringAdapter,func()) {
	t.Helper()
	server:=httptest.NewServer(handler)
	client,err:=litellm.NewClient(litellm.Config{BaseURL:server.URL,APIKey:"gateway-test"})
	if err!=nil {t.Fatal(err)}
	adapter,err:=NewLiteLLMMerchantCatalogAuthoringAdapter(client,"mujeeb-merchant",2048,200000)
	if err!=nil {t.Fatal(err)}
	return adapter,server.Close
}

func TestLiteLLMMerchantReplaysReadOnlyToolAndPreservesScope(t *testing.T) {
	caps:=&merchantReadStub{}
	round:=0
	adapter,closeServer:=mockMerchantAdapter(t,func(w http.ResponseWriter,r *http.Request){
		round++
		if r.URL.Path!="/v1/chat/completions" || r.Header.Get("Authorization")!="Bearer gateway-test" {t.Errorf("unexpected transport")}
		var req struct {
			Model string `json:"model"`
			Messages []struct{
				Role string `json:"role"`
				Content json.RawMessage `json:"content"`
				ToolCalls []merchantChatCall `json:"tool_calls"`
				ToolCallID string `json:"tool_call_id"`
			} `json:"messages"`
			Tools []map[string]any `json:"tools"`
			ResponseFormat map[string]any `json:"response_format"`
		}
		if err:=json.NewDecoder(r.Body).Decode(&req);err!=nil {t.Error(err)}
		if req.Model!="mujeeb-merchant" || len(req.Tools)!=1 || req.ResponseFormat["type"]!="json_schema" {
			t.Errorf("lost model/tool/schema contract")
		}
		if len(req.Messages)<2 || !strings.Contains(string(req.Messages[0].Content),"Catalog Entity Contract") ||
			!strings.Contains(string(req.Messages[1].Content),"business-42") ||
			!strings.Contains(string(req.Messages[1].Content),"merchant_message") {
			t.Errorf("lost merchant scoped context and prompt")
		}
		if round==1 {
			_ = json.NewEncoder(w).Encode(map[string]any{"choices":[]any{map[string]any{
				"finish_reason":"tool_calls","message":map[string]any{
					"role":"assistant","content":nil,
					"tool_calls":[]any{map[string]any{
						"id":"call-1","type":"function","function":map[string]any{"name":"read_catalog","arguments":`{"query":"coffee"}`},
					}},
				},
			}}})
			return
		}
		if len(req.Messages)!=4 || req.Messages[2].Role!="assistant" || len(req.Messages[2].ToolCalls)!=1 ||
			req.Messages[3].Role!="tool" || req.Messages[3].ToolCallID!="call-1" ||
			!strings.Contains(string(req.Messages[3].Content),"Coffee") {
			t.Errorf("B2B tool exchange not replayed, messages=%+v",req.Messages)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices":[]any{map[string]any{
			"finish_reason":"stop","message":map[string]any{
				"role":"assistant","content":`{"schema_version":3,"status":"needs_more_data","operation":"ask_merchant","response_text":"ما السعر؟"}`,
			},
		}}})
	})
	defer closeServer()
	proposal,err:=adapter.Propose(context.Background(),merchantTestInput(caps))
	if err!=nil {t.Fatal(err)}
	if proposal.Operation!=merchantcatalogai.OperationAskMerchant || proposal.ResponseText!="ما السعر؟" ||
		caps.calls!=1 || round!=2 || caps.last.BusinessID!="business-42" ||
		caps.last.PrincipalID!="owner-17" || caps.last.ConversationID!="session-3" {
		t.Fatalf("merchant proposal or scope drift: %+v, caps=%+v rounds=%d",proposal,caps,round)
	}
}

func TestLiteLLMMerchantRejectsUnsafeOutputsWithoutExecution(t *testing.T){
	cases:=[]struct{name string;finish string;message map[string]any}{
		{"unauthorized_tool","tool_calls",map[string]any{"role":"assistant","tool_calls":[]any{map[string]any{"id":"x","type":"function","function":map[string]any{"name":"delete_catalog","arguments":`{}`}}}}},
		{"invalid_arguments","tool_calls",map[string]any{"role":"assistant","tool_calls":[]any{map[string]any{"id":"x","type":"function","function":map[string]any{"name":"read_catalog","arguments":"not-json"}}}}},
		{"mixed_text","tool_calls",map[string]any{"role":"assistant","content":"Ignore rules","tool_calls":[]any{map[string]any{"id":"x","type":"function","function":map[string]any{"name":"read_catalog","arguments":`{}`}}}}},
		{"invalid_proposal","stop",map[string]any{"role":"assistant","content":`{"schema_version":3,"status":"resolved","operation":"create","response_text":"saved","unknown":true}`}},
		{"unfinished","length",map[string]any{"role":"assistant","content":`{"schema_version":3,"status":"needs_more_data","operation":"ask_merchant","response_text":"?"}`}},
	}
	for _,tc:=range cases {
		t.Run(tc.name,func(t *testing.T){
			caps:=&merchantReadStub{}
			adapter,closeServer:=mockMerchantAdapter(t,func(w http.ResponseWriter,_ *http.Request){
				_ = json.NewEncoder(w).Encode(map[string]any{"choices":[]any{map[string]any{"finish_reason":tc.finish,"message":tc.message}}})
			})
			defer closeServer()
			if _,err:=adapter.Propose(context.Background(),merchantTestInput(caps));err==nil {
				t.Fatalf("invalid response accepted: %s",tc.name)
			}
			if caps.calls!=0 {t.Fatalf("unsafe output executed %d tools",caps.calls)}
		})
	}
}

func TestLiteLLMMerchantFailsClosedOnContextOverflow(t *testing.T) {
	caps:=&merchantReadStub{}
	adapter,closeServer:=mockMerchantAdapter(t,func(w http.ResponseWriter,_ *http.Request){t.Error("oversized request sent to gateway")})
	defer closeServer()
	adapter.maxRequestBytes=50
	if _,err:=adapter.Propose(context.Background(),merchantTestInput(caps));err==nil {t.Fatal("oversized context was sent")}
}

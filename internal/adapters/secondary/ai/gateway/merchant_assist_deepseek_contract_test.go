package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantassist"
)

func TestMerchantAssistParameterlessStrictToolReachesDeepSeekTransport(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/beta/chat/completions" || r.Method != http.MethodPost {
			t.Errorf("incorrect DeepSeek strict endpoint: %s", r.URL.Path)
		}
		var body struct {
			Model string `json:"model"`
			ToolChoice string `json:"tool_choice"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
					Strict bool `json:"strict"`
					Parameters struct {
						Type string `json:"type"`
						Properties map[string]json.RawMessage `json:"properties"`
						Required []string `json:"required"`
						AdditionalProperties bool `json:"additionalProperties"`
					} `json:"parameters"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("invalid provider JSON: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body.Model != "deepseek-flash" || body.ToolChoice != "auto" ||
			len(body.Tools) != 1 || body.Tools[0].Function.Name != "dashboard_overview" ||
			!body.Tools[0].Function.Strict ||
			body.Tools[0].Function.Parameters.Required == nil ||
			body.Tools[0].Function.Parameters.Properties == nil ||
			len(body.Tools[0].Function.Parameters.Required) != 0 ||
			body.Tools[0].Function.Parameters.AdditionalProperties {
			t.Errorf("provider received invalid parameterless strict tool: %+v", body)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"model":"deepseek-flash","choices":[{"finish_reason":"tool_calls","message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"dashboard_overview","arguments":"{}"}}]}}],"usage":{"prompt_tokens":100,"completion_tokens":25}}`)
	}))
	defer server.Close()

	provider, err := native.NewDeepSeek(native.DeepSeekConfig{
		Model:"deepseek-flash", APIKey:"fake-test-key",
		BaseURL:server.URL+"/beta", Timeout:time.Second,
	})
	if err != nil { t.Fatal(err) }
	router, err := native.NewRouter(provider,nil,native.Policy{})
	if err != nil { t.Fatal(err) }
	client, err := native.NewCompletionClient(router)
	if err != nil { t.Fatal(err) }
	assistant := &MerchantAssistNativeTools{Client:client,Model:"mujeeb-merchant"}
	response, err := assistant.CompleteTools(context.Background(),
		[]merchantassist.ToolMessage{{Role:"user",Content:"كم عندي عميل؟"}},
		[]merchantassist.ToolDefinition{{
			Name:"dashboard_overview", Description:"Read approved dashboard totals",
			Parameters:map[string]any{
				"type":"object", "additionalProperties":false,
				"properties":map[string]any{}, "required":[]string{},
			},
		}}, "")
	if err != nil { t.Fatalf("strict parameterless tool failed before HTTP: %v",err) }
	if calls != 1 || response.FinishReason != "tool_calls" ||
		len(response.Calls)!=1 || response.Calls[0].Name != "dashboard_overview" ||
		string(response.Calls[0].Arguments) != "{}" {
		t.Fatalf("native strict function regression: calls=%d response=%+v",calls,response)
	}
}

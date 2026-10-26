package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
)

func TestMerchantAssistCompletionUsesStrictNativeDeepSeekContract(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/beta/chat/completions" {
			t.Errorf("unexpected DeepSeek route: %s %s", r.Method, r.URL.Path)
		}
		var wire struct {
			Model    string           `json:"model"`
			Messages []map[string]any `json:"messages"`
			Thinking map[string]string `json:"thinking"`
		}
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			t.Errorf("cannot parse wire request: %v", err)
		}
		if wire.Model != "deepseek-test" || len(wire.Messages) != 2 ||
			wire.Thinking["type"] != "disabled" {
			t.Errorf("invalid native request: model=%s messages=%d thinking=%v",
				wire.Model, len(wire.Messages), wire.Thinking)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"deepseek-test","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"هذه إجابة تجريبية."}}],"usage":{"prompt_tokens":10,"completion_tokens":8,"prompt_cache_hit_tokens":0}}`))
	}))
	defer server.Close()

	provider, err := native.NewDeepSeek(native.DeepSeekConfig{
		Model: "deepseek-test", APIKey: "local-test-key",
		BaseURL: server.URL + "/beta", Timeout: 2 * time.Second,
	})
	if err != nil { t.Fatal(err) }
	router, err := native.NewRouter(provider, nil, native.Policy{})
	if err != nil { t.Fatal(err) }
	client, err := native.NewCompletionClient(router)
	if err != nil { t.Fatal(err) }
	generator := &MerchantAssistCompletion{Client: client, Model: "mujeeb-merchant"}
	answer, err := generator.Generate(context.Background(), "قواعد موثوقة", "سؤال تجريبي")
	if err != nil { t.Fatalf("merchant assist failed before reaching native provider: %v", err) }
	if calls != 1 || answer != "هذه إجابة تجريبية." {
		t.Fatalf("unexpected invocation calls=%d answer=%q", calls, answer)
	}
}

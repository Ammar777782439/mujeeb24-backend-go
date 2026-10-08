package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

func TestBatchUsesInteractionsAndCompatibleJSONTokenSchema(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Error("missing API key header")
		}
		switch r.URL.Path {
		case "/v1beta/models/test-model:countTokens":
			request := body["generateContentRequest"].(map[string]any)
			if request["model"] != "models/test-model" {
				t.Error("missing tokenizer model")
			}
			config := request["generationConfig"].(map[string]any)
			if _, ok := config["responseSchema"]; ok {
				t.Error("legacy Schema protobuf cannot accept JSON Schema keywords")
			}
			schema := config["responseJsonSchema"].(map[string]any)
			if schema["additionalProperties"] != false {
				t.Error("closed output schema was lost")
			}
			_, _ = w.Write([]byte(`{"totalTokens":125}`))
		case "/v1/interactions":
			if body["store"] != false {
				t.Error("batch should be stateless")
			}
			if _, ok := body["previous_interaction_id"]; ok {
				t.Error("batch must not chain")
			}
			if _, ok := body["contents"]; ok {
				t.Error("legacy generation payload sent")
			}
			if body["system_instruction"] == "" || body["input"] == "" {
				t.Error("prompt content missing")
			}
			format := body["response_format"].(map[string]any)
			if format["type"] != "text" || format["mime_type"] != "application/json" {
				t.Error("wrong response format")
			}
			if format["schema"].(map[string]any)["additionalProperties"] != false {
				t.Error("schema lost")
			}
			_, _ = w.Write([]byte(`{"id":"batch-1","status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"{\"candidates\":[]}"}]}],"usage":{"total_input_tokens":125,"total_output_tokens":9,"total_cached_tokens":10}}`))
		default:
			t.Errorf("unexpected generation route %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client, err := NewBatchClient(BatchClientConfig{BaseURL: server.URL, APIKey: "test-key", Model: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	input := services.BatchEvaluationInput{CustomerMessage: "Hello", EntityContract: services.BuildCatalogEntityContractPayload(), Batch: services.CatalogAIBatchPayload{Items: []services.CatalogAIItem{{}}}}
	count, err := client.CountBatchTokens(context.Background(), input)
	if err != nil || count != 125 {
		t.Fatalf("count: %d %v", count, err)
	}
	result, err := client.EvaluateBatch(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || result.Usage.InputTokens != 125 || result.Usage.CachedTokens != 10 {
		t.Fatalf("unexpected usage: %+v", result.Usage)
	}
}

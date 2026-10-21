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
)

func newCatalogCountingGemini(t *testing.T, count int64) (*native.Gemini, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-2.5-flash:countTokens" {
			t.Errorf("wrong Gemini tokenizer endpoint: %s", r.URL.Path)
		}
		var decoded map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&decoded); err != nil {
			t.Error(err)
		}
		if len(decoded["generateContentRequest"]) == 0 {
			t.Error("Gemini token count did not include the full request")
		}
		_, _ = fmt.Fprintf(w, `{"totalTokens":%d}`, count)
	}))
	api, err := native.NewGemini(native.GeminiConfig{
		Model: "gemini-2.5-flash", APIKey: "test-key", BaseURL: srv.URL + "/v1beta", Timeout: time.Second,
	})
	if err != nil {
		srv.Close()
		t.Fatal(err)
	}
	return api, srv.Close
}

func TestNativeCatalogUsesGeminiCountTokensWithoutStandby(t *testing.T) {
	gem, done := newCatalogCountingGemini(t, 70)
	defer done()
	router, err := native.NewRouter(gem, nil, native.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := native.NewCompletionClient(router)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewNativeCatalogAdapter(client, "mujeeb-catalog", 4096, 32768)
	if err != nil {
		t.Fatal(err)
	}
	request := chatCompletionRequest{
		Model:     "mujeeb-catalog",
		Messages:  []chatCompletionMessage{
			{Role: "system", Content: "Only verified catalog facts"},
			{Role: "user", Content: "Find the correct item"},
		},
		MaxTokens: 1024,
	}
	actual, err := adapter.count(context.Background(), request)
	if err != nil || actual != 70 {
		t.Fatalf("exact Gemini input count ignored: actual=%d error=%v", actual, err)
	}
}

func TestNativeCatalogCountPreservesStandbySafety(t *testing.T) {
	gem, done := newCatalogCountingGemini(t, 70)
	defer done()
	backup := &customerNativeProvider{name: "deepseek"}
	router, err := native.NewRouter(gem, backup, native.Policy{ConsecutiveFailures: 2, Cooldown: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	client, err := native.NewCompletionClient(router)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewNativeCatalogAdapter(client, "mujeeb-catalog", 4096, 32768)
	if err != nil {
		t.Fatal(err)
	}
	request := chatCompletionRequest{
		Model:     "mujeeb-catalog",
		Messages:  []chatCompletionMessage{{Role: "user", Content: "Find all catalog items"}},
		MaxTokens: 1024,
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := adapter.count(context.Background(), request)
	if err != nil || actual < len(encoded)+4096 {
		t.Fatalf("unsafe cross-provider token admission: %d, %v", actual, err)
	}
}

package litellm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCountChatTokensUsesGatewayModelAndConservativeBudget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/utils/token_counter" || r.Header.Get("Authorization") != "Bearer private-key" {
			t.Errorf("unexpected token counter request %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.Model != "mujeeb-catalog" || len(payload.Messages) != 2 ||
			payload.Messages[0].Content != "system contract" || payload.Messages[1].Content != "customer and batch" {
			t.Errorf("lost token-count context %+v", payload)
		}
		_, _ = w.Write([]byte(`{"total_tokens":17,"tokenizer_type":"openai_tokenizer"}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "private-key"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.CountChatTokens(context.Background(), map[string]any{"model": "mujeeb-catalog", "messages": []map[string]string{{"role":"system","content":"system contract"},{"role":"user","content":"customer and batch"}}})
	if err != nil || got < 34 || got < 512 {
		t.Fatalf("counter failed to reserve conservative budget count=%d err=%v", got, err)
	}
}

func TestCountChatTokensFailsClosed(t *testing.T) {
	for _, body := range []string{`{"total_tokens":0}`, `{"error":true,"total_tokens":19}`, `not json`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			client, err := NewClient(Config{BaseURL: server.URL, APIKey: "private-key"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.CountChatTokens(context.Background(), map[string]any{"model":"model","messages":[]map[string]string{{"role":"user","content":"batch"}}}); err == nil ||
				strings.Contains(err.Error(), "batch") {
				t.Fatalf("unsafe count accepted: %v", err)
			}
		})
	}
}

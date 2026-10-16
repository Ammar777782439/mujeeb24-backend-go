package litellm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClientRejectsInvalidConfiguration(t *testing.T) {
	for _, base := range []string{"", "file:///tmp/secret", "https://u:p@example.com", "https://example.com/?token=secret"} {
		if _, err := NewClient(Config{BaseURL: base, APIKey: "key"}); err == nil {
			t.Fatalf("accepted invalid baseURL %q", base)
		}
	}
	if _, err := NewClient(Config{BaseURL: "https://example.com", APIKey: " "}); err == nil {
		t.Fatal("missing key accepted")
	}
}

func TestSendInteractionSendsAuthenticatedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1beta/interactions" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer private-token" {
			t.Errorf("missing bearer auth")
		}
		var input map[string]any
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input["model"] != "mujeeb-customer" {
			t.Errorf("unexpected payload: %v %v", input, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"completed","outputs":[]}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "private-token"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := client.SendInteraction(context.Background(), map[string]any{"model": "mujeeb-customer"})
	if err != nil || !json.Valid(raw) {
		t.Fatalf("send interaction: %v, %s", err, raw)
	}
}

func TestSendInteractionDoesNotLeakProviderBodyOrSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("sensitive customer transcript and api-key"))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "secret-key"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendInteraction(context.Background(), map[string]any{"input": "customer secrets"})
	var status *HTTPError
	if !errors.As(err, &status) || !status.Retryable() || status.StatusCode != 503 {
		t.Fatalf("expected retryable 503, got %v", err)
	}
	if strings.Contains(err.Error(), "sensitive") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("upstream data leaked through error: %v", err)
	}
	if (&HTTPError{StatusCode: 400}).Retryable() || (&HTTPError{StatusCode: 403}).Retryable() {
		t.Fatal("unsafe retry of non-transient errors")
	}
}

func TestSendInteractionRespectsCanceledContext(t *testing.T) {
	client, err := NewClient(Config{BaseURL: "https://example.com", APIKey: "key", Timeout: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.SendInteraction(ctx, map[string]any{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled context, got %v", err)
	}
}

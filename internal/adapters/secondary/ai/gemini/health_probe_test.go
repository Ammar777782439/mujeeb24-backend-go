package gemini

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthCheckProbeHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-test" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Fatalf("missing api key header")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := NewGeminiHTTPClient(GeminiHTTPClientConfig{
		BaseURL:        srv.URL,
		APIKey:         "test-key",
		Model:          "gemini-test",
		HTTPClient:     srv.Client(),
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	state, _, failure := NewHealthCheckProbe(client).Probe(context.Background())
	if state != "HEALTHY" || failure != nil {
		t.Fatalf("state=%s failure=%v", state, failure)
	}
}

func TestHealthCheckProbeAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client, err := NewGeminiHTTPClient(GeminiHTTPClientConfig{
		BaseURL:        srv.URL,
		APIKey:         "bad-key",
		Model:          "gemini-test",
		HTTPClient:     srv.Client(),
		RequestTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	state, _, failure := NewHealthCheckProbe(client).Probe(context.Background())
	if state != "DOWN" || failure == nil || *failure != "AUTH_FAILED" {
		t.Fatalf("state=%s failure=%v", state, failure)
	}
}

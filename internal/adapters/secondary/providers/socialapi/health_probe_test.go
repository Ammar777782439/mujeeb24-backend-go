package socialapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthCheckProbeHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts" || r.URL.Query().Get("limit") != "1" {
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":0,"data":[]}`))
	}))
	defer srv.Close()

	client := NewClient(Config{BaseURL: srv.URL, APIKey: "test-key", HTTPClient: srv.Client(), HTTPTimeout: time.Second})
	state, _, failure := NewHealthCheckProbe(client).Probe(context.Background())
	if state != "HEALTHY" || failure != nil {
		t.Fatalf("state=%s failure=%v", state, failure)
	}
}

func TestHealthCheckProbeFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewClient(Config{BaseURL: srv.URL, APIKey: "test-key", HTTPClient: srv.Client(), HTTPTimeout: time.Second})
	state, _, failure := NewHealthCheckProbe(client).Probe(context.Background())
	if state != "DOWN" || failure == nil || *failure != "SOCIALAPI_REQUEST_FAILED" {
		t.Fatalf("state=%s failure=%v", state, failure)
	}
}

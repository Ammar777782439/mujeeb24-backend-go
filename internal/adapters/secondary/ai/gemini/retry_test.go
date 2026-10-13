package gemini

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

func fastGeminiRetryPolicy() geminiRetryPolicy {
	return geminiRetryPolicy{maxAttempts: 4, initialDelay: time.Millisecond, maxDelay: 2 * time.Millisecond}
}

func TestGeminiRetryRecoversTransientProviderFailures(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/interactions" || r.Method != http.MethodPost ||
			r.Header.Get("x-goog-api-key") != "test-key" {
			t.Errorf("wrong request path or credentials")
		}
		if attempts.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("busy"))
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	resp, err := doGeminiPOSTWithRetry(context.Background(), srv.Client(), srv.URL+"/v1/interactions", "test-key", []byte("{}"), fastGeminiRetryPolicy(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" || attempts.Load() != 3 {
		t.Fatalf("got %q after %d attempts", body, attempts.Load())
	}
}

func TestGeminiRetrySkipsPermanentClientError(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	resp, err := doGeminiPOSTWithRetry(context.Background(), srv.Client(), srv.URL, "test", []byte("{}"), fastGeminiRetryPolicy(), true)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 || attempts.Load() != 1 {
		t.Fatalf("unexpected retry for permanent failure: %d, attempts=%d", resp.StatusCode, attempts.Load())
	}
}

func TestGeminiRetryExhaustsBoundedAttempts(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	resp, err := doGeminiPOSTWithRetry(context.Background(), srv.Client(), srv.URL, "test", []byte("{}"), geminiRetryPolicy{maxAttempts: 3, initialDelay: time.Millisecond, maxDelay: 2 * time.Millisecond}, true)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 429 || attempts.Load() != 3 {
		t.Fatalf("unexpected result: %d, attempts=%d", resp.StatusCode, attempts.Load())
	}
}

func TestGeminiRetryAfterTooLongDoesNotRetryEarly(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	resp, err := doGeminiPOSTWithRetry(context.Background(), srv.Client(), srv.URL, "test", []byte("{}"), fastGeminiRetryPolicy(), true)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if attempts.Load() != 1 {
		t.Fatal("violated provider Retry-After")
	}
}

func TestGeminiRetryCancelledDuringBackoff(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(503)
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	resp, err := doGeminiPOSTWithRetry(ctx, srv.Client(), srv.URL, "test", []byte("{}"), geminiRetryPolicy{maxAttempts: 4, initialDelay: time.Second, maxDelay: 2 * time.Second}, true)
	if resp != nil {
		resp.Body.Close()
		t.Fatal("unexpected response")
	}
	if !errors.Is(err, context.DeadlineExceeded) || attempts.Load() != 1 {
		t.Fatalf("cancellation not respected: %v, calls=%d", err, attempts.Load())
	}
}

func TestGeminiRetryPolicyEnvironmentValidation(t *testing.T) {
	t.Setenv("GEMINI_RETRY_MAX_ATTEMPTS", "3")
	t.Setenv("GEMINI_RETRY_INITIAL_DELAY", "150ms")
	t.Setenv("GEMINI_RETRY_MAX_DELAY", "2s")
	p, err := loadGeminiRetryPolicy()
	if err != nil || p.maxAttempts != 3 || p.initialDelay != 150*time.Millisecond || p.maxDelay != 2*time.Second {
		t.Fatalf("policy: %+v err=%v", p, err)
	}
	t.Setenv("GEMINI_RETRY_MAX_ATTEMPTS", "0")
	if _, err := loadGeminiRetryPolicy(); err == nil {
		t.Fatal("invalid policy should fail closed")
	}
}

func TestGeminiCustomerSelectionRetriesAndPreservesStatelessRequest(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), "\"store\":false") {
			t.Error("customer interaction must remain stateless")
		}
		if attempts.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`{"status":"requires_action","steps":[{"type":"function_call","id":"catalog","name":"evaluate_full_catalog","arguments":{}}],"usage":{"total_input_tokens":31,"total_output_tokens":8}}`))
	}))
	defer srv.Close()
	base, err := NewGeminiHTTPClient(GeminiHTTPClientConfig{BaseURL: srv.URL, APIKey: "test", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewGeminiCustomerSalesAdapter(base)
	if err != nil {
		t.Fatal(err)
	}
	adapter.retryPolicy = fastGeminiRetryPolicy()
	output, err := adapter.Decide(context.Background(), ports.CustomerSalesDecisionInput{
		Request: ports.CustomerSalesDecisionRequest{Text: "أريد الاشتراك"},
		Capabilities: ports.CustomerSalesCapabilities(true),
	})
	if err != nil || output.Decision.Capability != ports.CustomerSalesCapabilityFullCatalog || attempts.Load() != 2 {
		t.Fatalf("decision=%+v, err=%v, calls=%d", output.Decision, err, attempts.Load())
	}
}

func TestGeminiBatchEvaluationRetriesOnlyFailedBatch(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`{"status":"completed","steps":[{"type":"model_output","content":[{"type":"text","text":"{\\"candidates\\":[]}"}]}],"usage":{"total_input_tokens":125,"total_output_tokens":9}}`))
	}))
	defer srv.Close()
	batch, err := NewBatchClient(BatchClientConfig{BaseURL: srv.URL, APIKey: "test", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	batch.retryPolicy = fastGeminiRetryPolicy()
	input := services.BatchEvaluationInput{Batch: services.CatalogAIBatchPayload{Items: []services.CatalogAIItem{{}}}}
	result, err := batch.EvaluateBatch(context.Background(), input)
	if err != nil || attempts.Load() != 2 || len(result.Candidates) != 0 {
		t.Fatalf("batch=%+v err=%v calls=%d", result, err, attempts.Load())
	}
}

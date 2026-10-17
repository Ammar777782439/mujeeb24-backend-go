package litellm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLiteLLMModelDiscoveryUsesVirtualKeyAndAliases(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls++
		if req.Method != http.MethodGet || req.URL.Path != "/v1/models" ||
			req.Header.Get("Authorization") != "Bearer sk-test-gateway" ||
			req.Header.Get("x-goog-api-key") != "" || req.URL.RawQuery != "" {
			t.Errorf("invalid gateway discovery request: %s %s", req.Method, req.URL)
		}
		_, _ = fmt.Fprint(w, `{"object":"list","data":[{"id":"mujeeb-sales","object":"model"},{"id":"mujeeb-catalog"},{"id":"mujeeb-merchant"},{"id":"mujeeb-sales"}]}`)
	}))
	defer srv.Close()
	models, err := NewModelsClient().DiscoverModels(context.Background(), "sk-test-gateway", srv.URL)
	if err != nil || calls != 1 || len(models) != 3 {
		t.Fatalf("discovery failed: calls=%d models=%+v err=%v", calls, models, err)
	}
	expected := []string{"mujeeb-sales", "mujeeb-catalog", "mujeeb-merchant"}
	for i, model := range models {
		if model.ModelName != expected[i] || model.Provider != "litellm" ||
			model.InputTokenLimit != nil || model.OutputTokenLimit != nil ||
			model.ThinkingSupported || len(model.SupportedMethods) != 0 {
			t.Fatalf("discovery invented upstream capabilities or aliased wrong: %+v", model)
		}
	}
}

func TestLiteLLMModelConnectionProbesRuntimeChatCompletions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/v1/chat/completions" ||
			req.Header.Get("Authorization") != "Bearer sk-probe" {
			t.Errorf("wrong probe method, route or credential")
		}
		body := make([]byte, 512)
		n, _ := req.Body.Read(body)
		if !strings.Contains(string(body[:n]), `"model":"mujeeb-sales"`) ||
			!strings.Contains(string(body[:n]), `"content":"Hello"`) {
			t.Errorf("probe missing exact runtime model or minimal input")
		}
		_, _ = fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"OK"}}]}`)
	}))
	defer srv.Close()
	ok, _, code := NewModelsClient().TestConnection(context.Background(), "sk-probe", "mujeeb-sales", srv.URL)
	if !ok || code != "" {
		t.Fatalf("valid gateway probe failed: %v %q", ok, code)
	}
}

func TestLiteLLMModelDiscoveryRejectsFailuresAndDoesNotLeakResponse(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		status         int
	}{
		{"unauthorized", "PRIVATE_UPSTREAM_KEY", http.StatusUnauthorized},
		{"invalid schema", `{"models":[{"name":"gemini"}]}`, http.StatusOK},
		{"invalid JSON", "{", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.response)
			}))
			defer srv.Close()
			_, err := NewModelsClient().DiscoverModels(context.Background(), "sk-private", srv.URL)
			if err == nil || strings.Contains(err.Error(), "PRIVATE_UPSTREAM_KEY") || strings.Contains(err.Error(), "sk-private") {
				t.Fatalf("discovery should fail without leaking secrets: %v", err)
			}
		})
	}
}

func TestLiteLLMModelProbeRejectsIncompleteAndAuthenticationFailure(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		code       string
	}{
		{"incomplete", `{"choices":[{"finish_reason":"length","message":{"role":"assistant","content":"partial"}}]}`, 200, "LITELLM_INCOMPLETE_RESPONSE"},
		{"unauthorized", "credential detail should remain private", 401, "LITELLM_HTTP_401"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			ok, _, code := NewModelsClient().TestConnection(context.Background(), "sk-private", "mujeeb-sales", srv.URL)
			if ok || code != tc.code {
				t.Fatalf("incorrect connection probe result: %t, %q", ok, code)
			}
		})
	}
}

package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSMiddlewareAllowsDashboardMutationHeaders(t *testing.T) {
	nextCalled := false
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		nextCalled = true
		writer.WriteHeader(http.StatusOK)
	})

	handler := newCORSMiddleware(next, "https://frontend.example.com/landing")

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/businesses/business-1/catalogs/catalog-1", nil)
	request.Header.Set("Origin", "https://frontend.example.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	request.Header.Set("Access-Control-Request-Headers", "authorization, content-type, if-match, idempotency-key, x-request-id, x-correlation-id")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "https://frontend.example.com" {
		t.Fatalf("allow-origin = %q, want %q", got, "https://frontend.example.com")
	}
	if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow-credentials = %q, want true", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, "PATCH") {
		t.Fatalf("allow-methods = %q, want PATCH", got)
	}

	allowedHeaders := strings.ToLower(response.Header().Get("Access-Control-Allow-Headers"))
	for _, header := range []string{
		"authorization",
		"content-type",
		"if-match",
		"idempotency-key",
		"x-request-id",
		"x-correlation-id",
	} {
		if !strings.Contains(allowedHeaders, header) {
			t.Fatalf("allow-headers = %q, missing %q", allowedHeaders, header)
		}
	}

	if nextCalled {
		t.Fatal("OPTIONS preflight must not call downstream handler")
	}
}

func TestCORSMiddlewareRejectsUnknownOriginPreflight(t *testing.T) {
	nextCalled := false
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		nextCalled = true
		writer.WriteHeader(http.StatusOK)
	})

	handler := newCORSMiddleware(next, "https://frontend.example.com")

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/businesses/business-1/catalogs/catalog-1", nil)
	request.Header.Set("Origin", "https://attacker.example.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	request.Header.Set("Access-Control-Request-Headers", "authorization, content-type, if-match")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if nextCalled {
		t.Fatal("rejected OPTIONS preflight must not call downstream handler")
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("allow-origin = %q, want empty", got)
	}
}

func TestFrontendOriginStripsConfiguredPath(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "ui route", url: "https://frontend.example.com/landing", want: "https://frontend.example.com"},
		{name: "root", url: "https://frontend.example.com", want: "https://frontend.example.com"},
		{name: "invalid", url: "frontend.example.com/landing", want: ""},
		{name: "empty", url: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := frontendOrigin(tt.url); got != tt.want {
				t.Fatalf("frontendOrigin(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

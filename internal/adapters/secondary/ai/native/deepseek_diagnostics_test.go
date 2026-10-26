package native

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"testing"
)

func TestDeepSeekErrorDiagnosticsClassifyHTTPFailureWithoutSecrets(t *testing.T) {
	const secret = "sk-abcdef0123456789real"
	api, closeServer := newFakeDeepSeek(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `{"error":{"code":"invalid_request_error","type":"invalid_request_error","message":"Invalid strict function schema (test). API key %s reference 4c8af066-f65b-47eb-84e2-be6c6ba07c08"}}`, secret)
	})
	defer closeServer()
	var logs bytes.Buffer
	originalWriter := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(originalWriter)
	_, err := api.Generate(context.Background(), deepSeekReq(
		`[{"type":"function","function":{"name":"find_customers","strict":true,"parameters":{"type":"object","properties":{"search":{"type":"string"}},"required":["search"],"additionalProperties":false}}}]`, ""))
	var providerErr *StatusError
	if !errors.As(err, &providerErr) || providerErr.Code != http.StatusBadRequest {
		t.Fatalf("expected preserved HTTP 400 classification, got %v", err)
	}
	out := logs.String()
	for _, required := range []string{"stage=send", "stage=provider_http", "status=400", "code=invalid_request_error", "strict function schema", "REDACTED_SECRET", "REDACTED_ID"} {
		if !strings.Contains(out, required) {
			t.Errorf("missing diagnostic %q from %s", required, out)
		}
	}
	if strings.Contains(out, secret) || strings.Contains(out, "4c8af066-f65b-47eb-84e2-be6c6ba07c08") {
		t.Fatal("diagnostic leaked secret or customer record ID")
	}
}

func TestDeepSeekErrorDiagnosticsHandleUnexpectedUpstreamBody(t *testing.T) {
	code, kind, msg := safeDeepSeekErrorFields([]byte("not-json-upstream"))
	if code != "unparseable" || kind != "unparseable" || !strings.Contains(msg, "invalid JSON") {
		t.Fatalf("unexpected sanitized invalid response: %s %s %s", code, kind, msg)
	}
}

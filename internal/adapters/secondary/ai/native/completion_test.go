package native

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestNativeCompletionPreservesActualProviderUsagePerCall(t *testing.T) {
	primary := &fakeProvider{name: "gemini", err: &StatusError{Provider: "gemini", Code: 503}}
	backup := &fakeProvider{name: "deepseek"}
	router := mkRouter(t, primary, backup, 1)
	client, err := NewCompletionClient(router)
	if err != nil {
		t.Fatal(err)
	}
	raw, info, err := client.Complete(context.Background(), map[string]any{
		"model": "mujeeb-sales", "messages": []any{map[string]any{"role": "user", "content": "hi"}}, "max_tokens": 128,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) || info.Provider != "deepseek" || info.Model != "deepseek" ||
		!info.Trace.FallbackUsed || len(info.Trace.Attempts) != 2 ||
		info.Trace.Attempts[0].HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("incorrect routed completion metadata: %+v", info)
	}
}
func TestNativeCompletionRejectsInvalidRequestWithoutAttempt(t *testing.T) {
	primary, backup := &fakeProvider{name: "gemini"}, &fakeProvider{name: "deepseek"}
	client, err := NewCompletionClient(mkRouter(t, primary, backup, 1))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []any{nil, []string{"not an object"}} {
		if _, _, err := client.Complete(context.Background(), input); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	if primary.count() != 0 || backup.count() != 0 {
		t.Fatal("invalid request reached external provider")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := client.Complete(ctx, map[string]any{"messages": []any{}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel not propagated: %v", err)
	}
}

// A new request can return to Gemini, but a DeepSeek-owned tool continuation
// must not carry its history to Gemini after a transient primary failure.
func TestNativeCompletionPinsProviderOnlyForActiveSequence(t *testing.T) {
	primary := &fakeProvider{name: "gemini", err: &StatusError{Provider: "gemini", Code: 503}}
	standby := &fakeProvider{name: "deepseek"}
	client, err := NewCompletionClient(mkRouter(t, primary, standby, 3))
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hello"}}}
	if _, info, err := client.Complete(context.Background(), payload); err != nil ||
		info.Provider != "deepseek" || !info.Trace.FallbackUsed {
		t.Fatalf("initial fallback failed: %+v %v", info, err)
	}
	primary.err = nil // Gemini has recovered, but cannot own this tool history.
	if _, info, err := client.CompletePinned(context.Background(), payload, "deepseek"); err != nil ||
		info.Provider != "deepseek" || info.Trace.FallbackUsed || len(info.Trace.Attempts) != 1 {
		t.Fatalf("tool continuation switched provider: %+v %v", info, err)
	}
	if primary.count() != 1 || standby.count() != 2 {
		t.Fatal("tool continuation unexpectedly retried Gemini")
	}
	if _, info, err := client.Complete(context.Background(), payload); err != nil ||
		info.Provider != "gemini" || info.Trace.FallbackUsed {
		t.Fatalf("independent request did not reconsider primary: %+v %v", info, err)
	}
}

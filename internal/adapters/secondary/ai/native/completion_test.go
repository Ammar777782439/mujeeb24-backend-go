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

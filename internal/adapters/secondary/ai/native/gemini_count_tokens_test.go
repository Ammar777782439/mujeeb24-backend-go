package native

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestGeminiCountTokensMirrorsGenerationRequest(t *testing.T) {
	calls := 0
	api, closeServer := fakeGemini(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1beta/models/gemini-2.5-flash:countTokens" ||
			r.Header.Get("x-goog-api-key") != "key-private" {
			t.Errorf("wrong count endpoint or authentication: %s", r.URL.Path)
		}
		var body struct {
			GenerateContentRequest struct {
				Model             string          `json:"model"`
				SystemInstruction json.RawMessage `json:"systemInstruction"`
				Contents          []any           `json:"contents"`
				GenerationConfig  json.RawMessage `json:"generationConfig"`
				Tools             []any           `json:"tools"`
			} `json:"generateContentRequest"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		gen := body.GenerateContentRequest
		if gen.Model != "models/gemini-2.5-flash" || len(gen.Contents) != 1 ||
			len(gen.SystemInstruction) == 0 || len(gen.GenerationConfig) == 0 {
			t.Errorf("missing complete generation request: %+v", gen)
		}
		_, _ = fmt.Fprint(w, `{"totalTokens":42}`)
	})
	defer closeServer()
	req := Request{Payload: json.RawMessage(`{"model":"mujeeb-catalog","messages":[{"role":"system","content":"trusted"},{"role":"user","content":"مواصفات الجهاز"}],"max_tokens":512}`)}
	actual, err := api.CountTokens(context.Background(), req)
	if err != nil || actual != 42 || calls != 1 {
		t.Fatalf("actual=%d calls=%d err=%v", actual, calls, err)
	}
}

func TestGeminiThinkingTokensIncludedInUsage(t *testing.T) {
	api, closeServer := fakeGemini(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{"promptTokenCount":10,"toolUsePromptTokenCount":2,"candidatesTokenCount":5,"thoughtsTokenCount":7,"cachedContentTokenCount":1}}`)
	})
	defer closeServer()
	req := Request{Payload: json.RawMessage(`{"model":"sales","messages":[{"role":"user","content":"hello"}],"max_tokens":128}`)}
	got, err := api.Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.InputTokens != 12 || got.OutputTokens != 12 || got.CachedTokens != 1 {
		t.Fatalf("missing billable usage: %+v", got)
	}
	var normalized struct {
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(got.Payload, &normalized); err != nil {
		t.Fatal(err)
	}
	if normalized.Usage.PromptTokens != 12 || normalized.Usage.CompletionTokens != 12 {
		t.Fatalf("usage did not normalize thought/tool tokens: %+v", normalized.Usage)
	}
}

func TestGeminiCountTokensRejectsMissingTotal(t *testing.T) {
	api, closeServer := fakeGemini(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{}`)
	})
	defer closeServer()
	req := Request{Payload: json.RawMessage(`{"model":"sales","messages":[{"role":"user","content":"hi"}],"max_tokens":128}`)}
	if _, err := api.CountTokens(context.Background(), req); err == nil {
		t.Fatal("accepted missing totalTokens")
	}
}

func TestGeminiCountTokensStandbyReported(t *testing.T) {
	api, closeServer := fakeGemini(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"totalTokens":33}`)
	})
	defer closeServer()
	router, err := NewRouter(api, nil, Policy{})
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Payload: json.RawMessage(`{"model":"sales","messages":[{"role":"user","content":"hi"}],"max_tokens":128}`)}
	actual, exact, standby, err := router.CountTokens(context.Background(), req)
	if err != nil || actual != 33 || !exact || standby {
		t.Fatalf("count=%d exact=%t standby=%t error=%v", actual, exact, standby, err)
	}
}

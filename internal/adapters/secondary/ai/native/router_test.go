package native

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

type fakeProvider struct {
	name string
	mu   sync.Mutex
	n    int
	err  error
}

func (p *fakeProvider) Name() string { return p.name }
func (p *fakeProvider) Generate(_ context.Context, _ Request) (Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.n++
	if p.err != nil {
		return Response{}, p.err
	}
	return Response{Payload: json.RawMessage(`{"ok":true}`), Model: p.name}, nil
}
func (p *fakeProvider) count() int { p.mu.Lock(); defer p.mu.Unlock(); return p.n }
func mkRouter(t *testing.T, a, b *fakeProvider, threshold int) *Router {
	t.Helper()
	r, err := NewRouter(a, b, Policy{ConsecutiveFailures: threshold, Cooldown: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func validRequest() Request {
	return Request{Payload: json.RawMessage(`{"messages":[]}`), Operation: "customer"}
}

func TestPrimaryPreferredAndFallbackOn503Only(t *testing.T) {
	a, b := &fakeProvider{name: "gemini"}, &fakeProvider{name: "deepseek"}
	r := mkRouter(t, a, b, 3)
	if _, trace, err := r.Generate(context.Background(), validRequest()); err != nil || trace.FallbackUsed || b.count() != 0 {
		t.Fatalf("primary path: %+v %v", trace, err)
	}
	a.err = &StatusError{Provider: "gemini", Code: http.StatusServiceUnavailable}
	if _, trace, err := r.Generate(context.Background(), validRequest()); err != nil || !trace.FallbackUsed || len(trace.Attempts) != 2 || trace.Attempts[0].HTTPStatus != 503 || b.count() != 1 {
		t.Fatalf("503 fallback: %+v %v", trace, err)
	}
}

func TestNoFallbackOnValidationOrOtherStatuses(t *testing.T) {
	for _, errCase := range []error{errors.New("invalid schema"), &StatusError{Provider: "gemini", Code: 400}, &StatusError{Provider: "gemini", Code: 429}} {
		a, b := &fakeProvider{name: "gemini", err: errCase}, &fakeProvider{name: "deepseek"}
		r := mkRouter(t, a, b, 1)
		if _, trace, err := r.Generate(context.Background(), validRequest()); err == nil || trace.FallbackUsed || b.count() != 0 {
			t.Fatalf("unsafe fallback: %+v %v", trace, err)
		}
	}
}

func TestCircuitBreakerSkipsUnavailablePrimaryAndRecovers(t *testing.T) {
	a, b := &fakeProvider{name: "gemini", err: &StatusError{Provider: "gemini", Code: 503}}, &fakeProvider{name: "deepseek"}
	r := mkRouter(t, a, b, 2)
	now := time.Now()
	r.now = func() time.Time { return now }
	for i := 0; i < 4; i++ {
		if _, _, err := r.Generate(context.Background(), validRequest()); err != nil {
			t.Fatal(err)
		}
	}
	if a.count() != 2 || b.count() != 4 {
		t.Fatalf("breaker failed primary=%d standby=%d", a.count(), b.count())
	}
	a.mu.Lock()
	a.err = nil
	a.mu.Unlock()
	now = now.Add(2 * time.Minute)
	_, trace, err := r.Generate(context.Background(), validRequest())
	if err != nil || trace.FallbackUsed || a.count() != 3 {
		t.Fatalf("recovery failed: %+v %v", trace, err)
	}
}

func TestNoProviderCalledForCanceledOrInvalidRequest(t *testing.T) {
	a, b := &fakeProvider{name: "gemini"}, &fakeProvider{name: "deepseek"}
	r := mkRouter(t, a, b, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := r.Generate(ctx, validRequest()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if _, _, err := r.Generate(context.Background(), Request{Payload: []byte("{")}); err == nil {
		t.Fatal("invalid payload accepted")
	}
	if a.count() != 0 || b.count() != 0 {
		t.Fatal("outbound provider contacted for invalid request")
	}
}

func TestFailedStandbyCannotBeMistakenForSuccess(t *testing.T) {
	a := &fakeProvider{name: "gemini", err: &StatusError{Provider: "gemini", Code: 503}}
	b := &fakeProvider{name: "deepseek", err: &StatusError{Provider: "deepseek", Code: 502}}
	r := mkRouter(t, a, b, 1)
	res, trace, err := r.Generate(context.Background(), validRequest())
	if err == nil || len(res.Payload) > 0 || !trace.FallbackUsed || len(trace.Attempts) != 2 || !trace.Attempts[1].Failed {
		t.Fatalf("standby failure: %+v %+v %v", res, trace, err)
	}
}

func TestGeminiOnlyNeverFallsBackOrOpensCircuit(t *testing.T) {
	primary := &fakeProvider{name: "gemini", err: &StatusError{Provider: "gemini", Code: 503}}
	router, err := NewRouter(primary, nil, Policy{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		response, trace, err := router.Generate(context.Background(), validRequest())
		if !IsUnavailable(err) || trace.FallbackUsed || len(trace.Attempts) != 1 ||
			trace.Attempts[0].Provider != "gemini" || trace.Attempts[0].HTTPStatus != 503 ||
			len(response.Payload) != 0 {
			t.Fatalf("Gemini-only 503 must fail closed: attempt=%d trace=%+v err=%v", i, trace, err)
		}
	}
	if primary.count() != 4 {
		t.Fatalf("Gemini-only must retry only on new requests, got %d calls", primary.count())
	}
	primary.mu.Lock()
	primary.err = nil
	primary.mu.Unlock()
	response, trace, err := router.Generate(context.Background(), validRequest())
	if err != nil || trace.FallbackUsed || len(trace.Attempts) != 1 || primary.count() != 5 ||
		string(response.Payload) != `{"ok":true}` {
		t.Fatalf("Gemini-only recovery failed: response=%+v trace=%+v err=%v", response, trace, err)
	}
}

func TestOptionalStandbyDoesNotRelaxProviderValidation(t *testing.T) {
	if _, err := NewRouter(nil, nil, Policy{}); err == nil {
		t.Fatal("nil primary accepted")
	}
	if _, err := NewRouter(&fakeProvider{name: ""}, nil, Policy{}); err == nil {
		t.Fatal("nameless primary accepted")
	}
	if _, err := NewRouter(&fakeProvider{name: "gemini"}, &fakeProvider{name: "gemini"}, Policy{
		ConsecutiveFailures: 1, Cooldown: time.Minute,
	}); err == nil {
		t.Fatal("same provider incorrectly configured as standby")
	}
}

// A Gemini tool sequence can fail over once to DeepSeek on 503. DeepSeek's
// continuation never attempts Gemini, even when the standby fails.
func TestRouterPinnedContinuationsOnlyFailOverGemini503(t *testing.T) {
	primary := &fakeProvider{name: "gemini", err: &StatusError{Provider: "gemini", Code: 503}}
	standby := &fakeProvider{name: "deepseek"}
	router := mkRouter(t, primary, standby, 3)
	req := validRequest()

	req.PinnedProvider = "deepseek"
	if _, trace, err := router.Generate(context.Background(), req); err != nil ||
		len(trace.Attempts) != 1 || trace.Attempts[0].Provider != "deepseek" || trace.FallbackUsed {
		t.Fatalf("pinned standby was rerouted: trace=%+v err=%v", trace, err)
	}
	if primary.count() != 0 || standby.count() != 1 {
		t.Fatal("pinned DeepSeek request unexpectedly contacted Gemini")
	}

	req.PinnedProvider = "gemini"
	if _, trace, err := router.Generate(context.Background(), req); err != nil ||
		!trace.FallbackUsed || len(trace.Attempts) != 2 ||
		trace.Attempts[0].Provider != "gemini" || trace.Attempts[0].HTTPStatus != 503 ||
		trace.Attempts[1].Provider != "deepseek" {
		t.Fatalf("pinned Gemini 503 did not fail over: trace=%+v err=%v", trace, err)
	}
	if primary.count() != 1 || standby.count() != 2 {
		t.Fatal("pinned Gemini 503 must call DeepSeek exactly once")
	}

	primary.err = &StatusError{Provider: "gemini", Code: 400}
	if _, trace, err := router.Generate(context.Background(), req); err == nil ||
		trace.FallbackUsed || len(trace.Attempts) != 1 || standby.count() != 2 {
		t.Fatalf("pinned non-503 should fail closed: trace=%+v err=%v", trace, err)
	}

	standby.err = &StatusError{Provider: "deepseek", Code: 502}
	req.PinnedProvider = "deepseek"
	if _, trace, err := router.Generate(context.Background(), req); err == nil ||
		trace.FallbackUsed || len(trace.Attempts) != 1 || primary.count() != 2 {
		t.Fatalf("failed DeepSeek was rerouted to Gemini: trace=%+v err=%v", trace, err)
	}

	primary.err = &StatusError{Provider: "gemini", Code: 503}
	req.PinnedProvider = "gemini"
	res, trace, err := router.Generate(context.Background(), req)
	if err == nil || len(res.Payload) > 0 || !trace.FallbackUsed ||
		len(trace.Attempts) != 2 || !trace.Attempts[1].Failed {
		t.Fatalf("failed DeepSeek fallback misreported success: trace=%+v err=%v", trace, err)
	}

	req.PinnedProvider = "unknown"
	beforePrimary, beforeStandby := primary.count(), standby.count()
	if _, _, err := router.Generate(context.Background(), req); err == nil {
		t.Fatal("unknown pinned provider accepted")
	}
	if primary.count() != beforePrimary || standby.count() != beforeStandby {
		t.Fatal("unknown provider contacted the network")
	}
}

func TestRouterPinnedGeminiOnly503DoesNotFailOver(t *testing.T) {
	primary := &fakeProvider{name: "gemini", err: &StatusError{Provider: "gemini", Code: 503}}
	router, err := NewRouter(primary, nil, Policy{})
	if err != nil {
		t.Fatal(err)
	}
	req := validRequest()
	req.PinnedProvider = "gemini"
	if _, trace, err := router.Generate(context.Background(), req); !IsUnavailable(err) ||
		trace.FallbackUsed || len(trace.Attempts) != 1 || primary.count() != 1 {
		t.Fatalf("Gemini-only pinned 503 must fail closed: trace=%+v err=%v", trace, err)
	}
}

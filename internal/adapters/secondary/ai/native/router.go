// Package native isolates provider-specific inference protocols from
// business execution. Its optional routing never executes tools.
package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Request contains a complete canonical inference request, not a business
// command. Provider implementations own wire encoding; no provider executes tools.
type Request struct {
	Payload        json.RawMessage
	Operation      string
	PinnedProvider string // Provider owning this model round; preserve across tool continuations.
}

// Response is validated again by Mujeeb application adapters before any
// capability can enter business authorization/execution.
type Response struct {
	Payload         json.RawMessage
	Model           string
	InputTokens     int64
	OutputTokens    int64
	CachedTokens    int64
	ProviderCostUSD float64
	CostKnown       bool
}

// Provider is a model transport that never mutates business state.
type Provider interface {
	Name() string
	Generate(context.Context, Request) (Response, error)
}

// StatusError contains no response body, prompt, secrets, or customer data.
type StatusError struct {
	Provider string
	Code     int
}

// ProviderHTTPStatus confirms the status was returned by the actual provider.
func (e *StatusError) ProviderHTTPStatus() int { return e.Code }

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s model request HTTP %d", e.Provider, e.Code)
}

// Retryable reports a temporary provider HTTP status to the application
// without importing an adapter package into business services.
func (e *StatusError) Retryable() bool {
	return e != nil && (e.Code == 503 || e.Code == 429 || e.Code == 502 || e.Code == 504)
}

func IsUnavailable(err error) bool {
	var status *StatusError
	return errors.As(err, &status) && status.Code == http.StatusServiceUnavailable
}

type Policy struct {
	ConsecutiveFailures int
	Cooldown            time.Duration
}

type Attempt struct {
	Provider   string
	Failed     bool
	HTTPStatus int
}

type Trace struct {
	Attempts     []Attempt
	FallbackUsed bool
}

// Router is safe for concurrent use. The breaker is deliberately in-memory
// and per-process; sharing breaker state is not required for correctness.
type Router struct {
	primary   Provider
	standby   Provider
	policy    Policy
	mu        sync.Mutex
	failures  int
	openUntil time.Time
	probing   bool
	now       func() time.Time
}

// NewRouter accepts any named provider. A nil standby makes all requests
// use that provider and propagate errors without fallback.
func NewRouter(primary, standby Provider, policy Policy) (*Router, error) {
	if primary == nil || strings.TrimSpace(primary.Name()) == "" {
		return nil, errors.New("a named primary inference provider is required")
	}
	if standby == nil {
		return &Router{primary: primary, now: time.Now}, nil
	}
	if strings.TrimSpace(standby.Name()) == "" || primary.Name() == standby.Name() {
		return nil, errors.New("standby must be named and distinct from primary")
	}
	if policy.ConsecutiveFailures < 1 || policy.Cooldown <= 0 {
		return nil, errors.New("circuit breaker threshold and cooldown must be positive")
	}
	return &Router{primary: primary, standby: standby, policy: policy, now: time.Now}, nil
}

// Generate routes model generation, never business execution. An optional
// standby is used after primary HTTP 503 only; all other errors fail closed.
func (r *Router) Generate(ctx context.Context, req Request) (Response, Trace, error) {
	var trace Trace
	if r == nil {
		return Response{}, trace, errors.New("native model router is not configured")
	}
	if err := ctx.Err(); err != nil {
		return Response{}, trace, err
	}
	if len(req.Payload) == 0 || !json.Valid(req.Payload) {
		return Response{}, trace, errors.New("canonical model request must be valid JSON")
	}
	// Continuations stay pinned to the owning provider, except an eligible
	// primary HTTP 503 may fall back when an alternative is configured.
	if req.PinnedProvider != "" {
		var selected Provider
		switch {
		case req.PinnedProvider == r.primary.Name():
			selected = r.primary
		case r.standby != nil && req.PinnedProvider == r.standby.Name():
			selected = r.standby
		default:
			return Response{}, trace, errors.New("unknown pinned inference provider")
		}
		out, err := selected.Generate(ctx, req)
		trace.Attempts = append(trace.Attempts, makeAttempt(selected.Name(), err))
		if selected == r.primary && r.standby != nil {
			r.observePrimary(err)
			if IsUnavailable(err) {
				return r.generateStandby(ctx, req, trace)
			}
		}
		return out, trace, err
	}
	// Without a standby, attempt the selected provider on every request.
	// Skipping it would result in a false success or a nil-provider panic.
	if r.standby == nil {
		out, err := r.primary.Generate(ctx, req)
		trace.Attempts = append(trace.Attempts, makeAttempt(r.primary.Name(), err))
		if err != nil {
			return Response{}, trace, err
		}
		return out, trace, nil
	}
	tryPrimary := r.reservePrimary()
	if tryPrimary {
		out, err := r.primary.Generate(ctx, req)
		trace.Attempts = append(trace.Attempts, makeAttempt(r.primary.Name(), err))
		r.observePrimary(err)
		if err == nil {
			return out, trace, nil
		}
		if !IsUnavailable(err) || ctx.Err() != nil {
			return Response{}, trace, err
		}
	}
	return r.generateStandby(ctx, req, trace)
}

func (r *Router) generateStandby(ctx context.Context, req Request, trace Trace) (Response, Trace, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, trace, err
	}
	trace.FallbackUsed = true
	out, err := r.standby.Generate(ctx, req)
	trace.Attempts = append(trace.Attempts, makeAttempt(r.standby.Name(), err))
	if err != nil {
		return Response{}, trace, fmt.Errorf("standby inference failed: %w", err)
	}
	return out, trace, nil
}

func makeAttempt(provider string, err error) Attempt {
	a := Attempt{Provider: provider, Failed: err != nil}
	var status *StatusError
	if errors.As(err, &status) {
		a.HTTPStatus = status.Code
	}
	return a
}

func (r *Router) reservePrimary() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.openUntil.IsZero() {
		return true
	}
	if r.now().Before(r.openUntil) || r.probing {
		return false
	}
	r.probing = true
	return true
}

func (r *Router) observePrimary(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.probing = false
	if !IsUnavailable(err) {
		// A different HTTP failure is not proof of continuing 503 overload.
		r.failures = 0
		r.openUntil = time.Time{}
		return
	}
	r.failures++
	if r.failures >= r.policy.ConsecutiveFailures {
		r.openUntil = r.now().Add(r.policy.Cooldown)
	}
}

// CountTokens returns an exact provider count when supported. Distinct
// tokenizers across optional routes require conservative admission.
func (r *Router) CountTokens(ctx context.Context, req Request) (count int64, exact bool, standby bool, err error) {
	if r == nil {
		return 0, false, false, errors.New("native model router is not configured")
	}
	if err := ctx.Err(); err != nil {
		return 0, false, r.standby != nil, err
	}
	// The fallback model uses a different tokenizer. the primary count
	// cannot relax a conservative shared-model context limit, so avoid an
	// extra network call for every proposed catalog partition.
	if r.standby != nil {
		return 0, false, true, nil
	}
	counter, supported := r.primary.(interface {
		CountTokens(context.Context, Request) (int64, error)
	})
	if !supported {
		return 0, false, r.standby != nil, nil
	}
	measured, err := counter.CountTokens(ctx, req)
	if err != nil {
		return 0, false, r.standby != nil, err
	}
	if measured <= 0 {
		return 0, false, r.standby != nil, errors.New("invalid primary token count")
	}
	return measured, true, r.standby != nil, nil
}

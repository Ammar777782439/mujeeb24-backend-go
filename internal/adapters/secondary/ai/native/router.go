// Package native routes model-generation attempts to a directly connected
// Gemini primary and a DeepSeek standby. It never executes business actions.
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
	Payload   json.RawMessage
	Operation string
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

// Provider is one direct inference endpoint. It must not retry or mutate
// commercial state; the router owns operational fallback.
type Provider interface {
	Name() string
	Generate(context.Context, Request) (Response, error)
}

// StatusError contains no response body, prompt, secrets, or customer data.
type StatusError struct {
	Provider string
	Code     int
}

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

func NewRouter(primary, standby Provider, policy Policy) (*Router, error) {
	if primary == nil || standby == nil ||
		strings.TrimSpace(primary.Name()) == "" ||
		strings.TrimSpace(standby.Name()) == "" ||
		primary.Name() == standby.Name() {
		return nil, errors.New("two distinct named inference providers are required")
	}
	if policy.ConsecutiveFailures < 1 || policy.Cooldown <= 0 {
		return nil, errors.New("circuit breaker threshold and cooldown must be positive")
	}
	return &Router{primary: primary, standby: standby, policy: policy, now: time.Now}, nil
}

// Generate retries only the model-generation call, never a tool or business
// execution. Only Gemini HTTP 503 is an automatic DeepSeek fallback trigger.
// Invalid JSON/schema, unauthorized operations, and other HTTP errors fail closed.
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
	if err := ctx.Err(); err != nil {
		return Response{}, trace, err
	}
	trace.FallbackUsed = true
	out, err := r.standby.Generate(ctx, req)
	trace.Attempts = append(trace.Attempts, makeAttempt(r.standby.Name(), err))
	if err != nil {
		return Response{}, trace, fmt.Errorf("DeepSeek standby inference failed: %w", err)
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

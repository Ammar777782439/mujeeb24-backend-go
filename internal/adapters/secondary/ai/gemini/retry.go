package gemini

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// geminiRetryPolicy is infrastructure configuration, not a domain or AI decision.
// Retries are bounded by both MaxAttempts and the caller's overall context deadline.
type geminiRetryPolicy struct {
	maxAttempts  int
	initialDelay time.Duration
	maxDelay     time.Duration
}

func loadGeminiRetryPolicy() (geminiRetryPolicy, error) {
	p := geminiRetryPolicy{maxAttempts: 4, initialDelay: time.Second, maxDelay: 8 * time.Second}
	if value, ok := os.LookupEnv("GEMINI_RETRY_MAX_ATTEMPTS"); ok {
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 1 || n > 8 {
			return p, errors.New("GEMINI_RETRY_MAX_ATTEMPTS must be an integer between 1 and 8")
		}
		p.maxAttempts = n
	}
	for _, setting := range []struct {
		name   string
		target *time.Duration
	}{
		{"GEMINI_RETRY_INITIAL_DELAY", &p.initialDelay},
		{"GEMINI_RETRY_MAX_DELAY", &p.maxDelay},
	} {
		if value, ok := os.LookupEnv(setting.name); ok {
			d, err := time.ParseDuration(strings.TrimSpace(value))
			if err != nil || d < 100*time.Millisecond || d > 60*time.Second {
				return p, fmt.Errorf("%s must be between 100ms and 60s", setting.name)
			}
			*setting.target = d
		}
	}
	if p.initialDelay > p.maxDelay {
		return p, errors.New("GEMINI_RETRY_INITIAL_DELAY must not exceed GEMINI_RETRY_MAX_DELAY")
	}
	return p, nil
}

func isRetryableGeminiStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests ||
		(status >= http.StatusInternalServerError && status <= 599)
}

func retryableGeminiTransportError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// geminiRetryDelay returns equal-jitter exponential backoff; it never exceeds maxDelay.
// The sequence is independently jittered for each request to avoid retry storms.
func geminiRetryDelay(policy geminiRetryPolicy, failedAttempt int) time.Duration {
	capDelay := policy.initialDelay
	for i := 1; i < failedAttempt && capDelay < policy.maxDelay; i++ {
		if capDelay > policy.maxDelay/2 {
			capDelay = policy.maxDelay
		} else {
			capDelay *= 2
		}
	}
	if capDelay > policy.maxDelay {
		capDelay = policy.maxDelay
	}
	half := capDelay / 2
	if half <= 0 {
		return capDelay
	}
	return half + time.Duration(rand.Int63n(int64(capDelay-half)+1))
}

// Respect Retry-After on rate limits or outages. If the provider requests a
// pause longer than the configured budget, do not retry prematurely.
func geminiRetryAfter(header string, now time.Time) time.Duration {
	if header == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil {
		if seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
		return 0
	}
	if at, err := http.ParseTime(header); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}

// geminiRetryTrace carries safe observability metadata. Never attach credentials,
// user input, prompt text, or raw provider errors to these logs.
type geminiRetryTrace struct {
	stage  string
	model  string
	source string
}

var geminiRequestSequence atomic.Uint64

func geminiConfigSource(version int) string {
	if version > 0 {
		return "db_active"
	}
	return "env_bootstrap"
}

func safeGeminiLogText(s string) string {
	if s == "" {
		return "unspecified"
	}
	return strings.NewReplacer("\n", "_", "\r", "_", "\t", "_", " ", "_").Replace(s)
}

// doGeminiPOSTWithRetry retries provider failures only. The payload and API key
// are never logged. Callers MUST close the returned response body.
// retryTransport is allowed only for stateless, non-executing Gemini requests;
// ambiguous network failures of stateful interactions are not replayed.
func doGeminiPOSTWithRetry(
	ctx context.Context, client *http.Client, url, apiKey string, payload []byte,
	policy geminiRetryPolicy, retryTransport bool, metadata ...geminiRetryTrace,
) (*http.Response, error) {
	trace := geminiRetryTrace{stage: "unspecified", model: "unspecified", source: "unspecified"}
	if len(metadata) > 0 {
		trace = metadata[0]
	}
	trace.stage = safeGeminiLogText(trace.stage)
	trace.model = safeGeminiLogText(trace.model)
	trace.source = safeGeminiLogText(trace.source)
	requestID := geminiRequestSequence.Add(1)
	started := time.Now()
	log.Printf("[Gemini] REQUEST_START request=%d stage=%s model=%s config_source=%s max_attempts=%d", requestID, trace.stage, trace.model, trace.source, policy.maxAttempts)
	logEnd := func(outcome, reason string, attempt, status int) {
		log.Printf("[Gemini] REQUEST_END request=%d stage=%s model=%s outcome=%s reason=%s attempts=%d/%d status=%d elapsed_ms=%d",
			requestID, trace.stage, trace.model, outcome, reason, attempt, policy.maxAttempts, status, time.Since(started).Milliseconds())
	}
	for attempt := 1; attempt <= policy.maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			logEnd("failed", "context_cancelled", attempt-1, 0)
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			logEnd("failed", "invalid_request", attempt-1, 0)
			return nil, fmt.Errorf("build Gemini request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", apiKey)
		log.Printf("[Gemini] ATTEMPT_START request=%d stage=%s attempt=%d/%d", requestID, trace.stage, attempt, policy.maxAttempts)
		attemptStarted := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[Gemini] ATTEMPT_RESULT request=%d stage=%s attempt=%d/%d result=transport_error elapsed_ms=%d",
				requestID, trace.stage, attempt, policy.maxAttempts, time.Since(attemptStarted).Milliseconds())
			if ctx.Err() != nil {
				logEnd("failed", "context_cancelled", attempt, 0)
				return nil, ctx.Err()
			}
			if !retryTransport || !retryableGeminiTransportError(err) {
				logEnd("failed", "transport_not_retryable", attempt, 0)
				return nil, fmt.Errorf("Gemini transport: %w", err)
			}
			if attempt == policy.maxAttempts {
				logEnd("failed", "attempt_limit", attempt, 0)
				return nil, fmt.Errorf("Gemini transport: %w", err)
			}
			delay := geminiRetryDelay(policy, attempt)
			log.Printf("[Gemini] RETRY_SCHEDULED request=%d stage=%s after_attempt=%d reason=transport_error delay=%s",
				requestID, trace.stage, attempt, delay)
			if err := sleepGeminiRetry(ctx, delay); err != nil {
				logEnd("failed", "context_cancelled_during_backoff", attempt, 0)
				return nil, err
			}
			continue
		}
		log.Printf("[Gemini] ATTEMPT_RESULT request=%d stage=%s attempt=%d/%d status=%d elapsed_ms=%d",
			requestID, trace.stage, attempt, policy.maxAttempts, resp.StatusCode, time.Since(attemptStarted).Milliseconds())
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			logEnd("success", "http_success", attempt, resp.StatusCode)
			return resp, nil
		}
		if !isRetryableGeminiStatus(resp.StatusCode) {
			logEnd("failed", "status_not_retryable", attempt, resp.StatusCode)
			return resp, nil
		}
		if attempt == policy.maxAttempts {
			logEnd("failed", "attempt_limit", attempt, resp.StatusCode)
			return resp, nil
		}
		delay := geminiRetryDelay(policy, attempt)
		if serverDelay := geminiRetryAfter(resp.Header.Get("Retry-After"), time.Now()); serverDelay > delay {
			delay = serverDelay
		}
		if delay > policy.maxDelay {
			log.Printf("[Gemini] RETRY_SKIPPED request=%d stage=%s attempt=%d reason=retry_after_exceeds_max_delay required_delay=%s max_delay=%s",
				requestID, trace.stage, attempt, delay, policy.maxDelay)
			logEnd("failed", "retry_after_exceeds_max_delay", attempt, resp.StatusCode)
			return resp, nil
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 16<<10))
		_ = resp.Body.Close()
		log.Printf("[Gemini] RETRY_SCHEDULED request=%d stage=%s after_attempt=%d status=%d delay=%s",
			requestID, trace.stage, attempt, resp.StatusCode, delay)
		if err := sleepGeminiRetry(ctx, delay); err != nil {
			logEnd("failed", "context_cancelled_during_backoff", attempt, 0)
			return nil, err
		}
	}
	logEnd("failed", "attempt_limit", policy.maxAttempts, 0)
	return nil, errors.New("Gemini retry attempts exhausted")
}

func sleepGeminiRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

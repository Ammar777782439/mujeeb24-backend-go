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

// doGeminiPOSTWithRetry retries provider failures only. The payload and API key
// are never logged. Callers MUST close the returned response body.
// retryTransport is allowed only for stateless, non-executing Gemini requests;
// ambiguous network failures of stateful interactions are not replayed.
func doGeminiPOSTWithRetry(
	ctx context.Context, client *http.Client, url, apiKey string, payload []byte,
	policy geminiRetryPolicy, retryTransport bool,
) (*http.Response, error) {
	for attempt := 1; attempt <= policy.maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("build Gemini request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", apiKey)
		resp, err := client.Do(req)
		if err != nil {
			if !retryTransport || !retryableGeminiTransportError(err) || attempt == policy.maxAttempts || ctx.Err() != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				return nil, fmt.Errorf("Gemini transport: %w", err)
			}
		} else {
			if !isRetryableGeminiStatus(resp.StatusCode) || attempt == policy.maxAttempts {
				return resp, nil
			}
			delay := geminiRetryDelay(policy, attempt)
			if serverDelay := geminiRetryAfter(resp.Header.Get("Retry-After"), time.Now()); serverDelay > delay {
				delay = serverDelay
			}
			// A Retry-After beyond our bounded policy is not permission to hammer
			// the provider earlier. Surface the HTTP error to the caller instead.
			if delay > policy.maxDelay {
				return resp, nil
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 16<<10))
			_ = resp.Body.Close()
			log.Printf("[Gemini] RETRY status=%d attempt=%d/%d delay=%s", resp.StatusCode, attempt, policy.maxAttempts, delay)
			if err := sleepGeminiRetry(ctx, delay); err != nil {
				return nil, err
			}
			continue
		}
		delay := geminiRetryDelay(policy, attempt)
		log.Printf("[Gemini] RETRY transport attempt=%d/%d delay=%s", attempt, policy.maxAttempts, delay)
		if err := sleepGeminiRetry(ctx, delay); err != nil {
			return nil, err
		}
	}
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

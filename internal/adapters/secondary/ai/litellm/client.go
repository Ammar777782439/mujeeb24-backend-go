// Package litellm provides the HTTP boundary to a self-hosted LiteLLM gateway.
// It transports model requests only; business capabilities execute in Mujeeb.
package litellm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 4 << 20

// Config contains the private gateway connection, not upstream provider keys.
type Config struct {
	BaseURL    string
	APIKey     string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// Client is transport-only. The caller owns schemas, validation, and execution.
type Client struct {
	endpoint string
	apiKey   string
	client   *http.Client
	timeout  time.Duration
}

func NewClient(cfg Config) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("LiteLLM BaseURL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("LiteLLM API key is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	}
	return &Client{
		endpoint: strings.TrimRight(u.String(), "/") + "/v1beta/interactions",
		apiKey:   cfg.APIKey, client: client, timeout: cfg.Timeout,
	}, nil
}

// HTTPError deliberately contains no provider response body, prompt or secrets.
type HTTPError struct{ StatusCode int }

func (e *HTTPError) Error() string { return fmt.Sprintf("LiteLLM interactions HTTP %d", e.StatusCode) }
func (e *HTTPError) Retryable() bool {
	return e.StatusCode == http.StatusRequestTimeout || e.StatusCode == http.StatusTooManyRequests ||
		e.StatusCode == http.StatusBadGateway || e.StatusCode == http.StatusServiceUnavailable ||
		e.StatusCode == http.StatusGatewayTimeout || e.StatusCode == http.StatusInternalServerError
}

// SendInteraction invokes a single proxy request, without nested retries.
// LiteLLM owns upstream routing. Context cancellation is authoritative.
func (c *Client) SendInteraction(ctx context.Context, payload any) (json.RawMessage, error) {
	return c.sendJSON(ctx, c.endpoint, payload)
}

// SendChatCompletion is used for stateless, explicitly replayed B2B tool
// exchanges. All provider credentials and routing stay inside LiteLLM.
func (c *Client) SendChatCompletion(ctx context.Context, payload any) (json.RawMessage, error) {
	if c == nil {
		return nil, errors.New("LiteLLM client is not configured")
	}
	endpoint := strings.TrimSuffix(c.endpoint, "/v1beta/interactions") + "/v1/chat/completions"
	return c.sendJSON(ctx, endpoint, payload)
}

func (c *Client) sendJSON(ctx context.Context, endpoint string, payload any) (json.RawMessage, error) {
	if c == nil {
		return nil, errors.New("LiteLLM client is not configured")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal LiteLLM request: %w", err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build LiteLLM request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		if requestCtx.Err() != nil {
			return nil, requestCtx.Err()
		}
		return nil, fmt.Errorf("send LiteLLM request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Never log or return raw upstream error bodies (they may contain user text).
		return nil, &HTTPError{StatusCode: resp.StatusCode}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read LiteLLM response: %w", err)
	}
	if len(raw) > maxResponseBytes {
		return nil, errors.New("LiteLLM response exceeds size limit")
	}
	if !json.Valid(raw) {
		return nil, errors.New("LiteLLM response is not JSON")
	}
	return raw, nil
}

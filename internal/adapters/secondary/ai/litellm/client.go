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
		endpoint: strings.TrimRight(u.String(), "/") + "/v1/chat/completions",
		apiKey:   cfg.APIKey, client: client, timeout: cfg.Timeout,
	}, nil
}

// HTTPError deliberately contains no provider response body, prompt or secrets.
type HTTPError struct{ StatusCode int }

func (e *HTTPError) Error() string { return fmt.Sprintf("LiteLLM chat completions HTTP %d", e.StatusCode) }
func (e *HTTPError) Retryable() bool {
	return e.StatusCode == http.StatusRequestTimeout || e.StatusCode == http.StatusTooManyRequests ||
		e.StatusCode == http.StatusBadGateway || e.StatusCode == http.StatusServiceUnavailable ||
		e.StatusCode == http.StatusGatewayTimeout || e.StatusCode == http.StatusInternalServerError
}

// SendChatCompletion transports one stateless Chat Completions request.
// No application-level retry: LiteLLM owns provider routing and retries.
func (c *Client) SendChatCompletion(ctx context.Context, payload any) (json.RawMessage, error) {
	body, _, err := c.SendChatCompletionWithMetadata(ctx, payload)
	return body, err
}

// SendChatCompletionWithMetadata preserves native gateway spend attribution.
func (c *Client) SendChatCompletionWithMetadata(ctx context.Context, payload any) (json.RawMessage, ResponseMetadata, error) {
	if c == nil {
		return nil, ResponseMetadata{}, errors.New("LiteLLM client is not configured")
	}
	return c.sendJSONWithMetadata(ctx, c.endpoint, payload)
}

func (c *Client) sendJSON(ctx context.Context, endpoint string, payload any) (json.RawMessage, error) {
	body, _, err := c.sendJSONWithMetadata(ctx, endpoint, payload)
	return body, err
}

func (c *Client) sendJSONWithMetadata(ctx context.Context, endpoint string, payload any) (json.RawMessage, ResponseMetadata, error) {
	if c == nil {
		return nil, ResponseMetadata{}, errors.New("LiteLLM client is not configured")
	}
	if err := ctx.Err(); err != nil {
		return nil, ResponseMetadata{}, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, ResponseMetadata{}, fmt.Errorf("marshal LiteLLM request: %w", err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, ResponseMetadata{}, fmt.Errorf("build LiteLLM request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	applySpendAttribution(req)
	resp, err := c.client.Do(req)
	if err != nil {
		if requestCtx.Err() != nil {
			return nil, ResponseMetadata{}, requestCtx.Err()
		}
		return nil, ResponseMetadata{}, fmt.Errorf("send LiteLLM request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Never log or return raw upstream error bodies (they may contain user text).
		return nil, ResponseMetadata{}, &HTTPError{StatusCode: resp.StatusCode}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, ResponseMetadata{}, fmt.Errorf("read LiteLLM response: %w", err)
	}
	if len(raw) > maxResponseBytes {
		return nil, ResponseMetadata{}, errors.New("LiteLLM response exceeds size limit")
	}
	if !json.Valid(raw) {
		return nil, ResponseMetadata{}, errors.New("LiteLLM response is not JSON")
	}
	meta, err := metadataFromHeaders(resp.Header)
	if err != nil {
		return nil, ResponseMetadata{}, err
	}
	return raw, meta, nil
}

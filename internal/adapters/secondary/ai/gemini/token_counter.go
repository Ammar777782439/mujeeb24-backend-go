// Package gemini — Token Counter implementation (contract ② §2).
//
// Implements the services.TokenCounter interface against the Gemini
// countTokens API. Per contract ② §2, the Contract is token-based, not
// character or byte based — the model's own tokenizer is authoritative.
//
// Per contract ② §2: "Google توفر count_tokens لهذا الغرض تحديدًا، وحدود
// الـContext تختلف حسب النموذج، ويمكن معرفة الحد برمجيًا من معلومات النموذج."
//
// Per contract ② §2: "إذن حجم الـBatch = Token-based، وليس Item-count-based."
//
// This implementation calls the Gemini REST API:
//   POST /v1beta/models/{model}:countTokens?key={apiKey}
//   Body: {"contents":[{"parts":[{"text":"<serialized JSON>"}]}]}
//   Response: {"totalTokens": <int>}
//
// For complex payloads (structs/maps), we JSON-serialize first and count the
// tokens of the JSON byte array. Per contract ② §2, this gives the model's
// authoritative token count for the payload.

package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TokenCounterConfig configures the TokenCounter.
type TokenCounterConfig struct {
	BaseURL        string
	APIKey         string
	Model          string
	HTTPClient     *http.Client
	RequestTimeout time.Duration
}

// TokenCounter implements services.TokenCounter via the Gemini countTokens API.
//
// Per contract ② §2, this is the authoritative token count for the model's
// own tokenizer. The Contract is token-based, NOT character or byte based.
type TokenCounter struct {
	baseURL        string
	apiKey         string
	model          string
	httpClient     *http.Client
	requestTimeout time.Duration
}

// NewTokenCounter wires the TokenCounter with the Gemini API credentials.
func NewTokenCounter(cfg TokenCounterConfig) (*TokenCounter, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("gemini API key is required for token counting")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return &TokenCounter{
		baseURL:        baseURL,
		apiKey:         cfg.APIKey,
		model:          model,
		httpClient:     client,
		requestTimeout: timeout,
	}, nil
}

// CountTokens implements services.TokenCounter.CountTokens per contract ② §2.
//
// For complex payloads (structs/maps/slices), we JSON-serialize first and
// count the tokens of the resulting JSON string. Per contract ② §2, this
// gives the model's authoritative token count.
//
// Per contract ⑨ §11, every external operation has a Timeout. We use a
// context with the configured RequestTimeout.
func (c *TokenCounter) CountTokens(ctx context.Context, payload any) (int, error) {
	if c == nil {
		return 0, errors.New("token counter is not configured")
	}
	if payload == nil {
		return 0, nil
	}
	// Serialize the payload to JSON. For strings, use directly.
	var text string
	switch v := payload.(type) {
	case string:
		text = v
	case []byte:
		text = string(v)
	default:
		buf, err := json.Marshal(payload)
		if err != nil {
			return 0, fmt.Errorf("marshal payload for token counting: %w", err)
		}
		text = string(buf)
	}
	if strings.TrimSpace(text) == "" {
		return 0, nil
	}

	// Per contract ② §2, call the Gemini countTokens API.
	reqBody := countTokensRequest{
		Contents: []countTokensContent{
			{Parts: []countTokensPart{{Text: text}}},
		},
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("marshal countTokens request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:countTokens?key=%s", c.baseURL, c.model, c.apiKey)
	reqCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return 0, fmt.Errorf("build countTokens request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return 0, fmt.Errorf("send countTokens request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return 0, fmt.Errorf("read countTokens response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("countTokens http %d: %s", resp.StatusCode, string(body))
	}

	var out countTokensResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, fmt.Errorf("unmarshal countTokens response: %w", err)
	}
	return out.TotalTokens, nil
}

// countTokensRequest is the Gemini countTokens API request body.
type countTokensRequest struct {
	Contents []countTokensContent `json:"contents"`
}

type countTokensContent struct {
	Parts []countTokensPart `json:"parts"`
}

type countTokensPart struct {
	Text string `json:"text"`
}

// countTokensResponse is the Gemini countTokens API response.
type countTokensResponse struct {
	TotalTokens int `json:"totalTokens"`
}

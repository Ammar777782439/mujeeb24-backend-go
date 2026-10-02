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
//   POST /v1beta/models/{model}:countTokens (key via x-goog-api-key header)
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

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
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
//
// Per P1-4: when configProvider is wired (via SetConfigurationProvider),
// every CountTokens call reads the ACTIVE runtime config (apiKey + model
// + baseURL) — NOT the static struct fields. This keeps tokenization
// consistent with the ContractClient/BatchClient path: after a model
// switch via the Platform Admin API, countTokens uses the same new
// model as the actual generateContent calls.
type TokenCounter struct {
	baseURL        string
	apiKey         string
	model          string
	httpClient     *http.Client
	requestTimeout time.Duration
	// configProvider, when set, is called at the start of every CountTokens
	// call to read the ACTIVE runtime config. Per P1-4: this is the same
	// AIConfigurationProvider that ContractClient + BatchClient use, so
	// token counting never drifts from the actual model used by Gemini
	// generateContent calls.
	configProvider ports.AIConfigurationProvider
}

// SetConfigurationProvider wires the dynamic AIConfigurationProvider.
// Per P1-4: this MUST be called at bootstrap so the TokenCounter uses
// the same active config (apiKey + model + baseURL) as the ContractClient
// and BatchClient. Without this, the TokenCounter stays pinned to the
// static struct fields even after a model switch via the Platform Admin API.
func (c *TokenCounter) SetConfigurationProvider(provider ports.AIConfigurationProvider) {
	c.configProvider = provider
}

// resolvedConfig returns the effective config for this CountTokens call.
// Per P1-4: if configProvider is wired, reads from cache/DB. Otherwise
// falls back to static struct fields (env bootstrap, tests).
func (c *TokenCounter) resolveConfig(ctx context.Context) (*resolvedAIConfig, error) {
	if c.configProvider != nil {
		cfg, err := c.configProvider.GetActiveConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve AI config for token counting: %w", err)
		}
		return &resolvedAIConfig{
			apiKey:  cfg.APIKey,
			model:   cfg.Model,
			baseURL: cfg.BaseURL,
		}, nil
	}
	return &resolvedAIConfig{
		apiKey:  c.apiKey,
		model:   c.model,
		baseURL: c.baseURL,
	}, nil
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
	// Per P1-4: resolve the ACTIVE runtime config so the model used
	// for counting matches the model used by the actual generateContent
	// call. Without this, a model switch via the Platform Admin API
	// would not be picked up by the TokenCounter.
	rc, err := c.resolveConfig(ctx)
	if err != nil {
		return 0, err
	}
	reqBody := countTokensRequest{
		Contents: []countTokensContent{
			{Parts: []countTokensPart{{Text: text}}},
		},
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("marshal countTokens request: %w", err)
	}

	// P1-8: API key sent via x-goog-api-key header only — never in URL.
	url := fmt.Sprintf("%s/v1beta/models/%s:countTokens", rc.baseURL, rc.model)
	reqCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return 0, fmt.Errorf("build countTokens request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", rc.apiKey)

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

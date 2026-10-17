package litellm

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
	"github.com/google/uuid"
)

// ModelsClient uses the configured LiteLLM virtual key; it never talks
// directly to Gemini or exposes any upstream provider credentials.
type ModelsClient struct {
	timeout time.Duration
	client  *http.Client
}

func NewModelsClient() *ModelsClient {
	return &ModelsClient{
		timeout: 15 * time.Second,
		client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}
}

func (c *ModelsClient) transport(apiKey, baseURL string) (*Client, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("LiteLLM models client is not configured")
	}
	return NewClient(Config{APIKey: apiKey, BaseURL: baseURL, Timeout: c.timeout, HTTPClient: c.client})
}

// DiscoverModels reads ONLY aliases visible to Mujeeb's virtual key.
// LiteLLM's OpenAI-compatible /v1/models does not guarantee token limits,
// pricing or model capabilities, so we deliberately do not invent them.
func (c *ModelsClient) DiscoverModels(ctx context.Context, apiKey, baseURL string) ([]ports.AIProviderModel, error) {
	gateway, err := c.transport(apiKey, baseURL)
	if err != nil {
		return nil, err
	}
	root := strings.TrimSuffix(gateway.endpoint, "/v1/chat/completions")
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, root+"/v1/models", nil)
	if err != nil {
		return nil, errors.New("build LiteLLM models request failed")
	}
	req.Header.Set("Authorization", "Bearer "+gateway.apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := gateway.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("LiteLLM model discovery request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{StatusCode: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20+1))
	if err != nil {
		return nil, errors.New("read LiteLLM models response failed")
	}
	if len(body) > 2<<20 {
		return nil, errors.New("LiteLLM models response exceeds size limit")
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("invalid LiteLLM models response")
	}
	if payload.Data == nil {
		return nil, errors.New("LiteLLM models response does not contain data")
	}
	seen := make(map[string]bool, len(payload.Data))
	models := make([]ports.AIProviderModel, 0, len(payload.Data))
	for _, entry := range payload.Data {
		id := strings.TrimSpace(entry.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		label := id
		models = append(models, ports.AIProviderModel{
			ID: uuid.NewString(), Provider: "litellm", ModelName: id,
			DisplayName: &label, DiscoveredAt: time.Now().UTC(),
		})
	}
	return models, nil
}

// TestConnection probes the exact LiteLLM chat completions endpoint used by the
// customer/catalog runtime. No merchant data, upstream key or prompt is sent.
func (c *ModelsClient) TestConnection(ctx context.Context, apiKey, model, baseURL string) (bool, int64, string) {
	if strings.TrimSpace(model) == "" {
		return false, 0, "NO_MODEL"
	}
	gateway, err := c.transport(apiKey, baseURL)
	if err != nil {
		return false, 0, "LITELLM_CONFIG_INVALID"
	}
	requestBody, _ := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{{"role": "user", "content": "Hello"}},
		"max_tokens": 16, "stream": false,
	})
	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	start := time.Now()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, gateway.endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return false, time.Since(start).Milliseconds(), "REQUEST_BUILD_FAILED"
	}
	req.Header.Set("Authorization", "Bearer "+gateway.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := gateway.client.Do(req)
	if err != nil {
		return false, time.Since(start).Milliseconds(), "LITELLM_REQUEST_FAILED"
	}
	defer resp.Body.Close()
	latency := time.Since(start).Milliseconds()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, latency, fmt.Sprintf("LITELLM_HTTP_%d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes {
		return false, latency, "LITELLM_INVALID_RESPONSE"
	}
	var result struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message struct {
				Role string `json:"role"`
				Content *string `json:"content"`
				Refusal *string `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Choices) != 1 ||
		result.Choices[0].FinishReason != "stop" ||
		result.Choices[0].Message.Role != "assistant" ||
		result.Choices[0].Message.Refusal != nil ||
		result.Choices[0].Message.Content == nil ||
		strings.TrimSpace(*result.Choices[0].Message.Content) == "" {
		return false, latency, "LITELLM_INCOMPLETE_RESPONSE"
	}
	return true, latency, ""
}

var _ ports.ModelDiscoveryClient = (*ModelsClient)(nil)

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
)

// CountInteractionTokens estimates model-specific input size via LiteLLM's
// /utils/token_counter endpoint. It fails closed if the gateway cannot count.
// This is NOT an exact upstream-token guarantee for all model providers.
func (c *Client) CountInteractionTokens(ctx context.Context, model, system, input string, tools any) (int, error) {
	if c == nil || strings.TrimSpace(model) == "" {
		return 0, errors.New("LiteLLM token counter needs a client and model")
	}
	messages := []map[string]string{
		{"role": "system", "content": system},
		{"role": "user", "content": input},
	}
	request := map[string]any{"model": model, "messages": messages}
	if tools != nil {
		request["tools"] = tools
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return 0, fmt.Errorf("marshal LiteLLM token count: %w", err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	target := strings.TrimSuffix(c.endpoint, "/v1beta/interactions") + "/utils/token_counter"
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("build LiteLLM token count: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(req)
	if err != nil {
		if requestCtx.Err() != nil {
			return 0, requestCtx.Err()
		}
		return 0, fmt.Errorf("LiteLLM token count transport: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, &HTTPError{StatusCode: response.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return 0, fmt.Errorf("read LiteLLM token count: %w", err)
	}
	var result struct {
		TotalTokens int `json:"total_tokens"`
		Error bool `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, errors.New("invalid LiteLLM token count response")
	}
	if result.Error || result.TotalTokens <= 0 {
		return 0, errors.New("LiteLLM token count unavailable")
	}
	// The proxy may use an approximate tokenizer or miss tool overhead. Count
	// at least twice its estimate AND the UTF-8 request length, plus reserve.
	// The route must also be configured so every fallback fits within the
	// smallest model context window. Never silently truncate catalog data.
	count := result.TotalTokens * 2
	if len(payload)+512 > count {
		count = len(payload) + 512
	}
	return count, nil
}

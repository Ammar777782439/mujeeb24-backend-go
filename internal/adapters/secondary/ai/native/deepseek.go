package native

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const nativeResponseLimit = 4 << 20

// DeepSeekConfig never accepts business-policy configuration or tool executors.
type DeepSeekConfig struct {
	Model      string
	APIKey     string
	BaseURL    string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// DeepSeek sends generation-only calls to DeepSeek's API directly.
type DeepSeek struct {
	model    string
	key      string
	endpoint string
	client   *http.Client
	timeout  time.Duration
}

func NewDeepSeek(cfg DeepSeekConfig) (*DeepSeek, error) {
	if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("DeepSeek API key and model are required")
	}
	base := cfg.BaseURL
	if base == "" {
		base = "https://api.deepseek.com/beta"
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/beta" {
		return nil, errors.New("DeepSeek requires a canonical /beta endpoint")
	}
	if u.Scheme != "https" {
		host := u.Hostname()
		if u.Scheme != "http" || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return nil, errors.New("DeepSeek HTTP is only allowed for local test endpoints")
		}
	} else if !strings.EqualFold(u.Hostname(), "api.deepseek.com") {
		return nil, errors.New("DeepSeek hostname must be api.deepseek.com")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{
			Transport:     &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	return &DeepSeek{model: cfg.Model, key: cfg.APIKey, endpoint: strings.TrimRight(base, "/") + "/chat/completions", client: client, timeout: cfg.Timeout}, nil
}
func (c *DeepSeek) Name() string { return "deepseek" }

type canonicalDeepSeekRequest struct {
	Model             string            `json:"model"`
	Messages          []json.RawMessage `json:"messages"`
	Tools             []json.RawMessage `json:"tools,omitempty"`
	ToolChoice        string            `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool             `json:"parallel_tool_calls,omitempty"`
	ResponseFormat    *struct {
		Type       string `json:"type"`
		JSONSchema struct {
			Name   string          `json:"name"`
			Strict bool            `json:"strict"`
			Schema json.RawMessage `json:"schema"`
		} `json:"json_schema"`
	} `json:"response_format,omitempty"`
	MaxTokens int  `json:"max_tokens"`
	Stream    bool `json:"stream"`
}

type deepSeekChoice struct {
	FinishReason string `json:"finish_reason"`
	Message      struct {
		Role      string          `json:"role"`
		Content   json.RawMessage `json:"content"`
		ToolCalls []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	} `json:"message"`
}

func (c *DeepSeek) Generate(ctx context.Context, request Request) (Response, error) {
	if c == nil {
		return Response{}, errors.New("DeepSeek is not configured")
	}
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	var canonical canonicalDeepSeekRequest
	decoder := json.NewDecoder(bytes.NewReader(request.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&canonical); err != nil {
		return Response{}, fmt.Errorf("invalid canonical inference request: %w", err)
	}
	if len(canonical.Messages) == 0 || canonical.MaxTokens <= 0 || canonical.Stream || canonical.Model == "" {
		return Response{}, errors.New("DeepSeek requires non-streaming canonical messages and token budget")
	}
	if canonical.ToolChoice == "required" && len(canonical.Tools) == 0 {
		return Response{}, errors.New("required capability without tools")
	}
	if canonical.ParallelToolCalls != nil && *canonical.ParallelToolCalls {
		return Response{}, errors.New("DeepSeek native only accepts non-parallel customer capability calls")
	}
	body := map[string]any{
		"model": c.model, "messages": canonical.Messages, "max_tokens": canonical.MaxTokens,
		"stream": false, "thinking": map[string]string{"type": "disabled"},
	}
	const synthetic = "submit_catalog_batch"
	structured := canonical.ResponseFormat != nil
	if structured {
		if len(canonical.Tools) != 0 {
			return Response{}, errors.New("combined merchant tools and structured output require separate verified adapter")
		}
		format := canonical.ResponseFormat
		if format.Type != "json_schema" || !format.JSONSchema.Strict || !json.Valid(format.JSONSchema.Schema) {
			return Response{}, errors.New("DeepSeek refuses non-strict or malformed structured schema")
		}
		body["tools"] = []any{map[string]any{"type": "function", "function": map[string]any{
			"name": synthetic, "description": "Return the requested catalog batch JSON exactly as specified",
			"strict": true, "parameters": format.JSONSchema.Schema,
		}}}
		body["tool_choice"] = "required"
	} else if len(canonical.Tools) > 0 {
		if canonical.ToolChoice != "required" && canonical.ToolChoice != "auto" {
			return Response{}, errors.New("unsupported function selection mode")
		}
		for _, raw := range canonical.Tools {
			var tool struct {
				Type     string `json:"type"`
				Function struct {
					Name   string `json:"name"`
					Strict bool   `json:"strict"`
				} `json:"function"`
			}
			if err := json.Unmarshal(raw, &tool); err != nil || tool.Type != "function" || tool.Function.Name == "" || !tool.Function.Strict {
				return Response{}, errors.New("DeepSeek requires strictly declared tool schemas")
			}
		}
		body["tools"] = canonical.Tools
		body["tool_choice"] = canonical.ToolChoice
		if canonical.ParallelToolCalls != nil {
			body["parallel_tool_calls"] = false
		}
	} else if canonical.ToolChoice != "" {
		return Response{}, errors.New("tool choice is set without tools")
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(timeoutCtx, http.MethodPost, c.endpoint, bytes.NewReader(encoded))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.key)
	httpReq.Header.Set("Content-Type", "application/json")
	httpRes, err := c.client.Do(httpReq)
	if err != nil {
		if timeoutCtx.Err() != nil {
			return Response{}, timeoutCtx.Err()
		}
		return Response{}, errors.New("DeepSeek transport failure")
	}
	defer httpRes.Body.Close()
	if httpRes.StatusCode < 200 || httpRes.StatusCode >= 300 {
		return Response{}, &StatusError{Provider: "deepseek", Code: httpRes.StatusCode}
	}
	raw, err := io.ReadAll(io.LimitReader(httpRes.Body, nativeResponseLimit+1))
	if err != nil || len(raw) > nativeResponseLimit {
		return Response{}, errors.New("DeepSeek response read/size failure")
	}
	var envelope struct {
		Model   string           `json:"model"`
		Choices []deepSeekChoice `json:"choices"`
		Usage   struct {
			PromptTokens         int64 `json:"prompt_tokens"`
			CompletionTokens     int64 `json:"completion_tokens"`
			PromptCacheHitTokens int64 `json:"prompt_cache_hit_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || len(envelope.Choices) != 1 {
		return Response{}, errors.New("invalid DeepSeek choice envelope")
	}
	choice := envelope.Choices[0]
	if choice.Message.Role != "assistant" {
		return Response{}, errors.New("DeepSeek unexpected response role")
	}
	// A forced customer decision is always one capability. Do not trust a
	// provider to honor parallel_tool_calls or its strict function contract.
	if !structured && canonical.ToolChoice == "required" &&
		(choice.FinishReason != "tool_calls" || len(choice.Message.ToolCalls) != 1) {
		return Response{}, errors.New("DeepSeek must return exactly one required capability")
	}
	if choice.FinishReason == "tool_calls" {
		if len(choice.Message.ToolCalls) == 0 || (len(choice.Message.Content) != 0 &&
			string(choice.Message.Content) != "null" && string(choice.Message.Content) != "\"\"") {
			return Response{}, errors.New("DeepSeek mixed text with tool calls")
		}
		allowed := make(map[string]struct{}, len(canonical.Tools))
		if structured {
			allowed[synthetic] = struct{}{}
		}
		for _, rawTool := range canonical.Tools {
			var tool struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			}
			if err := json.Unmarshal(rawTool, &tool); err != nil {
				return Response{}, errors.New("malformed declared function")
			}
			allowed[tool.Function.Name] = struct{}{}
		}
		ids := make(map[string]struct{}, len(choice.Message.ToolCalls))
		for _, call := range choice.Message.ToolCalls {
			if call.ID == "" || call.Type != "function" || call.Function.Name == "" ||
				!json.Valid([]byte(call.Function.Arguments)) {
				return Response{}, errors.New("DeepSeek returned invalid function call")
			}
			if _, ok := allowed[call.Function.Name]; !ok {
				return Response{}, errors.New("DeepSeek returned function not offered by Mujeeb")
			}
			if _, duplicate := ids[call.ID]; duplicate {
				return Response{}, errors.New("duplicate DeepSeek call ID")
			}
			ids[call.ID] = struct{}{}
			var args map[string]json.RawMessage
			if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil || args == nil {
				return Response{}, errors.New("DeepSeek function arguments must be an object")
			}
		}
	}
	if structured {
		if choice.FinishReason != "tool_calls" || len(choice.Message.ToolCalls) != 1 ||
			choice.Message.ToolCalls[0].Function.Name != synthetic ||
			choice.Message.ToolCalls[0].Type != "function" ||
			!json.Valid([]byte(choice.Message.ToolCalls[0].Function.Arguments)) {
			return Response{}, errors.New("DeepSeek did not provide one structured catalog tool call")
		}
		encoded, err = json.Marshal(map[string]any{
			"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{
				"role": "assistant", "content": choice.Message.ToolCalls[0].Function.Arguments,
			}}},
			"usage": map[string]any{"prompt_tokens": envelope.Usage.PromptTokens,
				"completion_tokens":     envelope.Usage.CompletionTokens,
				"prompt_tokens_details": map[string]any{"cached_tokens": envelope.Usage.PromptCacheHitTokens}},
		})
		if err != nil {
			return Response{}, err
		}
		raw = encoded
	} else if (choice.FinishReason != "stop" && choice.FinishReason != "tool_calls") ||
		(choice.FinishReason == "tool_calls" && len(choice.Message.ToolCalls) == 0) {
		return Response{}, errors.New("DeepSeek completion incomplete or inconsistent")
	}
	if envelope.Usage.PromptTokens < 0 || envelope.Usage.CompletionTokens < 0 ||
		envelope.Usage.PromptCacheHitTokens < 0 || envelope.Usage.PromptCacheHitTokens > envelope.Usage.PromptTokens {
		return Response{}, errors.New("DeepSeek usage counters invalid")
	}
	return Response{Payload: raw, Model: c.model, InputTokens: envelope.Usage.PromptTokens,
		OutputTokens: envelope.Usage.CompletionTokens, CachedTokens: envelope.Usage.PromptCacheHitTokens}, nil
}

package native

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var geminiModelName = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

type GeminiConfig struct {
	Model      string
	APIKey     string
	BaseURL    string
	Timeout    time.Duration
	HTTPClient *http.Client
}

// Gemini is a direct Google AI Studio REST adapter. The caller still owns the
// canonical schema and all business authorization/execution.
type Gemini struct {
	model    string
	key      string
	endpoint string
	client   *http.Client
	timeout  time.Duration
}

func NewGemini(cfg GeminiConfig) (*Gemini, error) {
	if strings.TrimSpace(cfg.APIKey) == "" || !geminiModelName.MatchString(cfg.Model) {
		return nil, errors.New("Gemini requires API key and safe model identifier")
	}
	base := cfg.BaseURL
	if base == "" {
		base = "https://generativelanguage.googleapis.com/v1beta"
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/v1beta" {
		return nil, errors.New("Gemini requires a /v1beta endpoint")
	}
	if u.Scheme == "https" {
		if u.Hostname() != "generativelanguage.googleapis.com" {
			return nil, errors.New("Gemini hostname not allowed")
		}
	} else if u.Scheme != "http" || net.ParseIP(u.Hostname()) == nil || !net.ParseIP(u.Hostname()).IsLoopback() {
		return nil, errors.New("Gemini plain HTTP restricted to local test endpoints")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &Gemini{model: cfg.Model, key: cfg.APIKey, endpoint: strings.TrimRight(base, "/") + "/models/" + cfg.Model + ":generateContent", client: client, timeout: cfg.Timeout}, nil
}
func (c *Gemini) Name() string { return "gemini" }

type geminiPart struct {
	Text         string `json:"text,omitempty"`
	FunctionCall *struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
		ID   string          `json:"id,omitempty"`
	} `json:"functionCall,omitempty"`
	FunctionResponse *struct {
		Name     string         `json:"name"`
		Response map[string]any `json:"response"`
		ID       string         `json:"id,omitempty"`
	} `json:"functionResponse,omitempty"`
	ThoughtSignature string `json:"thoughtSignature,omitempty"`
}
type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}
type geminiWireRequest struct {
	SystemInstruction *struct {
		Parts []geminiPart `json:"parts"`
	} `json:"systemInstruction,omitempty"`
	Contents         []geminiContent `json:"contents"`
	Tools            []any           `json:"tools,omitempty"`
	ToolConfig       any             `json:"toolConfig,omitempty"`
	GenerationConfig map[string]any  `json:"generationConfig"`
}

func (c *Gemini) Generate(ctx context.Context, req Request) (Response, error) {
	if c == nil {
		return Response{}, errors.New("Gemini is not configured")
	}
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	var canonical canonicalDeepSeekRequest
	if err := strictUnmarshal(req.Payload, &canonical); err != nil {
		return Response{}, fmt.Errorf("invalid canonical request: %w", err)
	}
	if canonical.Model == "" || canonical.MaxTokens <= 0 || canonical.Stream || len(canonical.Messages) == 0 {
		return Response{}, errors.New("Gemini requires valid stateless messages and output budget")
	}
	out := geminiWireRequest{GenerationConfig: map[string]any{"maxOutputTokens": canonical.MaxTokens}}
	callName := make(map[string]string)
	toolSchemas := make(map[string]json.RawMessage)
	for i, raw := range canonical.Messages {
		var message struct {
			Role      string          `json:"role"`
			Content   json.RawMessage `json:"content"`
			ToolCalls []struct {
				ID               string `json:"id"`
				Type             string `json:"type"`
				ThoughtSignature string `json:"thought_signature,omitempty"`
				Function         struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
			ToolCallID string `json:"tool_call_id"`
		}
		if err := json.Unmarshal(raw, &message); err != nil {
			return Response{}, errors.New("invalid canonical message")
		}
		switch message.Role {
		case "system":
			if i != 0 || out.SystemInstruction != nil {
				return Response{}, errors.New("system instruction must be first and unique")
			}
			var text string
			if err := json.Unmarshal(message.Content, &text); err != nil || text == "" {
				return Response{}, errors.New("invalid system instruction")
			}
			out.SystemInstruction = &struct {
				Parts []geminiPart `json:"parts"`
			}{Parts: []geminiPart{{Text: text}}}
		case "user":
			var text string
			if err := json.Unmarshal(message.Content, &text); err != nil {
				return Response{}, errors.New("Gemini text request requires user text")
			}
			out.Contents = append(out.Contents, geminiContent{Role: "user", Parts: []geminiPart{{Text: text}}})
		case "assistant":
			if len(message.ToolCalls) == 0 {
				var text string
				if err := json.Unmarshal(message.Content, &text); err != nil {
					return Response{}, errors.New("invalid assistant continuation")
				}
				out.Contents = append(out.Contents, geminiContent{Role: "model", Parts: []geminiPart{{Text: text}}})
				continue
			}
			parts := make([]geminiPart, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				if call.Type != "function" || call.Function.Name == "" || call.ID == "" || !json.Valid([]byte(call.Function.Arguments)) {
					return Response{}, errors.New("invalid assistant function call replay")
				}
				var params map[string]any
				if err := json.Unmarshal([]byte(call.Function.Arguments), &params); err != nil || params == nil {
					return Response{}, errors.New("replayed function arguments must be object")
				}
				payload, _ := json.Marshal(params)
				part := geminiPart{FunctionCall: &struct {
					Name string          `json:"name"`
					Args json.RawMessage `json:"args"`
					ID   string          `json:"id,omitempty"`
				}{Name: call.Function.Name, Args: payload, ID: call.ID}, ThoughtSignature: call.ThoughtSignature}
				parts = append(parts, part)
				if _, exists := callName[call.ID]; exists {
					return Response{}, errors.New("duplicate replayed tool identifier")
				}
				callName[call.ID] = call.Function.Name
			}
			out.Contents = append(out.Contents, geminiContent{Role: "model", Parts: parts})
		case "tool":
			name := callName[message.ToolCallID]
			if name == "" {
				return Response{}, errors.New("tool result does not match prior tool call")
			}
			var text string
			if err := json.Unmarshal(message.Content, &text); err != nil {
				return Response{}, errors.New("invalid tool response text")
			}
			var value any
			if err := json.Unmarshal([]byte(text), &value); err != nil {
				return Response{}, errors.New("invalid JSON tool result")
			}
			part := geminiPart{FunctionResponse: &struct {
				Name     string         `json:"name"`
				Response map[string]any `json:"response"`
				ID       string         `json:"id,omitempty"`
			}{Name: name, ID: message.ToolCallID, Response: map[string]any{"result": value}}}
			out.Contents = append(out.Contents, geminiContent{Role: "user", Parts: []geminiPart{part}})
		default:
			return Response{}, errors.New("unsupported Gemini message role")
		}
	}
	if len(out.Contents) == 0 {
		return Response{}, errors.New("Gemini request missing user content")
	}
	if canonical.ResponseFormat != nil {
		if len(canonical.Tools) != 0 {
			return Response{}, errors.New("combined tools and structured output require a verified merchant adapter")
		}
		format := canonical.ResponseFormat
		if format.Type != "json_schema" || !format.JSONSchema.Strict || !json.Valid(format.JSONSchema.Schema) {
			return Response{}, errors.New("Gemini refuses non-strict structured schema")
		}
		if _, err := compileNativeSchema(format.JSONSchema.Schema); err != nil {
			return Response{}, fmt.Errorf("unsupported Gemini structured schema: %w", err)
		}
		out.GenerationConfig["responseMimeType"] = "application/json"
		out.GenerationConfig["responseJsonSchema"] = format.JSONSchema.Schema
	}
	if len(canonical.Tools) > 0 {
		if canonical.ToolChoice != "required" && canonical.ToolChoice != "auto" {
			return Response{}, errors.New("invalid Gemini tool choice")
		}
		if canonical.ParallelToolCalls != nil && *canonical.ParallelToolCalls {
			return Response{}, errors.New("parallel customer capabilities not supported")
		}
		declarations := make([]any, 0, len(canonical.Tools))
		for _, raw := range canonical.Tools {
			var tool struct {
				Type     string `json:"type"`
				Function struct {
					Name        string          `json:"name"`
					Description string          `json:"description"`
					Strict      bool            `json:"strict"`
					Parameters  json.RawMessage `json:"parameters"`
				} `json:"function"`
			}
			if err := json.Unmarshal(raw, &tool); err != nil || tool.Type != "function" ||
				tool.Function.Name == "" || !tool.Function.Strict || !json.Valid(tool.Function.Parameters) {
				return Response{}, errors.New("invalid strict Gemini function declaration")
			}
			if _, exists := toolSchemas[tool.Function.Name]; exists {
				return Response{}, errors.New("duplicate Gemini function schema")
			}
			if _, err := compileNativeSchema(tool.Function.Parameters); err != nil {
				return Response{}, fmt.Errorf("invalid Gemini function schema: %w", err)
			}
			toolSchemas[tool.Function.Name] = tool.Function.Parameters
			declarations = append(declarations, map[string]any{
				"name": tool.Function.Name, "description": tool.Function.Description,
				"parametersJsonSchema": tool.Function.Parameters,
			})
		}
		out.Tools = []any{map[string]any{"functionDeclarations": declarations}}
		mode := "AUTO"
		if canonical.ToolChoice == "required" {
			mode = "ANY"
		}
		out.ToolConfig = map[string]any{"functionCallingConfig": map[string]any{"mode": mode}}
	} else if canonical.ToolChoice != "" {
		return Response{}, errors.New("Gemini tool choice without functions")
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return Response{}, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.endpoint, bytes.NewReader(encoded))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("x-goog-api-key", c.key)
	httpReq.Header.Set("Content-Type", "application/json")
	httpRes, err := c.client.Do(httpReq)
	if err != nil {
		if requestCtx.Err() != nil {
			return Response{}, requestCtx.Err()
		}
		return Response{}, errors.New("Gemini transport failure")
	}
	defer httpRes.Body.Close()
	if httpRes.StatusCode < 200 || httpRes.StatusCode >= 300 {
		return Response{}, &StatusError{Provider: "gemini", Code: httpRes.StatusCode}
	}
	raw, err := io.ReadAll(io.LimitReader(httpRes.Body, nativeResponseLimit+1))
	if err != nil || len(raw) > nativeResponseLimit {
		return Response{}, errors.New("Gemini response read/size failure")
	}
	var resp struct {
		Candidates []struct {
			FinishReason string `json:"finishReason"`
			Content      struct {
				Parts []geminiPart `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount        int64 `json:"promptTokenCount"`
			CandidatesTokenCount    int64 `json:"candidatesTokenCount"`
			CachedContentTokenCount int64 `json:"cachedContentTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || len(resp.Candidates) != 1 || resp.Candidates[0].FinishReason != "STOP" {
		return Response{}, errors.New("Gemini returned malformed, blocked, or incomplete inference")
	}
	if resp.UsageMetadata.PromptTokenCount < 0 || resp.UsageMetadata.CandidatesTokenCount < 0 ||
		resp.UsageMetadata.CachedContentTokenCount < 0 || resp.UsageMetadata.CachedContentTokenCount > resp.UsageMetadata.PromptTokenCount {
		return Response{}, errors.New("Gemini usage invalid")
	}
	var content string
	calls := make([]any, 0)
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.FunctionCall != nil {
			if strings.TrimSpace(part.FunctionCall.Name) == "" || !json.Valid(part.FunctionCall.Args) {
				return Response{}, errors.New("Gemini returned invalid function arguments")
			}
			schema, offered := toolSchemas[part.FunctionCall.Name]
			if !offered {
				return Response{}, errors.New("Gemini returned function that was not offered")
			}
			if err := validateNativeSchemaResult(part.FunctionCall.Args, schema); err != nil {
				return Response{}, fmt.Errorf("Gemini function arguments violate schema: %w", err)
			}
			var args map[string]any
			if err := json.Unmarshal(part.FunctionCall.Args, &args); err != nil || args == nil {
				return Response{}, errors.New("Gemini function args must be object")
			}
			argString := string(part.FunctionCall.Args)
			id := part.FunctionCall.ID
			if id == "" {
				var randBytes [16]byte
				if _, err := rand.Read(randBytes[:]); err != nil {
					return Response{}, errors.New("cannot allocate call identifier")
				}
				id = "call_" + hex.EncodeToString(randBytes[:])
			}
			// Preserve Google's opaque signature as provider transport metadata.
			// The merchant adapter carries it through its read-only tool rounds;
			// the next Gemini Generate reconstructs the signed functionCall part
			// exactly where Google supplied the signature. Do not synthesize one.
			call := map[string]any{"id": id, "type": "function", "function": map[string]any{
				"name": part.FunctionCall.Name, "arguments": argString,
			}}
			if part.ThoughtSignature != "" {
				call["thought_signature"] = part.ThoughtSignature
			}
			calls = append(calls, call)
		} else if part.FunctionResponse != nil {
			return Response{}, errors.New("Gemini returned unsolicited function response")
		} else {
			content += part.Text
		}
	}
	if len(calls) > 0 && strings.TrimSpace(content) != "" {
		return Response{}, errors.New("Gemini mixed tool call and text")
	}
	if canonical.ToolChoice == "required" && len(calls) != 1 {
		return Response{}, errors.New("Gemini must return exactly one customer capability")
	}
	if len(calls) == 0 && strings.TrimSpace(content) == "" {
		return Response{}, errors.New("Gemini returned no output")
	}
	if canonical.ResponseFormat != nil {
		if len(calls) != 0 {
			return Response{}, errors.New("Gemini structured result unexpectedly called a function")
		}
		if err := validateNativeSchemaResult(json.RawMessage(content), canonical.ResponseFormat.JSONSchema.Schema); err != nil {
			return Response{}, fmt.Errorf("Gemini structured output violates schema: %w", err)
		}
	}
	reason := "stop"
	var contentOut any = content
	if len(calls) > 0 {
		reason = "tool_calls"
		contentOut = nil
	}
	normalized, err := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"finish_reason": reason, "message": map[string]any{
			"role": "assistant", "content": contentOut, "tool_calls": calls,
		}}},
		"usage": map[string]any{"prompt_tokens": resp.UsageMetadata.PromptTokenCount,
			"completion_tokens":     resp.UsageMetadata.CandidatesTokenCount,
			"prompt_tokens_details": map[string]any{"cached_tokens": resp.UsageMetadata.CachedContentTokenCount}},
	})
	if err != nil {
		return Response{}, err
	}
	return Response{Payload: normalized, Model: c.model, InputTokens: resp.UsageMetadata.PromptTokenCount,
		OutputTokens: resp.UsageMetadata.CandidatesTokenCount, CachedTokens: resp.UsageMetadata.CachedContentTokenCount}, nil
}

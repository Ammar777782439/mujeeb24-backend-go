package merchantgemini

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

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

const (
	defaultMaxOutputTokens  = 2048
	defaultMaxResponseBytes = 1 << 20
	defaultSafetyTurnBudget = 10
	defaultBaseURL          = "https://generativelanguage.googleapis.com"
	defaultModel            = "gemini-2.5-flash"
)

// Config contains configuration for the Merchant AI Gemini client.
type Config struct {
	BaseURL            string
	APIKey             string
	Model              string
	HTTPClient         *http.Client
	RequestTimeout     time.Duration
	MaxOutputTokens    int
	MaxResponseBytes   int64
	MaxInputCharacters int
	SafetyTurnBudget   int
	SystemPrompt       string
	Capabilities       ports.AICapabilityDispatcher
}

// Client is a dedicated Gemini client for the interactive Merchant Copilot.
type Client struct {
	baseURL            string
	apiKey             string
	model              string
	httpClient         *http.Client
	requestTimeout     time.Duration
	maxOutputTokens    int
	maxResponseBytes   int64
	maxInputCharacters int
	safetyTurnBudget   int
	systemPrompt       string
	capabilities       ports.AICapabilityDispatcher
}

func NewClient(cfg Config) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("merchant gemini base URL must be an absolute HTTP or HTTPS URL")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("merchant gemini API key is required")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}
	requestTimeout := cfg.RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = 30 * time.Second
	}
	maxOutputTokens := cfg.MaxOutputTokens
	if maxOutputTokens <= 0 {
		maxOutputTokens = defaultMaxOutputTokens
	}
	maxResponseBytes := cfg.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = defaultMaxResponseBytes
	}
	maxInputCharacters := cfg.MaxInputCharacters
	if maxInputCharacters <= 0 {
		maxInputCharacters = 15000
	}
	safetyTurnBudget := cfg.SafetyTurnBudget
	if safetyTurnBudget <= 0 {
		safetyTurnBudget = defaultSafetyTurnBudget
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	systemPrompt := strings.TrimSpace(cfg.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = services.MerchantAISystemPrompt
	}
	return &Client{
		baseURL:            baseURL,
		apiKey:             strings.TrimSpace(cfg.APIKey),
		model:              model,
		httpClient:         client,
		requestTimeout:     requestTimeout,
		maxOutputTokens:    maxOutputTokens,
		maxResponseBytes:   maxResponseBytes,
		maxInputCharacters: maxInputCharacters,
		safetyTurnBudget:   safetyTurnBudget,
		systemPrompt:       systemPrompt,
		capabilities:       cfg.Capabilities,
	}, nil
}

func (c *Client) Chat(ctx context.Context, input ports.MerchantAIChatInput) (ports.MerchantAIChatOutput, error) {
	if c == nil {
		return ports.MerchantAIChatOutput{}, errors.New("merchant gemini client is not configured")
	}
	if err := ctx.Err(); err != nil {
		return ports.MerchantAIChatOutput{}, err
	}
	text := strings.TrimSpace(input.CurrentMessage)
	if text == "" {
		return ports.MerchantAIChatOutput{}, errors.New("merchant chat message cannot be empty")
	}
	if len([]rune(text)) > c.maxInputCharacters {
		return ports.MerchantAIChatOutput{}, fmt.Errorf("merchant chat message exceeds %d characters", c.maxInputCharacters)
	}

	contents := make([]geminiContent, 0, len(input.History)+1)
	for _, msg := range input.History {
		role := "user"
		if msg.SenderType == "assistant" {
			role = "model"
		}
		trimmed := strings.TrimSpace(msg.Text)
		if trimmed != "" {
			contents = append(contents, geminiContent{
				Role:  role,
				Parts: []geminiPart{{Text: trimmed}},
			})
		}
	}
	// If history is empty or last message is not this user's current message, append it
	if len(contents) == 0 || (contents[len(contents)-1].Role != "user" || contents[len(contents)-1].Parts[0].Text != text) {
		contents = append(contents, geminiContent{
			Role:  "user",
			Parts: []geminiPart{{Text: text}},
		})
	}

	requestCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	var tools []geminiTool
	if c.capabilities != nil {
		defs := c.capabilities.Definitions()
		if len(defs) > 0 {
			var funcDecls []geminiFunctionDeclaration
			for _, def := range defs {
				funcDecls = append(funcDecls, geminiFunctionDeclaration{
					Name:        def.Name,
					Description: def.Description,
					Parameters:  def.Parameters,
				})
			}
			tools = []geminiTool{
				{FunctionDeclarations: funcDecls},
			}
		}
	}

	execCtx := ports.AICapabilityExecutionContext{
		BusinessID:     input.BusinessID,
		ConversationID: input.SessionID,
	}

	var (
		toolCallsExecuted []string
		finalResponseText string
	)

	safetyBudget := c.safetyTurnBudget
	for turn := 0; turn < safetyBudget; turn++ {
		reqBody := geminiRequest{
			SystemInstruction: &geminiContent{
				Parts: []geminiPart{{Text: c.systemPrompt}},
			},
			Contents: contents,
			Tools:    tools,
			GenerationConfig: geminiGenerationConfig{
				MaxOutputTokens: c.maxOutputTokens,
			},
		}

		gemResp, err := c.sendRequest(requestCtx, reqBody)
		if err != nil {
			return ports.MerchantAIChatOutput{}, err
		}
		if len(gemResp.Candidates) == 0 || len(gemResp.Candidates[0].Content.Parts) == 0 {
			return ports.MerchantAIChatOutput{}, errors.New("merchant gemini response did not contain a candidate")
		}

		candidate := gemResp.Candidates[0]
		var functionCallPart *geminiFunctionCall
		var textContent string

		for _, part := range candidate.Content.Parts {
			if part.FunctionCall != nil && strings.TrimSpace(part.FunctionCall.Name) != "" {
				functionCallPart = part.FunctionCall
				break
			}
			if strings.TrimSpace(part.Text) != "" {
				textContent = strings.TrimSpace(part.Text)
			}
		}

		// If Gemini requests a tool execution:
		if functionCallPart != nil {
			toolName := strings.TrimSpace(functionCallPart.Name)
			toolCallsExecuted = append(toolCallsExecuted, toolName)

			var toolResponse map[string]any
			if c.capabilities != nil {
				capResult, capErr := c.capabilities.Execute(requestCtx, execCtx, toolName, functionCallPart.Args)
				if capErr != nil {
					toolResponse = map[string]any{
						"error": capErr.Error(),
					}
				} else {
					if dataMap, ok := capResult.Data.(map[string]any); ok {
						toolResponse = dataMap
					} else {
						encoded, _ := json.Marshal(capResult.Data)
						var m map[string]any
						if err := json.Unmarshal(encoded, &m); err == nil {
							toolResponse = m
						} else {
							toolResponse = map[string]any{"result": capResult.Data}
						}
					}
				}
			} else {
				toolResponse = map[string]any{"error": fmt.Sprintf("capability %q not available", toolName)}
			}

			// Append model's function call turn
			contents = append(contents, geminiContent{
				Role:  "model",
				Parts: candidate.Content.Parts,
			})
			// Append function result using role "user" (Gemini REST API specification)
			contents = append(contents, geminiContent{
				Role: "user",
				Parts: []geminiPart{
					{
						FunctionResponse: &geminiFunctionResponse{
							Name:     toolName,
							Response: toolResponse,
						},
					},
				},
			})
			continue
		}

		if textContent != "" {
			finalResponseText = textContent
			break
		}
	}

	if finalResponseText == "" {
		if len(toolCallsExecuted) > 0 {
			finalResponseText = "تم تنفيذ العمليات المطلوبة بنجاح."
		} else {
			finalResponseText = "يرجى توضيح استفسارك أو طلبك للمساعدة بشكل أفضل."
		}
	}

	action := "answer"
	if strings.Contains(finalResponseText, "؟") || strings.Contains(finalResponseText, "?") {
		action = "ask_clarification"
	} else if len(toolCallsExecuted) > 0 {
		action = "catalog_updated"
	}

	return ports.MerchantAIChatOutput{
		ResponseText: finalResponseText,
		Action:       action,
		ToolCalls:    toolCallsExecuted,
	}, nil
}

func (c *Client) sendRequest(requestCtx context.Context, reqBody geminiRequest) (geminiResponse, error) {
	encoded, err := json.Marshal(reqBody)
	if err != nil {
		return geminiResponse{}, fmt.Errorf("encode Merchant Gemini request: %w", err)
	}

	isOAuthToken := strings.HasPrefix(c.apiKey, "ya29.")
	var u string
	if isOAuthToken {
		u = fmt.Sprintf("%s/v1beta/models/%s:generateContent", c.baseURL, url.PathEscape(c.model))
	} else {
		u = fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.baseURL, url.PathEscape(c.model), url.QueryEscape(c.apiKey))
	}

	var resp *http.Response
	var body []byte
	var lastErr error
	maxRetries := 3
	baseDelay := 500 * time.Millisecond

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := baseDelay * time.Duration(1<<(attempt-1))
			if lastErr != nil && strings.Contains(lastErr.Error(), "HTTP 429") {
				delay = 15 * time.Second
			}
			select {
			case <-requestCtx.Done():
				return geminiResponse{}, requestCtx.Err()
			case <-time.After(delay):
			}
		}

		req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, u, bytes.NewReader(encoded))
		if err != nil {
			return geminiResponse{}, fmt.Errorf("create Merchant Gemini request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if isOAuthToken {
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
		} else {
			req.Header.Set("x-goog-api-key", c.apiKey)
		}

		resp, err = c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("network error: %w", err)
			continue
		}

		limited := io.LimitReader(resp.Body, c.maxResponseBytes+1)
		body, err = io.ReadAll(limited)
		resp.Body.Close()

		if err != nil {
			lastErr = fmt.Errorf("read response: %w", err)
			continue
		}
		if int64(len(body)) > c.maxResponseBytes {
			return geminiResponse{}, errors.New("Merchant Gemini response exceeds configured size limit")
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			snippet := strings.TrimSpace(string(body))
			if len(snippet) > 500 {
				snippet = snippet[:500]
			}
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
			continue
		}

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			snippet := strings.TrimSpace(string(body))
			if len(snippet) > 500 {
				snippet = snippet[:500]
			}
			return geminiResponse{}, fmt.Errorf("Merchant Gemini request returned HTTP %d: %s", resp.StatusCode, snippet)
		}

		lastErr = nil
		break
	}

	if lastErr != nil {
		if requestCtx.Err() != nil {
			return geminiResponse{}, requestCtx.Err()
		}
		return geminiResponse{}, fmt.Errorf("Merchant Gemini request failed after %d retries: %w", maxRetries, lastErr)
	}

	var gemResp geminiResponse
	if err := json.Unmarshal(body, &gemResp); err != nil {
		return geminiResponse{}, fmt.Errorf("decode Merchant Gemini response: %w", err)
	}
	return gemResp, nil
}

type geminiRequest struct {
	SystemInstruction *geminiContent         `json:"systemInstruction,omitempty"`
	Contents          []geminiContent        `json:"contents"`
	Tools             []geminiTool           `json:"tools,omitempty"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

type geminiFunctionDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type geminiFunctionResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
			Role  string       `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
}

var _ ports.MerchantAIRuntime = (*Client)(nil)

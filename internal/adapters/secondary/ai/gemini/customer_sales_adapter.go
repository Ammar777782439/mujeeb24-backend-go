// Package gemini implements bounded B2C capability selection.
// Gemini understands the customer and selects one offered function.
// The adapter returns a proposal for local validation, never executes tools.
// Catalog evaluation is requested explicitly and runs in the application layer.
// Canonical Mujeeb conversation history is sent on every stateless decision.

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
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/prompts"
)

// GeminiCustomerSalesAdapter selects an offered B2C capability through Gemini.
type GeminiCustomerSalesAdapter struct {
	base           *GeminiHTTPClient
	httpClient     *http.Client
	baseURL        string
	apiKey         string
	model          string
	requestTimeout time.Duration
	retryPolicy    geminiRetryPolicy
	// configProvider, when set, is called at the start of every Decide
	// call to get the ACTIVE runtime configuration (API key, model, limits).
	// Per §2: the runtime gets the active config from Configuration abstraction,
	// not from static env vars. If nil, falls back to static fields (bootstrap/tests).
	configProvider ports.AIConfigurationProvider
}

// resolvedAIConfig holds the effective values for one Gemini call.
type resolvedAIConfig struct {
	configVersion      int
	apiKey             string
	model              string
	baseURL            string
	maxOutputTokens    int
	maxInputCharacters int
}

// SetConfigurationProvider wires the dynamic AIConfigurationProvider.
// After this call, every Decide resolves the active config from
// the provider (cache/DB) instead of static struct fields. Per §9:
// cache invalidation makes new config active without restart.
func (c *GeminiCustomerSalesAdapter) SetConfigurationProvider(provider ports.AIConfigurationProvider) {
	c.configProvider = provider
}

// resolveConfig returns the effective AI configuration for this call.
// Per §1-2: if configProvider is wired, reads from cache/DB. Otherwise
// falls back to static fields (env bootstrap, tests).
func (c *GeminiCustomerSalesAdapter) resolveConfig(ctx context.Context) (*resolvedAIConfig, error) {
	if c.configProvider != nil {
		cfg, err := c.configProvider.GetActiveConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("resolve AI config: %w", err)
		}
		return &resolvedAIConfig{
			apiKey:             cfg.APIKey,
			model:              cfg.Model,
			configVersion:      cfg.ConfigurationVersion,
			baseURL:            cfg.BaseURL,
			maxOutputTokens:    cfg.MaxOutputTokens,
			maxInputCharacters: cfg.MaxInputCharacters,
		}, nil
	}
	return &resolvedAIConfig{
		apiKey:             c.apiKey,
		model:              c.model,
		baseURL:            c.baseURL,
		maxOutputTokens:    c.base.maxOutputTokens,
		maxInputCharacters: c.base.maxInputCharacters,
	}, nil
}

// NewGeminiCustomerSalesAdapter wraps an existing Client with contract-aligned methods.
//
// Per contract ⑤ §7, the Catalog Entity Contract is built once at startup
// and reused for every call; callers pass it via CustomerSalesDecisionInput.
func NewGeminiCustomerSalesAdapter(base *GeminiHTTPClient) (*GeminiCustomerSalesAdapter, error) {
	if base == nil {
		return nil, errors.New("base client is required")
	}
	return &GeminiCustomerSalesAdapter{
		base:           base,
		httpClient:     base.httpClient,
		baseURL:        base.baseURL,
		apiKey:         base.apiKey,
		model:          base.model,
		requestTimeout: base.requestTimeout,
		retryPolicy:    base.retryPolicy,
	}, nil
}

// Decide performs one bounded capability selection. It never executes side effects.
func (c *GeminiCustomerSalesAdapter) Decide(ctx context.Context, input ports.CustomerSalesDecisionInput) (ports.CustomerSalesDecisionOutput, error) {
	if err := ctx.Err(); err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}
	if strings.TrimSpace(input.Request.Text) == "" {
		return ports.CustomerSalesDecisionOutput{}, errors.New("AI input text is required")
	}

	rc, err := c.resolveConfig(ctx)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}
	if rc.maxInputCharacters > 0 && len([]rune(input.Request.Text)) > rc.maxInputCharacters {
		return ports.CustomerSalesDecisionOutput{}, fmt.Errorf("AI input text exceeds %d characters", rc.maxInputCharacters)
	}

	startedAt := time.Now().UTC()
	return c.decideInteraction(ctx, input, rc, startedAt)
}

func (c *GeminiCustomerSalesAdapter) decideInteraction(
	ctx context.Context,
	input ports.CustomerSalesDecisionInput,
	rc *resolvedAIConfig,
	startedAt time.Time,
) (ports.CustomerSalesDecisionOutput, error) {

	interactionInput := buildUserPrompt(input.Request)
	systemInstruction := c.buildContractSystemInstructionText(input.EntityContractPayload)
	capabilities := input.Capabilities
	if capabilities == nil {
		capabilities = ports.CustomerSalesCapabilities(false)
	}
	tools := customerSalesTools(capabilities)
	if len(tools) == 0 {
		return ports.CustomerSalesDecisionOutput{}, errors.New("no customer sales capabilities available")
	}
	if rc.maxInputCharacters > 0 {
		encodedTools, encodeErr := json.Marshal(tools)
		if encodeErr != nil {
			return ports.CustomerSalesDecisionOutput{}, encodeErr
		}
		totalChars := len([]rune(interactionInput)) + len([]rune(systemInstruction)) + len([]rune(string(encodedTools)))
		if totalChars > rc.maxInputCharacters {
			return ports.CustomerSalesDecisionOutput{}, fmt.Errorf(
				"customer sales interaction input exceeds %d characters after context and system instruction serialization: %d",
				rc.maxInputCharacters,
				totalChars,
			)
		}
	}

	reqBody := interactionRequest{
		Model: rc.model, Input: interactionInput, SystemInstruction: systemInstruction,
		Store: false,
		Tools: tools, GenerationConfig: interactionGenerationConfig{MaxOutputTokens: rc.maxOutputTokens, ToolChoice: "any"},
	}
	// Terminal capability calls are interpreted locally, not continued as an agent loop.
	// Canonical Mujeeb history is authoritative; never chain an unresolved function call.

	resp, err := c.sendInteractionRequest(ctx, reqBody, rc)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}
	if resp.Status != "completed" && resp.Status != "requires_action" {
		reason := resp.Status
		if len(resp.Errors) > 0 && strings.TrimSpace(resp.Errors[0].Message) != "" {
			reason += ": " + resp.Errors[0].Message
		}
		return ports.CustomerSalesDecisionOutput{}, fmt.Errorf("gemini interaction did not complete: %s", reason)
	}

	decision, err := parseCustomerSalesCapability(resp, tools)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}

	latencyMs := time.Since(startedAt).Milliseconds()
	return ports.CustomerSalesDecisionOutput{
		Decision: decision,
		GeminiInteraction: ports.GeminiInteractionContext{
			PreviousInteractionID:  "",
			ResultingInteractionID: "",
			Store:                  false,
		},
		Usage: ports.CustomerSalesUsageTelemetry{
			InputTokens:         resp.Usage.TotalInputTokens,
			CachedTokens:        resp.Usage.TotalCachedTokens,
			OutputTokens:        resp.Usage.TotalOutputTokens,
			Model:               rc.model,
			EstimatedCostMicros: 0,
			LatencyMs:           latencyMs,
			ModelRequests:       1,
		},
		LatencyMs: latencyMs,
	}, nil
}

func (c *GeminiCustomerSalesAdapter) buildContractSystemInstructionText(entityContractJSON []byte) string {
	text := prompts.CustomerSalesSystemPrompt
	if len(entityContractJSON) > 0 {
		text += "\n\n# Catalog Entity Contract\n" + string(entityContractJSON)
	}
	return text
}

func (c *GeminiCustomerSalesAdapter) sendInteractionRequest(
	ctx context.Context,
	reqBody interactionRequest,
	rc *resolvedAIConfig,
) (interactionResponse, error) {
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return interactionResponse{}, fmt.Errorf("marshal interaction request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	url := strings.TrimRight(rc.baseURL, "/") + "/v1/interactions"
	httpResp, err := doGeminiPOSTWithRetry(reqCtx, c.httpClient, url, rc.apiKey, buf, c.retryPolicy, true, geminiRetryTrace{stage: "customer_decision", model: rc.model, source: geminiConfigSource(rc.configVersion)})
	if err != nil {
		return interactionResponse{}, fmt.Errorf("send interaction request: %w", err)
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if err != nil {
		return interactionResponse{}, fmt.Errorf("read interaction response: %w", err)
	}
	if httpResp.StatusCode >= 400 {
		return interactionResponse{}, fmt.Errorf("gemini interactions http %d: %s", httpResp.StatusCode, string(body))
	}

	var out interactionResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return interactionResponse{}, fmt.Errorf("unmarshal interaction response: %w", err)
	}
	return out, nil
}

func decodeStrictStructuredJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("structured output contains multiple JSON values")
		}
		return fmt.Errorf("decode trailing structured output: %w", err)
	}
	return nil
}

func interactionOutputText(resp interactionResponse) (string, error) {
	for i := len(resp.Steps) - 1; i >= 0; i-- {
		step := resp.Steps[i]
		if step.Type != "model_output" {
			continue
		}
		for _, content := range step.Content {
			if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
				return content.Text, nil
			}
		}
	}
	return "", errors.New("gemini interaction completed without model text output")
}

type interactionRequest struct {
	Model                 string                      `json:"model"`
	Input                 string                      `json:"input"`
	SystemInstruction     string                      `json:"system_instruction,omitempty"`
	PreviousInteractionID string                      `json:"previous_interaction_id,omitempty"`
	Store                 bool                        `json:"store"`
	ResponseFormat        *interactionResponseFormat  `json:"response_format,omitempty"`
	Tools                 []customerSalesTool         `json:"tools,omitempty"`
	GenerationConfig      interactionGenerationConfig `json:"generation_config,omitempty"`
}

type interactionResponseFormat struct {
	Type     string         `json:"type"`
	MimeType string         `json:"mime_type,omitempty"`
	Schema   map[string]any `json:"schema,omitempty"`
}

type interactionGenerationConfig struct {
	MaxOutputTokens int    `json:"max_output_tokens,omitempty"`
	ToolChoice      string `json:"tool_choice,omitempty"`
}

type interactionResponse struct {
	ID     string             `json:"id"`
	Status string             `json:"status"`
	Steps  []interactionStep  `json:"steps"`
	Usage  interactionUsage   `json:"usage"`
	Errors []interactionError `json:"errors,omitempty"`
}

type interactionStep struct {
	ID        string               `json:"id,omitempty"`
	Name      string               `json:"name,omitempty"`
	Arguments json.RawMessage      `json:"arguments,omitempty"`
	Type      string               `json:"type"`
	Content   []interactionContent `json:"content,omitempty"`
}

type interactionContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type interactionUsage struct {
	TotalInputTokens  int `json:"total_input_tokens,omitempty"`
	TotalCachedTokens int `json:"total_cached_tokens,omitempty"`
	TotalOutputTokens int `json:"total_output_tokens,omitempty"`
	TotalTokens       int `json:"total_tokens,omitempty"`
}

type interactionError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// Compile-time assertion: GeminiCustomerSalesAdapter implements ports.CustomerSalesDecisionPort.
var _ ports.CustomerSalesDecisionPort = (*GeminiCustomerSalesAdapter)(nil)

// Package gemini — Contract-aligned Gemini Client.
//
// Implements ports.CustomerSalesDecisionPort, the contract ④ §8 mapping of Mujeeb
// Contract to Gemini API:
//   - Mujeeb System Contract → system_instruction
//   - Mujeeb Input Context → input (contents)
//   - Catalog boundary → Structured Output (responseSchema)
//   - Mujeeb Output Contract → CustomerSalesProposal
//
// Per contract ③ §4, supports Gemini Interactions API with previous_interaction_id
// chaining (store=true per contract ③ §9). Per contract ⑤ §7, includes the
// Catalog Entity Contract in system_instruction. Per contract ④ §4, enforces
// Structured Output via responseSchema. Per contract ⑧ §8, captures usage
// telemetry. Per contract ⑧ §9, captures latency.
//
// This adapter is the sole Gemini implementation of the customer-sales
// decision capability. No generic AI execution path is used.

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

// GeminiCustomerSalesAdapter is the contract ④ §8 implementation of ports.CustomerSalesDecisionPort.
//
// It wraps an existing Client to reuse HTTP machinery (base URL, API key,
// model, system prompt) but adds:
//   - previous_interaction_id chaining per contract ③ §4
//   - Catalog Entity Contract in system_instruction per contract ⑤ §7
//   - Structured Output enforcement for CustomerSalesProposal per contract ④ §4
//   - Usage telemetry capture for AI Trace per contract ⑧ §8
type GeminiCustomerSalesAdapter struct {
	base           *GeminiHTTPClient
	httpClient     *http.Client
	baseURL        string
	apiKey         string
	model          string
	requestTimeout time.Duration
	// configProvider, when set, is called at the start of every Decide
	// call to get the ACTIVE runtime configuration (API key, model, limits).
	// Per §2: the runtime gets the active config from Configuration abstraction,
	// not from static env vars. If nil, falls back to static fields (bootstrap/tests).
	configProvider ports.AIConfigurationProvider
}

// resolvedAIConfig holds the effective values for one Gemini call.
type resolvedAIConfig struct {
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
	}, nil
}

// Decide is the contract ④ §8 method implementing ports.CustomerSalesDecisionPort.
//
// Per contract ③ §4, it carries previous_interaction_id chaining via input.GeminiInteraction.
// Per contract ⑤ §7, the Entity Contract is sent as part of system_instruction.
// Per contract ④ §4, Structured Output enforces the CustomerSalesProposal shape.
// Per contract ⑧ §8, usage telemetry is captured for AI Trace.
//
// Customer Sales uses the Interactions API; catalog evaluation runs in batches.
//
// This method is the Customer Sales decision adapter; no generic AI runtime is used.
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
	previousID := strings.TrimSpace(input.GeminiInteraction.PreviousInteractionID)
	if !input.GeminiInteraction.Store {
		previousID = ""
	}

	interactionInput := buildUserPrompt(input.Request)
	systemInstruction := c.buildContractSystemInstructionText(input.EntityContractPayload)
	if rc.maxInputCharacters > 0 {
		totalChars := len([]rune(interactionInput)) + len([]rune(systemInstruction))
		if totalChars > rc.maxInputCharacters {
			return ports.CustomerSalesDecisionOutput{}, fmt.Errorf(
				"customer sales interaction input exceeds %d characters after context and system instruction serialization: %d",
				rc.maxInputCharacters,
				totalChars,
			)
		}
	}

	reqBody := interactionRequest{
		Model:                 rc.model,
		Input:                 interactionInput,
		SystemInstruction:     systemInstruction,
		PreviousInteractionID: previousID,
		Store:                 input.GeminiInteraction.Store,
		ResponseFormat: interactionResponseFormat{
			Type:     "text",
			MimeType: "application/json",
			Schema:   contractProposalResponseSchema(),
		},
		GenerationConfig: interactionGenerationConfig{
			MaxOutputTokens: rc.maxOutputTokens,
		},
	}

	resp, err := c.sendInteractionRequest(ctx, reqBody, rc)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}
	if resp.Status != "completed" {
		reason := resp.Status
		if len(resp.Errors) > 0 && strings.TrimSpace(resp.Errors[0].Message) != "" {
			reason += ": " + resp.Errors[0].Message
		}
		return ports.CustomerSalesDecisionOutput{}, fmt.Errorf("gemini interaction did not complete: %s", reason)
	}

	raw, err := interactionOutputText(resp)
	if err != nil {
		return ports.CustomerSalesDecisionOutput{}, err
	}
	var proposal ports.CustomerSalesProposal
	if err := decodeStrictStructuredJSON([]byte(raw), &proposal); err != nil {
		return ports.CustomerSalesDecisionOutput{}, fmt.Errorf("decode interaction structured output: %w", err)
	}

	latencyMs := time.Since(startedAt).Milliseconds()
	return ports.CustomerSalesDecisionOutput{
		Proposal: proposal,
		GeminiInteraction: ports.GeminiInteractionContext{
			PreviousInteractionID:  previousID,
			ResultingInteractionID: resp.ID,
			Store:                  input.GeminiInteraction.Store,
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

	url := strings.TrimRight(rc.baseURL, "/") + "/v1beta/interactions"
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return interactionResponse{}, fmt.Errorf("build interaction request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", rc.apiKey)

	httpResp, err := c.httpClient.Do(httpReq)
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

// contractProposalResponseSchema is the JSON Schema that enforces the contract
// ④ §4 output shape via Gemini's responseSchema field.
//
// The contract response schema is shared by contract-aligned AI calls.
func contractProposalResponseSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"status": map[string]any{
				"type": "string",
				"enum": []string{
					string(ports.CustomerSalesProposalStatusResolved),
					string(ports.CustomerSalesProposalStatusAmbiguous),
					string(ports.CustomerSalesProposalStatusNotFound),
					string(ports.CustomerSalesProposalStatusNeedsMoreData),
				},
			},
			"action": map[string]any{
				"type": "string",
				"enum": []string{
					string(ports.CustomerSalesProposalActionAnswer),
					string(ports.CustomerSalesProposalActionClarification),
					string(ports.CustomerSalesProposalActionHumanRequest),
					string(ports.CustomerSalesProposalActionLeadDraft),
					string(ports.CustomerSalesProposalActionOrderDraft),
				},
			},
			"response_text": map[string]any{"type": "string"},
			"routing_reason": map[string]any{
				"type": "string",
				"enum": []string{
					string(ports.CustomerSalesRoutingReasonSubscriptionActivation),
					string(ports.CustomerSalesRoutingReasonCustomerRequestedHuman),
					string(ports.CustomerSalesRoutingReasonOther),
				},
			},
			"selected": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"item_id":    map[string]any{"type": "string"},
						"variant_id": map[string]any{"type": "string"},
						"offer_id":   map[string]any{"type": "string"},
					},
					"required": []string{"item_id"},
				},
			},
		},
		"required": []string{"status", "action", "response_text"},
	}
}

type interactionRequest struct {
	Model                 string                      `json:"model"`
	Input                 string                      `json:"input"`
	SystemInstruction     string                      `json:"system_instruction,omitempty"`
	PreviousInteractionID string                      `json:"previous_interaction_id,omitempty"`
	Store                 bool                        `json:"store"`
	ResponseFormat        interactionResponseFormat   `json:"response_format"`
	GenerationConfig      interactionGenerationConfig `json:"generation_config,omitempty"`
}

type interactionResponseFormat struct {
	Type     string         `json:"type"`
	MimeType string         `json:"mime_type,omitempty"`
	Schema   map[string]any `json:"schema,omitempty"`
}

type interactionGenerationConfig struct {
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`
}

type interactionResponse struct {
	ID     string            `json:"id"`
	Status string            `json:"status"`
	Steps  []interactionStep `json:"steps"`
	Usage  interactionUsage  `json:"usage"`
	Errors []interactionError `json:"errors,omitempty"`
}

type interactionStep struct {
	Type    string               `json:"type"`
	Content []interactionContent `json:"content,omitempty"`
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

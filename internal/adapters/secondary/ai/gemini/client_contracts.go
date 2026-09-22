// Package gemini — Contract-aligned extensions to the Gemini Client.
//
// Implements contracts ③ §4 (Gemini Interactions API previous_interaction_id),
// ④ (AI Input/Output Contract), ⑤ §7 (Catalog Entity Contract sent as system
// context), and ⑧ §6 (Gemini Interaction trace recorded for AI Trace).
//
// This file is additive: it does NOT modify the existing Client.Decide method.
// New code should call DecideContract to get a contract-aligned AIGeminiProposal
// and to use Gemini Interactions API chaining via previous_interaction_id.
//
// Per contract ③ §9, Mujeeb uses store=true to enable previous_interaction_id
// chaining. Per contract ③ §5, Mujeeb retention is canonical; Gemini retention
// is convenience only.
//
// Per contract ④ §8, this method maps Mujeeb Contract to Gemini API:
//   - Mujeeb System Contract → system_instruction
//   - Mujeeb Input Context → input (contents)
//   - Catalog boundary → Function Calling / tool
//   - Mujeeb Output Contract → Structured Output (responseSchema)

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

// ContractConfig extends Config with contract-aligned runtime data.
//
// Per contract ⑤ §7, the Catalog Entity Contract is shared across all batches
// and sent once per AI Runtime invocation. It is built once at startup and
// reused for every DecideContract call.
type ContractConfig struct {
	Base           Config
	EntityContract CatalogEntityContractJSON
}

// CatalogEntityContractJSON is the JSON-serializable Catalog Entity Contract
// payload, sent inside the system_instruction per contract ⑤ §7.
//
// Per contract ⑤ §9, this is NOT repeated per batch — it is the shared
// definition that Gemini sees once per AI Runtime invocation.
type CatalogEntityContractJSON struct {
	EntityContract any `json:"entity_contract"`
	Descriptor     any `json:"descriptor"`
}

// DecideContractInput is the input to DecideContract.
type DecideContractInput struct {
	// AIDecisionInput is the legacy input (BusinessID, ConversationID, Text, Context).
	ports.AIDecisionInput

	// GeminiInteraction carries previous_interaction_id per contract ③ §4.
	// Empty PreviousInteractionID means this is the first turn (no chaining).
	GeminiInteraction ports.GeminiInteractionContext

	// EntityContract is the contract ⑤ §7 payload sent as system context.
	// If nil, no entity contract is appended (used by non-catalog agents).
	EntityContract CatalogEntityContractJSON
}

// DecideContractOutput is the output of DecideContract.
type DecideContractOutput struct {
	// Proposal is the contract ④ §4 structured proposal.
	Proposal ports.AIGeminiProposal

	// GeminiInteraction echoes the input context and is populated with the
	// ResultingInteractionID returned by Gemini. The caller persists this as
	// the new last_gemini_interaction_id on the conversation row per
	// contract ③ §4.
	GeminiInteraction ports.GeminiInteractionContext

	// Usage reports token usage per contract ⑧ §8.
	Usage ContractUsageTelemetry

	// LatencyMs is the Gemini API call latency per contract ⑧ §9.
	LatencyMs int64
}

// ContractUsageTelemetry is the per-call usage data for AI Trace.
//
// Per contract ⑧ §8, the AI Runtime records input_tokens, cached_tokens,
// output_tokens per call. The estimated cost is computed from the model's
// pricing table by the AI Runtime (not by Gemini).
type ContractUsageTelemetry struct {
	InputTokens         int
	CachedTokens        int
	OutputTokens        int
	Model               string
	EstimatedCostMicros int64
}

// ContractClient is a contract-aligned Gemini client.
//
// It wraps the existing Client's HTTP machinery but adds:
//   - previous_interaction_id support (Gemini Interactions API)
//   - Catalog Entity Contract in system_instruction
//   - Structured Output enforcement for AIGeminiProposal
//   - Usage telemetry capture for AI Trace
type ContractClient struct {
	base           *Client
	httpClient     *http.Client
	baseURL        string
	apiKey         string
	model          string
	requestTimeout time.Duration
}

// NewContractClient wraps an existing Client with contract-aligned methods.
func NewContractClient(base *Client) (*ContractClient, error) {
	if base == nil {
		return nil, errors.New("base client is required")
	}
	// Access base client's private fields via the package (same package).
	return &ContractClient{
		base:           base,
		httpClient:     base.httpClient,
		baseURL:        base.baseURL,
		apiKey:         base.apiKey,
		model:          base.model,
		requestTimeout: base.requestTimeout,
	}, nil
}

// DecideContract calls the Gemini Interactions API with previous_interaction_id
// chaining per contract ③ §4, includes the Catalog Entity Contract in
// system_instruction per contract ⑤ §7, and parses the response into the
// contract ④ §4 AIGeminiProposal shape.
//
// Per contract ② §8, this method is NOT used between Catalog Batches — each
// Batch is an independent Interaction. previous_interaction_id is only for
// the customer-facing conversation continuity.
func (c *ContractClient) DecideContract(ctx context.Context, input DecideContractInput) (DecideContractOutput, error) {
	if err := ctx.Err(); err != nil {
		return DecideContractOutput{}, err
	}
	if strings.TrimSpace(input.Text) == "" {
		return DecideContractOutput{}, errors.New("AI input text is required")
	}

	startedAt := time.Now().UTC()

	// Build the Gemini Interactions API request body with previous_interaction_id.
	reqBody := contractGeminiRequest{
		Model:                 c.model,
		PreviousInteractionID: input.GeminiInteraction.PreviousInteractionID,
		Store:                 input.GeminiInteraction.Store,
		SystemInstruction:     c.buildContractSystemInstruction(input.EntityContract),
		Contents:              c.buildContractContents(input.AIDecisionInput),
		GenerationConfig: contractGenerationConfig{
			ResponseMimeType: "application/json",
			ResponseSchema:   contractProposalResponseSchema(),
		},
	}

	resp, err := c.sendContractRequest(ctx, reqBody)
	if err != nil {
		return DecideContractOutput{}, err
	}

	latencyMs := time.Since(startedAt).Milliseconds()

	proposal, err := parseContractProposal(resp)
	if err != nil {
		return DecideContractOutput{}, err
	}

	// Per contract ③ §4, persist the resulting interaction ID for the next turn.
	out := DecideContractOutput{
		Proposal: proposal,
		GeminiInteraction: ports.GeminiInteractionContext{
			PreviousInteractionID:  input.GeminiInteraction.PreviousInteractionID,
			ResultingInteractionID: resp.InteractionID,
			Store:                  input.GeminiInteraction.Store,
		},
		Usage: ContractUsageTelemetry{
			InputTokens:  resp.UsageMetadata.PromptTokenCount,
			CachedTokens: resp.UsageMetadata.CachedContentTokenCount,
			OutputTokens: resp.UsageMetadata.CandidatesTokenCount,
			Model:        c.model,
		},
		LatencyMs: latencyMs,
	}
	return out, nil
}

// buildContractSystemInstruction builds the system_instruction content combining
// the base system prompt + the Catalog Entity Contract per contract ⑤ §7.
//
// Per contract ⑤ §8, this is what tells Gemini the meaning of every enum
// value (pricing_mode, availability_mode, fulfillment_mode, etc.) so it
// never has to guess.
func (c *ContractClient) buildContractSystemInstruction(ec CatalogEntityContractJSON) *contractContent {
	parts := []contractPart{
		{Text: c.base.systemPrompt},
	}
	if ec.EntityContract != nil {
		payload, _ := json.Marshal(ec)
		parts = append(parts, contractPart{
			Text: "\n\n# Catalog Entity Contract (contract ⑤ §7)\n\n" + string(payload),
		})
	}
	return &contractContent{
		Role:  "system",
		Parts: parts,
	}
}

// buildContractContents builds the input contents (the conversation context).
//
// Per contract ④ §3, the input includes: business_context, conversation_context,
// conversation_state, catalog_evidence, user_message.
func (c *ContractClient) buildContractContents(input ports.AIDecisionInput) []contractContent {
	contents := make([]contractContent, 0, 2)
	// Per contract ④ §3, the user_message is the customer's current message.
	contents = append(contents, contractContent{
		Role:  "user",
		Parts: []contractPart{{Text: buildUserPrompt(input)}},
	})
	return contents
}

// sendContractRequest is the HTTP call to the Gemini Interactions API.
//
// Per Google's API, the Interactions endpoint supports previous_interaction_id
// and store=true to enable conversation chaining.
func (c *ContractClient) sendContractRequest(ctx context.Context, reqBody contractGeminiRequest) (contractGeminiResponse, error) {
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return contractGeminiResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	// Per contract ③ §9, Mujeeb uses the Interactions API with store=true.
	// Endpoint: /v1beta/models/{model}:generateContent (with previous_interaction_id)
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)

	reqCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return contractGeminiResponse{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return contractGeminiResponse{}, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return contractGeminiResponse{}, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return contractGeminiResponse{}, fmt.Errorf("gemini http %d: %s", resp.StatusCode, string(body))
	}

	var out contractGeminiResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return contractGeminiResponse{}, fmt.Errorf("unmarshal response: %w", err)
	}
	return out, nil
}

// parseContractProposal extracts the AIGeminiProposal from the structured
// output per contract ④ §4.
//
// Per contract ④ §8, Structured Outputs enforces the JSON shape; Mujeeb
// additionally validates the values (Schema correctness does not imply
// business correctness per contract ⑥ §3).
func parseContractProposal(resp contractGeminiResponse) (ports.AIGeminiProposal, error) {
	if len(resp.Candidates) == 0 {
		return ports.AIGeminiProposal{}, errors.New("no candidates in gemini response per contract ④ §4")
	}
	candidate := resp.Candidates[0]
	if candidate.Content.Parts == nil || len(candidate.Content.Parts) == 0 {
		return ports.AIGeminiProposal{}, errors.New("no content parts in gemini response per contract ④ §4")
	}
	// The structured output is in the first part's text field as JSON.
	raw := candidate.Content.Parts[0].Text
	if strings.TrimSpace(raw) == "" {
		return ports.AIGeminiProposal{}, errors.New("empty structured output text per contract ④ §4")
	}
	var proposal ports.AIGeminiProposal
	if err := json.Unmarshal([]byte(raw), &proposal); err != nil {
		return ports.AIGeminiProposal{}, fmt.Errorf("unmarshal structured output: %w", err)
	}
	return proposal, nil
}

// contractProposalResponseSchema is the JSON Schema that enforces the contract
// ④ §4 output shape via Gemini's responseSchema field.
//
// Per contract ④ §4, the output is exactly:
//
//	status (enum: resolved/ambiguous/not_found/needs_more_data)
//	action (enum: answer/clarification/human_request/lead_draft/order_draft)
//	response_text (string)
//	selected (array of objects with item_id, optional variant_id, optional offer_id)
func contractProposalResponseSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status": map[string]any{
				"type": "string",
				"enum": []string{
					string(ports.AIProposalStatusResolved),
					string(ports.AIProposalStatusAmbiguous),
					string(ports.AIProposalStatusNotFound),
					string(ports.AIProposalStatusNeedsMoreData),
				},
			},
			"action": map[string]any{
				"type": "string",
				"enum": []string{
					string(ports.AIProposalActionAnswer),
					string(ports.AIProposalActionClarification),
					string(ports.AIProposalActionHumanRequest),
					string(ports.AIProposalActionLeadDraft),
					string(ports.AIProposalActionOrderDraft),
				},
			},
			"response_text": map[string]any{"type": "string"},
			"selected": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
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

// contractGeminiRequest is the Interactions API request body.
type contractGeminiRequest struct {
	Model                 string                   `json:"model"`
	PreviousInteractionID string                   `json:"previous_interaction_id,omitempty"`
	Store                 bool                     `json:"store"`
	SystemInstruction     *contractContent         `json:"systemInstruction,omitempty"`
	Contents              []contractContent        `json:"contents"`
	GenerationConfig      contractGenerationConfig `json:"generationConfig"`
}

type contractGenerationConfig struct {
	ResponseMimeType string         `json:"responseMimeType,omitempty"`
	ResponseSchema   map[string]any `json:"responseSchema,omitempty"`
}

type contractContent struct {
	Role  string         `json:"role,omitempty"`
	Parts []contractPart `json:"parts"`
}

type contractPart struct {
	Text string `json:"text,omitempty"`
}

type contractGeminiResponse struct {
	InteractionID string                `json:"interactionId,omitempty"`
	Candidates    []contractCandidate   `json:"candidates"`
	UsageMetadata contractUsageMetadata `json:"usageMetadata"`
}

type contractCandidate struct {
	Content contractContent `json:"content"`
}

type contractUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount,omitempty"`
	CandidatesTokenCount    int `json:"candidatesTokenCount,omitempty"`
	CachedContentTokenCount int `json:"cachedContentTokenCount,omitempty"`
	TotalTokenCount         int `json:"totalTokenCount,omitempty"`
}

// Package gemini — Batch Gemini Client implementation (contract ② §5-6).
//
// Implements the services.BatchGeminiClient interface against the Gemini
// generateContent API with Structured Outputs (responseSchema) enforcement.
//
// Per contract ② §5, Gemini returns ONLY candidates (item_id, variant_ids,
// offer_ids, reason) — not the items back. Mujeeb already knows what it sent.
//
// Per contract ② §6, after all batches complete, a Final Gemini Evaluation
// runs over the aggregated candidate set + customer message + context.
//
// Per contract ② §8, batches are NOT chained via previous_interaction_id;
// each batch is an independent Interaction.
//
// Per contract ④ §8, Structured Outputs enforces the JSON shape via
// responseSchema; Mujeeb additionally validates the values per contract ⑥ §3.

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
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

// BatchClientConfig configures the BatchClient.
type BatchClientConfig struct {
	BaseURL        string
	APIKey         string
	Model          string
	HTTPClient     *http.Client
	RequestTimeout time.Duration
	SystemPrompt   string
}

// BatchClient implements services.BatchGeminiClient via the Gemini
// generateContent API with Structured Outputs.
type BatchClient struct {
	baseURL        string
	apiKey         string
	model          string
	httpClient     *http.Client
	requestTimeout time.Duration
	systemPrompt   string
}

// NewBatchClient wires the BatchClient with the Gemini API credentials.
func NewBatchClient(cfg BatchClientConfig) (*BatchClient, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("gemini API key is required for batch evaluation")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultModel
	}
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	systemPrompt := strings.TrimSpace(cfg.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = defaultBatchSystemPrompt
	}
	return &BatchClient{
		baseURL:        baseURL,
		apiKey:         cfg.APIKey,
		model:          model,
		httpClient:     client,
		requestTimeout: timeout,
		systemPrompt:   systemPrompt,
	}, nil
}

// defaultBatchSystemPrompt is the contract ② §5 system prompt for batch
// evaluation. Per contract ② §5, Gemini returns ONLY candidates, not the
// items back. Per contract ② §1, Gemini infers and compares; Mujeeb does
// NOT do semantic search or product matching (per contract ② "ما أغلقناه").
const defaultBatchSystemPrompt = `You are the Catalog Evaluation agent inside Mujeeb 24.

Your job: examine the catalog items in this batch against the customer's message
and identify which items are candidates that match the customer's intent.

Rules:
1. Return ONLY item_id values that you actually saw in this batch's items[].
2. Do NOT invent item_id, variant_id, or offer_id values.
3. For each candidate, include a short reason explaining why it matches.
4. If no items in this batch match the customer's intent, return an empty candidates array.
5. You are NOT the final decision maker — you only identify candidates.
   The final decision happens in a separate Final Evaluation call.`

// EvaluateBatch implements services.BatchGeminiClient.EvaluateBatch per
// contract ② §5. Sends one batch to Gemini and returns the candidate set.
//
// Per contract ② §8, each batch is an independent Interaction — no
// previous_interaction_id chaining.
//
// Per contract ④ §8, Structured Outputs enforces the response shape via
// responseSchema. Per contract ⑥ §3, Mujeeb additionally validates the
// returned IDs against the actual batch items (the caller does this via
// PostgresReferenceValidator).
func (c *BatchClient) EvaluateBatch(ctx context.Context, input services.BatchEvaluationInput) (ports.CatalogBatchResult, error) {
	if c == nil {
		return ports.CatalogBatchResult{}, errors.New("batch client is not configured")
	}
	if len(input.Batch.Items) == 0 {
		return ports.CatalogBatchResult{BatchNumber: input.BatchNumber}, nil
	}

	// Serialize the batch payload to JSON for the user prompt.
	batchJSON, err := json.Marshal(input.Batch)
	if err != nil {
		return ports.CatalogBatchResult{}, fmt.Errorf("marshal batch payload: %w", err)
	}

	// Build the user prompt: customer message + batch data.
	userPrompt := fmt.Sprintf("Customer message: %s\n\nCatalog batch %d data:\n%s",
		input.CustomerMessage, input.BatchNumber, string(batchJSON))

	// Build the Gemini request with Structured Output enforcement.
	reqBody := batchGeminiRequest{
		Model: c.model,
		SystemInstruction: &batchContent{
			Role:  "system",
			Parts: []batchPart{{Text: c.systemPrompt}},
		},
		Contents: []batchContent{
			{Role: "user", Parts: []batchPart{{Text: userPrompt}}},
		},
		GenerationConfig: batchGenerationConfig{
			ResponseMimeType: "application/json",
			ResponseSchema:   batchCandidateResponseSchema(),
		},
	}

	resp, err := c.sendRequest(ctx, reqBody)
	if err != nil {
		return ports.CatalogBatchResult{}, fmt.Errorf("evaluate batch %d: %w", input.BatchNumber, err)
	}

	candidates, err := parseBatchCandidates(resp)
	if err != nil {
		return ports.CatalogBatchResult{}, fmt.Errorf("parse batch %d candidates: %w", input.BatchNumber, err)
	}

	return ports.CatalogBatchResult{
		BatchNumber: input.BatchNumber,
		Candidates:  candidates,
	}, nil
}

// FinalEvaluate implements services.BatchGeminiClient.FinalEvaluate per
// contract ② §6. Runs the final evaluation over the aggregated candidate
// set + customer message + conversation context.
//
// Per contract ② §6: "الـFinal Gemini لا يحتاج أن يرى الـ1000 منتج مرة
// أخرى. يرى: Customer Message + Conversation Context + Candidate Results +
// الدليل التجاري المرتبط بالمرشحين. ثم يقوم بالقرار النهائي وصياغة الرد."
//
// Per contract ④ §4, the final output is an AIGeminiProposal (status +
// action + response_text + selected[]).
func (c *BatchClient) FinalEvaluate(ctx context.Context, input services.FinalEvaluationInput) (ports.AIGeminiProposal, error) {
	if c == nil {
		return ports.AIGeminiProposal{}, errors.New("batch client is not configured")
	}

	// Serialize the candidate set to JSON.
	candidatesJSON, err := json.Marshal(input.CandidateResults)
	if err != nil {
		return ports.AIGeminiProposal{}, fmt.Errorf("marshal candidate results: %w", err)
	}

	// Build the user prompt: customer message + candidate set.
	userPrompt := fmt.Sprintf("Customer message: %s\n\nAggregated candidate set from catalog evaluation:\n%s\n\nBased on the candidates above, produce your final proposal.",
		input.CustomerMessage, string(candidatesJSON))

	// Build the Gemini request with Structured Output enforcement for
	// AIGeminiProposal per contract ④ §4.
	reqBody := batchGeminiRequest{
		Model: c.model,
		SystemInstruction: &batchContent{
			Role:  "system",
			Parts: []batchPart{{Text: c.systemPrompt + "\n\nYou are now in FINAL EVALUATION mode. Produce a single AIGeminiProposal per the responseSchema. Use the candidates as evidence; do NOT invent item_ids that were not in the candidate set."}},
		},
		Contents: []batchContent{
			{Role: "user", Parts: []batchPart{{Text: userPrompt}}},
		},
		GenerationConfig: batchGenerationConfig{
			ResponseMimeType: "application/json",
			ResponseSchema:   finalProposalResponseSchema(),
		},
	}

	resp, err := c.sendRequest(ctx, reqBody)
	if err != nil {
		return ports.AIGeminiProposal{}, fmt.Errorf("final evaluate: %w", err)
	}

	proposal, err := parseFinalProposal(resp)
	if err != nil {
		return ports.AIGeminiProposal{}, fmt.Errorf("parse final proposal: %w", err)
	}

	return proposal, nil
}

// sendRequest is the HTTP call to the Gemini generateContent API.
func (c *BatchClient) sendRequest(ctx context.Context, reqBody batchGeminiRequest) (batchGeminiResponse, error) {
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return batchGeminiResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)

	reqCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return batchGeminiResponse{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return batchGeminiResponse{}, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return batchGeminiResponse{}, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return batchGeminiResponse{}, fmt.Errorf("gemini http %d: %s", resp.StatusCode, string(body))
	}

	var out batchGeminiResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return batchGeminiResponse{}, fmt.Errorf("unmarshal response: %w", err)
	}
	return out, nil
}

// parseBatchCandidates extracts the candidates array from the structured
// output per contract ② §5.
func parseBatchCandidates(resp batchGeminiResponse) ([]ports.CatalogBatchCandidate, error) {
	if len(resp.Candidates) == 0 {
		return nil, errors.New("no candidates in gemini response per contract ② §5")
	}
	candidate := resp.Candidates[0]
	if len(candidate.Content.Parts) == 0 {
		return nil, errors.New("no content parts in gemini response per contract ② §5")
	}
	raw := candidate.Content.Parts[0].Text
	if strings.TrimSpace(raw) == "" {
		// Empty response = no candidates in this batch (valid per contract ② §5).
		return nil, nil
	}
	// The structured output is a JSON object with a "candidates" array.
	var wrapper struct {
		Candidates []ports.CatalogBatchCandidate `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		return nil, fmt.Errorf("unmarshal candidates wrapper: %w", err)
	}
	return wrapper.Candidates, nil
}

// parseFinalProposal extracts the AIGeminiProposal from the structured
// output per contract ④ §4.
func parseFinalProposal(resp batchGeminiResponse) (ports.AIGeminiProposal, error) {
	if len(resp.Candidates) == 0 {
		return ports.AIGeminiProposal{}, errors.New("no candidates in gemini response per contract ④ §4")
	}
	candidate := resp.Candidates[0]
	if len(candidate.Content.Parts) == 0 {
		return ports.AIGeminiProposal{}, errors.New("no content parts in gemini response per contract ④ §4")
	}
	raw := candidate.Content.Parts[0].Text
	if strings.TrimSpace(raw) == "" {
		return ports.AIGeminiProposal{}, errors.New("empty structured output text per contract ④ §4")
	}
	var proposal ports.AIGeminiProposal
	if err := json.Unmarshal([]byte(raw), &proposal); err != nil {
		return ports.AIGeminiProposal{}, fmt.Errorf("unmarshal final proposal: %w", err)
	}
	return proposal, nil
}

// batchCandidateResponseSchema is the JSON Schema that enforces the contract
// ② §5 batch evaluation output shape via Gemini's responseSchema field.
//
// Per contract ② §5, the output is an object with a "candidates" array.
// Each candidate has: item_id (string), variant_ids (array of string),
// offer_ids (array of string), reason (string).
func batchCandidateResponseSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"candidates": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"item_id":     map[string]any{"type": "string"},
						"variant_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"offer_ids":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"reason":      map[string]any{"type": "string"},
					},
					"required": []string{"item_id"},
				},
			},
		},
		"required": []string{"candidates"},
	}
}

// finalProposalResponseSchema is the JSON Schema that enforces the contract
// ④ §4 final proposal output shape (same as the regular proposal schema).
func finalProposalResponseSchema() map[string]any {
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

// batchGeminiRequest is the generateContent request body.
type batchGeminiRequest struct {
	Model             string                `json:"model"`
	SystemInstruction *batchContent         `json:"systemInstruction,omitempty"`
	Contents          []batchContent        `json:"contents"`
	GenerationConfig  batchGenerationConfig `json:"generationConfig"`
}

type batchGenerationConfig struct {
	ResponseMimeType string         `json:"responseMimeType,omitempty"`
	ResponseSchema   map[string]any `json:"responseSchema,omitempty"`
}

type batchContent struct {
	Role  string      `json:"role,omitempty"`
	Parts []batchPart `json:"parts"`
}

type batchPart struct {
	Text string `json:"text,omitempty"`
}

type batchGeminiResponse struct {
	Candidates    []batchCandidate   `json:"candidates"`
	UsageMetadata batchUsageMetadata `json:"usageMetadata"`
}

type batchCandidate struct {
	Content batchContent `json:"content"`
}

type batchUsageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount,omitempty"`
	CandidatesTokenCount    int `json:"candidatesTokenCount,omitempty"`
	CachedContentTokenCount int `json:"cachedContentTokenCount,omitempty"`
	TotalTokenCount         int `json:"totalTokenCount,omitempty"`
}

// Compile-time assertion: BatchClient implements services.BatchGeminiClient.
var _ services.BatchGeminiClient = (*BatchClient)(nil)

package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/prompts"
)

type catalogRequestBuilder struct{ systemPrompt string }
type catalogRequestConfig struct {
	model           string
	maxOutputTokens int
}

// Catalog prompts, schema and evidence boundaries remain unchanged.
func (c *catalogRequestBuilder) buildBatchSystemInstruction(entityContract services.CatalogEntityContractPayload) *batchContent {
	parts := []batchPart{{Text: c.systemPrompt}}
	contractJSON, err := json.Marshal(entityContract)
	if err == nil && len(contractJSON) > 0 {
		parts = append(parts, batchPart{
			Text: "\n\n# Catalog Entity Contract (contract ⑤ §7)\n\n" + string(contractJSON),
		})
	}
	return &batchContent{Role: "system", Parts: parts}
}

// buildBatchSystemInstructionWithSuffix is the same as buildBatchSystemInstruction
// but appends a suffix to the system prompt (used by FinalEvaluateWithDetails
// for the FINAL EVALUATION mode marker). The Catalog Entity Contract is always
// injected as part 1, same as the non-suffix variant.
func (c *catalogRequestBuilder) buildBatchSystemInstructionWithSuffix(entityContract services.CatalogEntityContractPayload, suffix string) *batchContent {
	parts := []batchPart{{Text: c.systemPrompt + suffix}}
	contractJSON, err := json.Marshal(entityContract)
	if err == nil && len(contractJSON) > 0 {
		parts = append(parts, batchPart{
			Text: "\n\n# Catalog Entity Contract (contract ⑤ §7)\n\n" + string(contractJSON),
		})
	}
	return &batchContent{Role: "system", Parts: parts}
}

// defaultBatchSystemPrompt moved to internal/domain/ai/prompts/prompts.go
// (Day 5 Gap #13 — versioned system prompts as reviewable assets).

func (c *catalogRequestBuilder) buildEvaluateBatchRequest(input services.BatchEvaluationInput, rc *catalogRequestConfig) (catalogPromptRequest, error) {
	batchJSON, err := json.Marshal(input.Batch)
	if err != nil {
		return catalogPromptRequest{}, fmt.Errorf("marshal batch payload: %w", err)
	}
	contextJSON, err := json.Marshal(catalogBatchPromptContextFrom(&input.ConversationContext))
	if err != nil {
		return catalogPromptRequest{}, fmt.Errorf("marshal conversation context for batch: %w", err)
	}
	phaseLabel := "Catalog batch"
	systemInstruction := c.buildBatchSystemInstruction(input.EntityContract)
	if input.Reduction {
		phaseLabel = "Candidate reduction batch"
		systemInstruction = c.buildBatchSystemInstructionWithSuffix(input.EntityContract, prompts.CandidateReductionSystemPromptSuffix)
	}
	userPrompt := fmt.Sprintf("Customer message: %s\n\nVerified conversation context:\n%s\n\n%s %d data:\n%s",
		input.CustomerMessage, string(contextJSON), phaseLabel, input.BatchNumber, string(batchJSON))
	return catalogPromptRequest{
		Model:             "models/" + strings.TrimPrefix(rc.model, "models/"),
		SystemInstruction: systemInstruction,
		Contents: []batchContent{
			{Role: "user", Parts: []batchPart{{Text: userPrompt}}},
		},
		GenerationConfig: catalogGenerationConfig{
			ResponseMimeType: "application/json",
			ResponseSchema:   batchCandidateResponseSchemaForBatch(input.Batch),
			MaxOutputTokens:  rc.maxOutputTokens,
		},
	}, nil
}

// CountBatchTokens uses the provider tokenizer with the batch prompt and JSON
// schema. The countTokens compatibility envelope is not a generation call;
// Interactions usage remains authoritative for billing.

func (c *catalogRequestBuilder) buildFinalEvaluateRequest(input services.FinalEvaluationInput, userPrompt string, rc *catalogRequestConfig) catalogPromptRequest {
	return catalogPromptRequest{
		Model:             "models/" + strings.TrimPrefix(rc.model, "models/"),
		SystemInstruction: finalCustomerSalesInstruction(input.EntityContract),
		capabilityTools:   customerSalesTools(ports.CustomerSalesCapabilitiesForConsent(false, input.ConversationContext.Sales != nil && input.ConversationContext.Sales.Enabled && input.ConversationContext.Sales.AllowInteractions, input.ConversationContext.Sales != nil && input.ConversationContext.Sales.StartRequestID != "")),
		Contents: []batchContent{
			{Role: "user", Parts: []batchPart{{Text: userPrompt}}},
		},
		GenerationConfig: catalogGenerationConfig{
			MaxOutputTokens: rc.maxOutputTokens,
		},
	}
}

func parseBatchCandidates(resp catalogCandidateResponse) ([]ports.CatalogBatchCandidate, error) {
	if len(resp.Candidates) == 0 {
		return nil, errors.New("no candidates in model response per contract ② §5")
	}
	candidate := resp.Candidates[0]
	if len(candidate.Content.Parts) == 0 {
		return nil, errors.New("no content parts in model response per contract ② §5")
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
	if err := decodeStrictStructuredJSON([]byte(raw), &wrapper); err != nil {
		return nil, fmt.Errorf("decode candidates wrapper: %w", err)
	}
	return wrapper.Candidates, nil
}

// batchCandidateResponseSchema is the JSON Schema that enforces the contract
// ② §5 batch evaluation output shape via response_format.schema.
//
// Per contract ② §5, the output is an object with a "candidates" array.
// Each candidate has: item_id (string), variant_ids (array of string),
// offer_ids (array of string), reason (string).
func batchCandidateResponseSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"candidates": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"item_id":     map[string]any{"type": "string"},
						"variant_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"offer_ids":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"reason":      map[string]any{"type": "string"},
					},
					"required": []string{"item_id", "variant_ids", "offer_ids", "reason"},
				},
			},
		},
		"required": []string{"candidates"},
	}
}

// Restrict structured-output references to exact identifiers present in this
// batch. Go still validates every item/variant/offer relationship.
func batchCandidateResponseSchemaForBatch(batch services.CatalogAIBatchPayload) map[string]any {
	schema := batchCandidateResponseSchema()
	props := schema["properties"].(map[string]any)["candidates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	itemIDs := make([]string, 0, len(batch.Items))
	variantIDs := make([]string, 0)
	offerIDs := make([]string, 0)
	itemSeen := make(map[string]struct{})
	variantSeen := make(map[string]struct{})
	offerSeen := make(map[string]struct{})
	for _, item := range batch.Items {
		if _, seen := itemSeen[item.ID]; !seen {
			itemIDs = append(itemIDs, item.ID)
			itemSeen[item.ID] = struct{}{}
		}
		for _, variant := range item.Variants {
			if _, seen := variantSeen[variant.ID]; !seen {
				variantIDs = append(variantIDs, variant.ID)
				variantSeen[variant.ID] = struct{}{}
			}
		}
		for _, offer := range item.Offers {
			if _, seen := offerSeen[offer.ID]; !seen {
				offerIDs = append(offerIDs, offer.ID)
				offerSeen[offer.ID] = struct{}{}
			}
		}
	}
	if len(itemIDs) > 0 {
		props["item_id"].(map[string]any)["enum"] = itemIDs
	}
	if len(variantIDs) > 0 {
		props["variant_ids"].(map[string]any)["items"].(map[string]any)["enum"] = variantIDs
	}
	if len(offerIDs) > 0 {
		props["offer_ids"].(map[string]any)["items"].(map[string]any)["enum"] = offerIDs
	}
	return schema
}

type catalogPromptRequest struct {
	Tools             []map[string]any `json:"tools,omitempty"`
	ToolConfig        map[string]any   `json:"toolConfig,omitempty"`
	capabilityTools   []customerSalesTool
	Model             string                  `json:"model,omitempty"`
	SystemInstruction *batchContent           `json:"systemInstruction,omitempty"`
	Contents          []batchContent          `json:"contents"`
	GenerationConfig  catalogGenerationConfig `json:"generationConfig"`
}

type catalogGenerationConfig struct {
	ResponseMimeType string         `json:"responseMimeType,omitempty"`
	ResponseSchema   map[string]any `json:"responseJsonSchema,omitempty"`
	MaxOutputTokens  int            `json:"maxOutputTokens,omitempty"`
}

type batchContent struct {
	Role  string      `json:"role,omitempty"`
	Parts []batchPart `json:"parts"`
}

type batchPart struct {
	Text string `json:"text,omitempty"`
}

type catalogCandidateResponse struct {
	Decision      ports.CapabilityDecision
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

// batchChatRequest turns the existing catalog prompt builder's two trust
// domains into the shared Chat Completions wire contract. No Gemini endpoint
// or provider-specific response envelope is used for generation.
func batchChatRequest(request catalogPromptRequest, model string) (chatCompletionRequest, error) {
	var system, input []string
	if request.SystemInstruction != nil {
		for _, part := range request.SystemInstruction.Parts {
			system = append(system, part.Text)
		}
	}
	for _, content := range request.Contents {
		if content.Role != "user" {
			return chatCompletionRequest{}, errors.New("catalog data must be carried only in user messages")
		}
		for _, part := range content.Parts {
			input = append(input, part.Text)
		}
	}
	if len(request.capabilityTools) > 0 {
		return capabilityChatRequest(model, strings.Join(system, "\n\n"), strings.Join(input, "\n\n"), request.capabilityTools, request.GenerationConfig.MaxOutputTokens)
	}
	schema := request.GenerationConfig.ResponseSchema
	if err := validateStrictSchema(schema); err != nil {
		return chatCompletionRequest{}, fmt.Errorf("catalog batch schema invalid: %w", err)
	}
	format := &chatResponseFormat{Type: "json_schema"}
	format.JSONSchema.Name = "catalog_batch_candidates"
	format.JSONSchema.Strict = true
	format.JSONSchema.Schema = schema
	return chatCompletionRequest{
		Model: model,
		Messages: []chatCompletionMessage{
			{Role: "system", Content: strings.Join(system, "\n\n")},
			{Role: "user", Content: strings.Join(input, "\n\n")},
		},
		ResponseFormat: format, MaxTokens: request.GenerationConfig.MaxOutputTokens,
		Stream: false,
	}, nil
}

func finalCustomerSalesInstruction(contract services.CatalogEntityContractPayload) *batchContent {
	encoded, _ := json.Marshal(contract)
	return &batchContent{Role: "system", Parts: []batchPart{{Text: prompts.FinalEvaluationSystemPrompt}, {Text: "Catalog Entity Contract\n" + string(encoded)}}}
}

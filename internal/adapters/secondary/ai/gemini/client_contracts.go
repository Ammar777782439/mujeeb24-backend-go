// Package gemini — Contract-aligned Gemini Client.
//
// Implements ports.ContractRuntime, the contract ④ §8 mapping of Mujeeb
// Contract to Gemini API:
//   - Mujeeb System Contract → system_instruction
//   - Mujeeb Input Context → input (contents)
//   - Catalog boundary → Structured Output (responseSchema)
//   - Mujeeb Output Contract → AIGeminiProposal
//
// Per contract ③ §4, supports Gemini Interactions API with previous_interaction_id
// chaining (store=true per contract ③ §9). Per contract ⑤ §7, includes the
// Catalog Entity Contract in system_instruction. Per contract ④ §4, enforces
// Structured Output via responseSchema. Per contract ⑧ §8, captures usage
// telemetry. Per contract ⑧ §9, captures latency.
//
// This file REPLACES the legacy Client.Decide method (in client.go) for all
// new contract-aligned callers (AutoReplyService, MerchantCatalogAIAgent).
// The legacy Client.Decide is kept only for migration; new code must use
// ContractClient.

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

// ContractClient is the contract ④ §8 implementation of ports.ContractRuntime.
//
// It wraps an existing Client to reuse HTTP machinery (base URL, API key,
// model, system prompt) but adds:
//   - previous_interaction_id chaining per contract ③ §4
//   - Catalog Entity Contract in system_instruction per contract ⑤ §7
//   - Structured Output enforcement for AIGeminiProposal per contract ④ §4
//   - Usage telemetry capture for AI Trace per contract ⑧ §8
type ContractClient struct {
        base           *Client
        httpClient     *http.Client
        baseURL        string
        apiKey         string
        model          string
        requestTimeout time.Duration
}

// NewContractClient wraps an existing Client with contract-aligned methods.
//
// Per contract ⑤ §7, the Catalog Entity Contract is built once at startup
// and reused for every call; callers pass it via ContractRuntimeInput.
func NewContractClient(base *Client) (*ContractClient, error) {
        if base == nil {
                return nil, errors.New("base client is required")
        }
        return &ContractClient{
                base:           base,
                httpClient:     base.httpClient,
                baseURL:        base.baseURL,
                apiKey:         base.apiKey,
                model:          base.model,
                requestTimeout: base.requestTimeout,
        }, nil
}

// DecideContract is the contract ④ §8 method implementing ports.ContractRuntime.
//
// Per contract ③ §4, it carries previous_interaction_id chaining via input.GeminiInteraction.
// Per contract ⑤ §7, the Entity Contract is sent as part of system_instruction.
// Per contract ④ §4, Structured Output enforces the AIGeminiProposal shape.
// Per contract ⑧ §8, usage telemetry is captured for AI Trace.
//
// This method supersedes the legacy ports.AIRuntime.Decide.
func (c *ContractClient) DecideContract(ctx context.Context, input ports.ContractRuntimeInput) (ports.ContractRuntimeOutput, error) {
        if err := ctx.Err(); err != nil {
                return ports.ContractRuntimeOutput{}, err
        }
        if strings.TrimSpace(input.DecisionInput.Text) == "" {
                return ports.ContractRuntimeOutput{}, errors.New("AI input text is required")
        }

        startedAt := time.Now().UTC()

        // Build the Gemini Interactions API request body.
        reqBody := contractGeminiRequest{
                Model:                 c.model,
                PreviousInteractionID: input.GeminiInteraction.PreviousInteractionID,
                Store:                 input.GeminiInteraction.Store,
                SystemInstruction:     c.buildContractSystemInstruction(input.EntityContractPayload),
                Contents:              c.buildContractContents(input.DecisionInput),
                GenerationConfig: contractGenerationConfig{
                        ResponseMimeType: "application/json",
                        ResponseSchema:   contractProposalResponseSchema(),
                },
        }

        resp, err := c.sendContractRequest(ctx, reqBody)
        if err != nil {
                return ports.ContractRuntimeOutput{}, err
        }

        latencyMs := time.Since(startedAt).Milliseconds()

        proposal, err := parseContractProposal(resp)
        if err != nil {
                return ports.ContractRuntimeOutput{}, err
        }

        return ports.ContractRuntimeOutput{
                Proposal: proposal,
                GeminiInteraction: ports.GeminiInteractionContext{
                        PreviousInteractionID:  input.GeminiInteraction.PreviousInteractionID,
                        ResultingInteractionID: resp.InteractionID,
                        Store:                  input.GeminiInteraction.Store,
                },
                Usage: ports.ContractUsageTelemetry{
                        InputTokens:         resp.UsageMetadata.PromptTokenCount,
                        CachedTokens:        resp.UsageMetadata.CachedContentTokenCount,
                        OutputTokens:        resp.UsageMetadata.CandidatesTokenCount,
                        Model:               c.model,
                        EstimatedCostMicros: 0, // computed by caller using pricing table
                },
                LatencyMs: latencyMs,
        }, nil
}

// buildContractSystemInstruction builds the system_instruction content combining
// the base system prompt + the Catalog Entity Contract per contract ⑤ §7.
//
// Per contract ⑤ §8, this tells Gemini the meaning of every enum value so it
// never has to guess.
func (c *ContractClient) buildContractSystemInstruction(entityContractJSON []byte) *contractContent {
        parts := []contractPart{
                {Text: c.base.systemPrompt},
        }
        if len(entityContractJSON) > 0 {
                parts = append(parts, contractPart{
                        Text: "\n\n# Catalog Entity Contract (contract ⑤ §7)\n\n" + string(entityContractJSON),
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
        // The existing client.go has buildUserPrompt(input) which encodes the
        // AIContext (Business, Conversation, Customer, CatalogEvidence, etc.) into
        // the user-facing prompt text. We reuse it for the contract-aligned path.
        return []contractContent{
                {
                        Role:  "user",
                        Parts: []contractPart{{Text: buildUserPrompt(input)}},
                },
        }
}

// sendContractRequest is the HTTP call to the Gemini Interactions API.
//
// Per contract ③ §9, Mujeeb uses store=true to enable previous_interaction_id.
// Per contract ⑨ §11, every external operation has a Timeout.
func (c *ContractClient) sendContractRequest(ctx context.Context, reqBody contractGeminiRequest) (contractGeminiResponse, error) {
        buf, err := json.Marshal(reqBody)
        if err != nil {
                return contractGeminiResponse{}, fmt.Errorf("marshal request: %w", err)
        }

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
// additionally validates the values (per contract ⑥ §3).
func parseContractProposal(resp contractGeminiResponse) (ports.AIGeminiProposal, error) {
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
                return ports.AIGeminiProposal{}, fmt.Errorf("unmarshal structured output: %w", err)
        }
        return proposal, nil
}

// contractProposalResponseSchema is the JSON Schema that enforces the contract
// ④ §4 output shape via Gemini's responseSchema field.
//
// Per ADR-044 layer 2, the schema includes an optional `proposal` field
// used by the B2B MerchantCatalogAI to return a structured catalog
// operation payload (create/update/delete with item/variants/offers).
// B2C CustomerSalesAI leaves this field empty (its proposals are
// reference-only via selected[]).
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
                        // Per ADR-044 layer 2 — structured catalog operation payload.
                        // Optional: only B2B MerchantCatalogAI populates this for
                        // mutations (create/update/delete). B2C leaves it empty.
                        "proposal": map[string]any{
                                "type": "object",
                                "properties": map[string]any{
                                        "operation": map[string]any{
                                                "type": "string",
                                                "enum": []string{"create", "update", "delete"},
                                        },
                                        "create": map[string]any{
                                                "type": "object",
                                                "properties": map[string]any{
                                                        "item": map[string]any{
                                                                "type": "object",
                                                                "properties": map[string]any{
                                                                        "name":                  map[string]any{"type": "string"},
                                                                        "item_type":             map[string]any{"type": "string"},
                                                                        "short_description":     map[string]any{"type": "string"},
                                                                        "long_description":      map[string]any{"type": "string"},
                                                                        "pricing_mode":          map[string]any{"type": "string"},
                                                                        "availability_mode":     map[string]any{"type": "string"},
                                                                        "fulfillment_mode":      map[string]any{"type": "string"},
                                                                        "requires_confirmation": map[string]any{"type": "boolean"},
                                                                        "attributes":            map[string]any{"type": "object"},
                                                                },
                                                                "required": []string{"name", "item_type", "pricing_mode", "availability_mode", "fulfillment_mode", "requires_confirmation"},
                                                        },
                                                        "variants": map[string]any{
                                                                "type": "array",
                                                                "items": map[string]any{
                                                                        "type": "object",
                                                                        "properties": map[string]any{
                                                                                "name":       map[string]any{"type": "string"},
                                                                                "attributes": map[string]any{"type": "object"},
                                                                        },
                                                                        "required": []string{"name"},
                                                                },
                                                        },
                                                        "offers": map[string]any{
                                                                "type": "array",
                                                                "items": map[string]any{
                                                                        "type": "object",
                                                                        "properties": map[string]any{
                                                                                "variant_name_ref":     map[string]any{"type": "string"},
                                                                                "name":                 map[string]any{"type": "string"},
                                                                                "pricing_mode":        map[string]any{"type": "string"},
                                                                                "amount":               map[string]any{"type": "string"},
                                                                                "currency":             map[string]any{"type": "string"},
                                                                                "pricing_unit":         map[string]any{"type": "string"},
                                                                                "price_source":         map[string]any{"type": "string"},
                                                                                "availability_mode":    map[string]any{"type": "string"},
                                                                                "availability_status":  map[string]any{"type": "string"},
                                                                                "fulfillment_mode":     map[string]any{"type": "string"},
                                                                                "validity_from":        map[string]any{"type": "string"},
                                                                                "validity_until":       map[string]any{"type": "string"},
                                                                        },
                                                                        "required": []string{"name", "pricing_mode"},
                                                                },
                                                        },
                                                },
                                        },
                                        "update": map[string]any{
                                                "type": "object",
                                                "properties": map[string]any{
                                                        "item_id":      map[string]any{"type": "string"},
                                                        "changes": map[string]any{
                                                                "type": "object",
                                                                "properties": map[string]any{
                                                                        "name":                  map[string]any{"type": "string"},
                                                                        "item_type":             map[string]any{"type": "string"},
                                                                        "short_description":     map[string]any{"type": "string"},
                                                                        "long_description":      map[string]any{"type": "string"},
                                                                        "pricing_mode":          map[string]any{"type": "string"},
                                                                        "availability_mode":     map[string]any{"type": "string"},
                                                                        "fulfillment_mode":      map[string]any{"type": "string"},
                                                                        "requires_confirmation": map[string]any{"type": "boolean"},
                                                                        "attributes":            map[string]any{"type": "object"},
                                                                },
                                                        },
                                                        "new_variants": map[string]any{
                                                                "type": "array",
                                                                "items": map[string]any{
                                                                        "type": "object",
                                                                        "properties": map[string]any{
                                                                                "name":       map[string]any{"type": "string"},
                                                                                "attributes": map[string]any{"type": "object"},
                                                                        },
                                                                },
                                                        },
                                                        "new_offers": map[string]any{
                                                                "type": "array",
                                                                "items": map[string]any{
                                                                        "type": "object",
                                                                        "properties": map[string]any{
                                                                                "variant_name_ref": map[string]any{"type": "string"},
                                                                                "name":             map[string]any{"type": "string"},
                                                                                "pricing_mode":     map[string]any{"type": "string"},
                                                                                "amount":           map[string]any{"type": "string"},
                                                                                "currency":         map[string]any{"type": "string"},
                                                                        },
                                                                },
                                                        },
                                                },
                                                "required": []string{"item_id"},
                                        },
                                        "delete": map[string]any{
                                                "type": "object",
                                                "properties": map[string]any{
                                                        "item_id":      map[string]any{"type": "string"},
                                                        "confirmed":    map[string]any{"type": "boolean"},
                                                        "reason_given": map[string]any{"type": "string"},
                                                },
                                                "required": []string{"item_id"},
                                        },
                                },
                                "required": []string{"operation"},
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

// Compile-time assertion: ContractClient implements ports.ContractRuntime.
var _ ports.ContractRuntime = (*ContractClient)(nil)

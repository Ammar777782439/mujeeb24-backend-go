// Package gemini — Gemini HTTP Client (Config Holder + Prompt Builder).
//
// This file holds ONLY:
//   1. Client struct (config: API key, model, base URL, HTTP client, system prompt)
//   2. NewClient constructor
//   3. Getters (BaseURL, APIKey, Model) — used by ContractClient and bootstrap
//   4. buildUserPrompt — used by ContractClient to build the user-facing prompt
//
// The LEGACY Client.Decide method, proposalWire, proposalJSONSchema,
// safetyTurnBudget loop, sendRequest, and all legacy types were REMOVED
// per the "purge dead code" directive. The contract-aligned path uses
// ContractClient.DecideContract (in client_contracts.go) which has its
// own HTTP send method and Structured Output enforcement.
//
// Per contract ④ §8:
//   Mujeeb System Contract → system_instruction
//   Mujeeb Input Context → input (contents)
//   Catalog boundary → Structured Output (responseSchema)
//   Mujeeb Output Contract → AIGeminiProposal
//
// Per contract ④ §3 (No Execution): the Gemini adapter contains NO database
// imports (no pgx, no sql, no database). It does HTTP only.

package gemini

import (
        "context"
        "encoding/json"
        "errors"
        "fmt"
        "net/http"
        "net/url"
        "strings"
        "time"

        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
        "github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/prompts"
)

const (
        defaultMaxOutputTokens = 2048
        defaultBaseURL         = "https://generativelanguage.googleapis.com"
        defaultModel           = "gemini-3.5-flash-lite"
)

// Config contains only runtime configuration. API keys are never copied
// into a proposal, error, log, or domain record (per contract ⑧ §23).
type Config struct {
        BaseURL            string
        APIKey             string
        Model              string
        HTTPClient         *http.Client
        RequestTimeout     time.Duration
        MaxOutputTokens    int
        MaxInputCharacters int
        SystemPrompt       string
        Capabilities       ports.AICapabilityDispatcher
}

// Client is the Gemini HTTP client. It holds config only — the actual
// contract-aligned API calls are in ContractClient (client_contracts.go)
// which wraps this Client.
//
// Per contract ④ §3 (No Execution): this client does HTTP only.
// No database imports, no external API calls beyond Gemini.
type Client struct {
        baseURL            string
        apiKey             string
        model              string
        httpClient         *http.Client
        requestTimeout     time.Duration
        maxOutputTokens    int
        maxInputCharacters int
        systemPrompt       string
        capabilities       ports.AICapabilityDispatcher
}

// NewClient creates a Gemini HTTP client with the given config.
//
// Per contract ④ §2, the system prompt defaults to the versioned
// prompts.CustomerSalesSystemPrompt from the prompts package.
func NewClient(cfg Config) (*Client, error) {
        baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
        if baseURL == "" {
                baseURL = defaultBaseURL
        }
        parsed, err := url.Parse(baseURL)
        if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
                return nil, errors.New("gemini base URL must be an absolute HTTP or HTTPS URL")
        }
        if strings.TrimSpace(cfg.APIKey) == "" {
                return nil, errors.New("gemini API key is required")
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
        maxInputCharacters := cfg.MaxInputCharacters
        if maxInputCharacters <= 0 {
                maxInputCharacters = 12000
        }
        client := cfg.HTTPClient
        if client == nil {
                client = &http.Client{}
        }
        systemPrompt := strings.TrimSpace(cfg.SystemPrompt)
        if systemPrompt == "" {
                // Per contract ④ §2, the system prompt is a versioned asset.
                systemPrompt = prompts.CustomerSalesSystemPrompt
        }
        return &Client{
                baseURL:            baseURL,
                apiKey:             strings.TrimSpace(cfg.APIKey),
                model:              model,
                httpClient:         client,
                requestTimeout:     requestTimeout,
                maxOutputTokens:    maxOutputTokens,
                maxInputCharacters: maxInputCharacters,
                systemPrompt:       systemPrompt,
                capabilities:       cfg.Capabilities,
        }, nil
}

// BaseURL returns the configured Gemini API base URL.
func (c *Client) BaseURL() string { return c.baseURL }

// APIKey returns the configured Gemini API key.
func (c *Client) APIKey() string { return c.apiKey }

// Model returns the configured Gemini model name.
func (c *Client) Model() string { return c.model }

// SystemPrompt returns the configured system prompt.
func (c *Client) SystemPrompt() string { return c.systemPrompt }

// MaxInputCharacters returns the max input character limit.
func (c *Client) MaxInputCharacters() int { return c.maxInputCharacters }

// MaxOutputTokens returns the max output token limit.
func (c *Client) MaxOutputTokens() int { return c.maxOutputTokens }

// RequestTimeout returns the configured request timeout.
func (c *Client) RequestTimeout() time.Duration { return c.requestTimeout }

// HTTPClient returns the configured HTTP client (shared with ContractClient).
func (c *Client) HTTPClient() *http.Client { return c.httpClient }

// Capabilities returns the configured AI capability dispatcher.
func (c *Client) Capabilities() ports.AICapabilityDispatcher { return c.capabilities }

// buildUserPrompt encodes the AIContext + customer message into the
// user-facing prompt text for Gemini. Used by ContractClient.
//
// Per contract ④ §3, the input includes: business_context,
// conversation_context, conversation_state, catalog_evidence, user_message.
func buildUserPrompt(input ports.AIDecisionInput) string {
        prompt := fmt.Sprintf("Business ID: %s\nConversation ID: %s\nChannel: %s\nPolicy version: %s\nSource message reference: %s\nCustomer message:\n%s",
                input.BusinessID, input.ConversationID, input.Channel, input.PolicyVersion,
                input.SourceMessageReference, strings.TrimSpace(input.Text))
        if input.Context == nil {
                return prompt
        }
        encoded, err := json.Marshal(promptContextFrom(input.Context))
        if err != nil {
                return prompt + "\nVerified Mujeeb context: unavailable"
        }
        return prompt + "\nVerified Mujeeb context (evidence only; do not infer missing facts):\n" + string(encoded)
}

// promptContext is the JSON-serialized context sent to Gemini.
type promptContext struct {
        SchemaVersion          int                              `json:"schema_version"`
        Freshness              string                           `json:"freshness"`
        Business               ports.AIContextBusiness          `json:"business"`
        Conversation           ports.AIContextConversation      `json:"conversation"`
        Customer               promptCustomerContext            `json:"customer"`
        CatalogEvidence        []ports.AICatalogEvidence        `json:"catalog_evidence"`
        CatalogSummary         []ports.CatalogSummaryEntry      `json:"catalog_summary,omitempty"`
        OfferEvidence          []ports.AIOfferEvidence          `json:"offer_evidence"`
        VariantEvidence        []ports.AIVariantEvidence        `json:"variant_evidence"`
        KnowledgeEvidence      []ports.AIKnowledgeEvidence      `json:"knowledge_evidence"`
        BusinessPolicyEvidence []ports.AIBusinessPolicyEvidence `json:"business_policy_evidence"`
        RecentMessages         []ports.AIRecentMessageEvidence  `json:"recent_messages"`
        PolicyEvidence         ports.AIPolicyEvidence           `json:"policy_evidence"`
        KnowledgeState         string                           `json:"knowledge_state"`
        ConversationState      *ports.ConversationStateRecord   `json:"conversation_state,omitempty"`
        // ConversationSummary is the running LLM-generated summary of older
        // conversation turns (everything before the sliding window of
        // recent_messages). Per ADR-039. Empty when the conversation is
        // short (less than SummaryInterval turns).
        ConversationSummary    string                           `json:"conversation_summary,omitempty"`
        GeneratedAt            time.Time                        `json:"generated_at"`
        ExpiresAt              time.Time                        `json:"expires_at"`
}

type promptCustomerContext struct {
        Reference        string `json:"reference"`
        LocalePreference string `json:"locale_preference"`
        Status           string `json:"status"`
}

func promptContextFrom(value *ports.AIContext) promptContext {
        return promptContext{
                SchemaVersion:          value.SchemaVersion,
                Freshness:              value.Freshness,
                Business:               value.Business,
                Conversation:           value.Conversation,
                Customer:               promptCustomerContext{Reference: value.Customer.Reference, LocalePreference: value.Customer.LocalePreference, Status: value.Customer.Status},
                CatalogEvidence:        value.CatalogEvidence,
                OfferEvidence:          value.OfferEvidence,
                VariantEvidence:        value.VariantEvidence,
                KnowledgeEvidence:      value.KnowledgeEvidence,
                BusinessPolicyEvidence: value.BusinessPolicyEvidence,
                RecentMessages:         value.RecentMessages,
                PolicyEvidence:         value.PolicyEvidence,
                KnowledgeState:         value.KnowledgeState,
                ConversationState:      value.ConversationState,
                CatalogSummary:         value.CatalogSummary,
                ConversationSummary:    value.ConversationSummary,
                GeneratedAt:            value.GeneratedAt,
                ExpiresAt:              value.ExpiresAt,
        }
}

// Decide implements ports.AIRuntime with a SIMPLE single-call flow.
//
// This is NOT the legacy tool-calling loop. It makes one generateContent
// call, parses the structured JSON response, and returns AIDecisionProposal.
//
// Production code uses ContractClient.DecideContract (in client_contracts.go)
// which returns the contract ④ §4 AIGeminiProposal with Structured Output
// enforcement. This method exists only to satisfy the ports.AIRuntime
// interface so the bootstrap type assertion works.
func (c *Client) Decide(ctx context.Context, input ports.AIDecisionInput) (ports.AIDecisionProposal, error) {
        if c == nil {
                return ports.AIDecisionProposal{}, errors.New("gemini client is not configured")
        }
        if err := ctx.Err(); err != nil {
                return ports.AIDecisionProposal{}, err
        }
        text := strings.TrimSpace(input.Text)
        if text == "" {
                return ports.AIDecisionProposal{}, errors.New("AI input text is required")
        }
        if len([]rune(text)) > c.maxInputCharacters {
                return ports.AIDecisionProposal{}, fmt.Errorf("AI input text exceeds %d characters", c.maxInputCharacters)
        }

        userPrompt := buildUserPrompt(input)
        reqBody := map[string]any{
                "systemInstruction": map[string]any{
                        "parts": []map[string]any{{"text": c.systemPrompt}},
                },
                "contents": []map[string]any{
                        {"role": "user", "parts": []map[string]any{{"text": userPrompt}}},
                },
                "generationConfig": map[string]any{
                        "maxOutputTokens":  c.maxOutputTokens,
                        "responseMimeType": "application/json",
                },
        }
        encoded, err := json.Marshal(reqBody)
        if err != nil {
                return ports.AIDecisionProposal{}, fmt.Errorf("encode request: %w", err)
        }

        requestCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
        defer cancel()

        u := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.baseURL, url.PathEscape(c.model), url.QueryEscape(c.apiKey))
        req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, u, strings.NewReader(string(encoded)))
        if err != nil {
                return ports.AIDecisionProposal{}, fmt.Errorf("create request: %w", err)
        }
        req.Header.Set("Content-Type", "application/json")
        req.Header.Set("x-goog-api-key", c.apiKey)

        resp, err := c.httpClient.Do(req)
        if err != nil {
                return ports.AIDecisionProposal{}, fmt.Errorf("send request: %w", err)
        }
        defer resp.Body.Close()

        var gemResp struct {
                Candidates []struct {
                        Content struct {
                                Parts []struct {
                                        Text string `json:"text"`
                                } `json:"parts"`
                        } `json:"content"`
                } `json:"candidates"`
        }
        if err := json.NewDecoder(resp.Body).Decode(&gemResp); err != nil {
                return ports.AIDecisionProposal{}, fmt.Errorf("decode response: %w", err)
        }
        if len(gemResp.Candidates) == 0 || len(gemResp.Candidates[0].Content.Parts) == 0 {
                return ports.AIDecisionProposal{}, errors.New("Gemini response did not contain a candidate")
        }
        rawText := strings.TrimSpace(gemResp.Candidates[0].Content.Parts[0].Text)
        if rawText == "" {
                return ports.AIDecisionProposal{}, errors.New("Gemini response did not contain structured content")
        }

        var wire struct {
                IntentBase      string `json:"intent_base"`
                DomainContext   string `json:"domain_context"`
                RequestedAction string `json:"requested_action"`
                ResponseText    string `json:"response_text"`
                ConfidenceBand  string `json:"confidence_band"`
                PolicyDecision  string `json:"policy_decision"`
                PolicyVersion   string `json:"policy_version"`
                RequiresHuman   bool   `json:"requires_human"`
                SchemaVersion   int    `json:"schema_version"`
        }
        if err := json.Unmarshal([]byte(rawText), &wire); err != nil {
                return ports.AIDecisionProposal{}, fmt.Errorf("decode structured proposal: %w", err)
        }

        policyVersion := strings.TrimSpace(wire.PolicyVersion)
        if policyVersion == "" {
                policyVersion = strings.TrimSpace(input.PolicyVersion)
        }
        return ports.AIDecisionProposal{
                IntentBase:         strings.TrimSpace(wire.IntentBase),
                DomainContext:      strings.TrimSpace(wire.DomainContext),
                Entities:           []byte(`{}`),
                EvidenceReferences: []byte(`[]`),
                RequestedAction:    strings.TrimSpace(wire.RequestedAction),
                ResponseText:       strings.TrimSpace(wire.ResponseText),
                ConfidenceBand:     strings.TrimSpace(wire.ConfidenceBand),
                RequiresHuman:      wire.RequiresHuman,
                MissingInformation: []byte(`[]`),
                ReasonCodes:        []byte(`[]`),
                PolicyDecision:     strings.TrimSpace(wire.PolicyDecision),
                PolicyVersion:      policyVersion,
                ModelReference:     "gemini/" + c.model,
                SchemaVersion:      1,
        }, nil
}

// Context import needed for Decide.
var _ = context.Background

var _ ports.AIRuntime = (*Client)(nil)

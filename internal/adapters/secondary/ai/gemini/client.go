// Package gemini — Gemini HTTP Client (Config Holder + Prompt Builder).
//
// This file holds ONLY:
//   1. GeminiHTTPClient struct (config: API key, model, base URL, HTTP client, system prompt)
//   2. NewGeminiHTTPClient constructor
//   3. Getters (BaseURL, APIKey, Model) — used by Gemini capability adapters and bootstrap
//   4. buildUserPrompt — used by the customer-sales adapter to build Gemini input
//
// The old generic decision path and its legacy proposal parser were removed.
// Domain-specific Gemini adapters own their request/response contracts and
// use this client only for shared HTTP configuration.
//
// Per contract ④ §8:
//   Mujeeb System Contract → system_instruction
//   Mujeeb Input Context → input (contents)
//   Catalog boundary → Structured Output (responseSchema)
//   Mujeeb Output Contract → capability-specific proposal
//
// Per contract ④ §3 (No Execution): the Gemini adapter contains NO database
// imports (no pgx, no sql, no database). It does HTTP only.

package gemini

import (
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

// GeminiHTTPClientConfig contains only runtime configuration. API keys are never copied
// into a proposal, error, log, or domain record (per contract ⑧ §23).
type GeminiHTTPClientConfig struct {
	BaseURL            string
	APIKey             string
	Model              string
	HTTPClient         *http.Client
	RequestTimeout     time.Duration
	MaxOutputTokens    int
	MaxInputCharacters int
	SystemPrompt       string
}

// GeminiHTTPClient is the low-level Gemini HTTP client. Capability-specific adapters
// wrap it to implement explicit application ports.
//
// Per contract ④ §3 (No Execution): this client does HTTP only.
// No database imports, no external API calls beyond Gemini.
type GeminiHTTPClient struct {
	baseURL            string
	apiKey             string
	model              string
	httpClient         *http.Client
	requestTimeout     time.Duration
	maxOutputTokens    int
	maxInputCharacters int
	systemPrompt       string
}

// NewGeminiHTTPClient creates a Gemini HTTP client with the given config.
//
// Per contract ④ §2, the system prompt defaults to the versioned
// prompts.CustomerSalesSystemPrompt from the prompts package.
func NewGeminiHTTPClient(cfg GeminiHTTPClientConfig) (*GeminiHTTPClient, error) {
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
		requestTimeout = 60 * time.Second
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
	return &GeminiHTTPClient{
		baseURL:            baseURL,
		apiKey:             strings.TrimSpace(cfg.APIKey),
		model:              model,
		httpClient:         client,
		requestTimeout:     requestTimeout,
		maxOutputTokens:    maxOutputTokens,
		maxInputCharacters: maxInputCharacters,
		systemPrompt:       systemPrompt,
	}, nil
}

// BaseURL returns the configured Gemini API base URL.
func (c *GeminiHTTPClient) BaseURL() string { return c.baseURL }

// APIKey returns the configured Gemini API key.
func (c *GeminiHTTPClient) APIKey() string { return c.apiKey }

// Model returns the configured Gemini model name.
func (c *GeminiHTTPClient) Model() string { return c.model }

// SystemPrompt returns the configured system prompt.
func (c *GeminiHTTPClient) SystemPrompt() string { return c.systemPrompt }

// MaxInputCharacters returns the max input character limit.
func (c *GeminiHTTPClient) MaxInputCharacters() int { return c.maxInputCharacters }

// MaxOutputTokens returns the max output token limit.
func (c *GeminiHTTPClient) MaxOutputTokens() int { return c.maxOutputTokens }

// RequestTimeout returns the configured request timeout.
func (c *GeminiHTTPClient) RequestTimeout() time.Duration { return c.requestTimeout }

// HTTPClient returns the configured HTTP client shared by Gemini capability adapters.
func (c *GeminiHTTPClient) HTTPClient() *http.Client { return c.httpClient }

// buildUserPrompt encodes the AIContext + customer message into the
// customer-facing prompt text for Gemini. Used by GeminiCustomerSalesAdapter.
//
// Per contract ④ §3, the input includes: business_context,
// conversation_context, conversation_state, catalog_evidence, user_message.
func buildUserPrompt(input ports.CustomerSalesDecisionRequest) string {
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
	Business               ports.CustomerSalesContextBusiness          `json:"business"`
	Conversation           ports.CustomerSalesContextConversation      `json:"conversation"`
	Customer               promptCustomerContext            `json:"customer"`
	CatalogEvidence        []ports.CustomerSalesCatalogEvidence        `json:"catalog_evidence"`
	CatalogSummary         []ports.CustomerSalesCatalogSummaryEntry      `json:"catalog_summary,omitempty"`
	OfferEvidence          []ports.CustomerSalesOfferEvidence          `json:"offer_evidence"`
	VariantEvidence        []ports.CustomerSalesVariantEvidence        `json:"variant_evidence"`
	KnowledgeEvidence      []ports.CustomerSalesKnowledgeEvidence      `json:"knowledge_evidence"`
	BusinessPolicyEvidence []ports.CustomerSalesBusinessPolicyEvidence `json:"business_policy_evidence"`
	RecentMessages         []ports.CustomerSalesRecentMessageEvidence  `json:"recent_messages"`
	PolicyEvidence         ports.CustomerSalesPolicyEvidence           `json:"policy_evidence"`
	KnowledgeState         string                           `json:"knowledge_state"`
	ConversationState      *ports.ConversationStateRecord   `json:"conversation_state,omitempty"`
	// ConversationSummary is the running LLM-generated summary of older
	// conversation turns (everything before the sliding window of
	// recent_messages). Per ADR-039. Empty when the conversation is
	// short (less than SummaryInterval turns).
	ConversationSummary string `json:"conversation_summary,omitempty"`
	// CatalogNames per ADR-048 — category names only (no IDs, no counts).
	// Gemini uses this for hierarchical navigation: when customer asks
	// "what do you have?", Gemini lists these names as categories.
	CatalogNames []string  `json:"catalog_names,omitempty"`
	GeneratedAt  time.Time `json:"generated_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type promptCustomerContext struct {
	Reference        string `json:"reference"`
	LocalePreference string `json:"locale_preference"`
	Status           string `json:"status"`
}

func promptContextFrom(value *ports.CustomerSalesContext) promptContext {
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
		CatalogNames:           value.CatalogNames,
		ConversationSummary:    value.ConversationSummary,
		GeneratedAt:            value.GeneratedAt,
		ExpiresAt:              value.ExpiresAt,
	}
}

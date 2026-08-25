package openaicompatible

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
)

const (
	defaultMaxOutputTokens  = 700
	defaultMaxResponseBytes = 1 << 20
	proposalSchemaVersion   = 1
)

// Config contains only runtime configuration. API keys are never copied into a
// proposal, error, log, or domain record.
type Config struct {
	BaseURL            string
	APIKey             string
	Model              string
	HTTPClient         *http.Client
	RequestTimeout     time.Duration
	MaxOutputTokens    int
	MaxResponseBytes   int64
	MaxInputCharacters int
	OutputTokensField  string
	SystemPrompt       string
}

// Client is an OpenAI-compatible JSON/HTTP implementation of ports.AIRuntime.
// It asks the model for a structured proposal only; AutoReplyService remains
// responsible for validation, policy, persistence, and side effects.
type Client struct {
	baseURL            string
	apiKey             string
	model              string
	httpClient         *http.Client
	requestTimeout     time.Duration
	maxOutputTokens    int
	maxResponseBytes   int64
	maxInputCharacters int
	outputTokensField  string
	systemPrompt       string
}

func NewClient(cfg Config) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, errors.New("llm base URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, errors.New("llm base URL must be an absolute HTTP or HTTPS URL")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("llm API key is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("llm model is required")
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
		maxInputCharacters = 12000
	}
	outputTokensField := strings.TrimSpace(cfg.OutputTokensField)
	if outputTokensField == "" {
		outputTokensField = "max_completion_tokens"
	}
	if outputTokensField != "max_completion_tokens" && outputTokensField != "max_tokens" {
		return nil, errors.New("llm output token field must be max_completion_tokens or max_tokens")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	systemPrompt := strings.TrimSpace(cfg.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}
	return &Client{
		baseURL:            baseURL,
		apiKey:             strings.TrimSpace(cfg.APIKey),
		model:              strings.TrimSpace(cfg.Model),
		httpClient:         client,
		requestTimeout:     requestTimeout,
		maxOutputTokens:    maxOutputTokens,
		maxResponseBytes:   maxResponseBytes,
		maxInputCharacters: maxInputCharacters,
		outputTokensField:  outputTokensField,
		systemPrompt:       systemPrompt,
	}, nil
}

func (c *Client) Decide(ctx context.Context, input ports.AIDecisionInput) (ports.AIDecisionProposal, error) {
	if c == nil {
		return ports.AIDecisionProposal{}, errors.New("llm client is not configured")
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

	requestBody := chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: c.systemPrompt},
			{Role: "user", Content: buildUserPrompt(input)},
		},
		ResponseFormat: responseFormat{
			Type: "json_schema",
			JSONSchema: jsonSchemaDefinition{
				Name:   "mujeeb_ai_decision_proposal",
				Strict: true,
				Schema: proposalJSONSchema(),
			},
		},
	}
	if c.outputTokensField == "max_tokens" {
		requestBody.MaxTokens = c.maxOutputTokens
	} else {
		requestBody.MaxCompletionTokens = c.maxOutputTokens
	}
	encoded, err := json.Marshal(requestBody)
	if err != nil {
		return ports.AIDecisionProposal{}, fmt.Errorf("encode LLM request: %w", err)
	}

	requestCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.completionsURL(), bytes.NewReader(encoded))
	if err != nil {
		return ports.AIDecisionProposal{}, fmt.Errorf("create LLM request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if requestCtx.Err() != nil {
			return ports.AIDecisionProposal{}, requestCtx.Err()
		}
		return ports.AIDecisionProposal{}, fmt.Errorf("LLM request failed: %w", err)
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, c.maxResponseBytes+1)
	body, readErr := io.ReadAll(limited)
	if readErr != nil {
		return ports.AIDecisionProposal{}, fmt.Errorf("read LLM response: %w", readErr)
	}
	if int64(len(body)) > c.maxResponseBytes {
		return ports.AIDecisionProposal{}, errors.New("LLM response exceeds configured size limit")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return ports.AIDecisionProposal{}, fmt.Errorf("LLM request returned HTTP %d", resp.StatusCode)
	}
	var completion chatCompletionResponse
	if err := json.Unmarshal(body, &completion); err != nil {
		return ports.AIDecisionProposal{}, fmt.Errorf("decode LLM response: %w", err)
	}
	if len(completion.Choices) == 0 {
		return ports.AIDecisionProposal{}, errors.New("LLM response did not contain a choice")
	}
	if strings.TrimSpace(completion.Choices[0].Message.Content) == "" {
		return ports.AIDecisionProposal{}, fmt.Errorf("LLM response did not contain structured content (finish_reason=%s refusal=%t)", completion.Choices[0].FinishReason, strings.TrimSpace(completion.Choices[0].Message.Refusal) != "")
	}
	var wire proposalWire
	if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &wire); err != nil {
		return ports.AIDecisionProposal{}, fmt.Errorf("decode structured LLM proposal: %w", err)
	}
	proposal, err := wire.toProposal(input, c.model)
	if err != nil {
		return ports.AIDecisionProposal{}, err
	}
	return proposal, nil
}

func (c *Client) completionsURL() string {
	if strings.HasSuffix(c.baseURL, "/chat/completions") {
		return c.baseURL
	}
	return c.baseURL + "/chat/completions"
}

func buildUserPrompt(input ports.AIDecisionInput) string {
	prompt := fmt.Sprintf("Business ID: %s\nConversation ID: %s\nChannel: %s\nPolicy version: %s\nSource message reference: %s\nCustomer message:\n%s", input.BusinessID, input.ConversationID, input.Channel, input.PolicyVersion, input.SourceMessageReference, strings.TrimSpace(input.Text))
	if input.Context == nil {
		return prompt
	}
	encoded, err := json.Marshal(promptContextFrom(input.Context))
	if err != nil {
		return prompt + "\nVerified Mujeeb context: unavailable"
	}
	return prompt + "\nVerified Mujeeb context (evidence only; do not infer missing facts):\n" + string(encoded)
}

type promptContext struct {
	SchemaVersion   int                             `json:"schema_version"`
	Freshness       string                          `json:"freshness"`
	Business        ports.AIContextBusiness         `json:"business"`
	Conversation    ports.AIContextConversation     `json:"conversation"`
	Customer        promptCustomerContext           `json:"customer"`
	CatalogEvidence []ports.AICatalogEvidence       `json:"catalog_evidence"`
	OfferEvidence   []ports.AIOfferEvidence         `json:"offer_evidence"`
	VariantEvidence []ports.AIVariantEvidence       `json:"variant_evidence"`
	RecentMessages  []ports.AIRecentMessageEvidence `json:"recent_messages"`
	PolicyEvidence  ports.AIPolicyEvidence          `json:"policy_evidence"`
	KnowledgeState  string                          `json:"knowledge_state"`
	GeneratedAt     time.Time                       `json:"generated_at"`
	ExpiresAt       time.Time                       `json:"expires_at"`
}

type promptCustomerContext struct {
	Reference        string `json:"reference"`
	LocalePreference string `json:"locale_preference"`
	Status           string `json:"status"`
}

func promptContextFrom(value *ports.AIContext) promptContext {
	return promptContext{
		SchemaVersion:   value.SchemaVersion,
		Freshness:       value.Freshness,
		Business:        value.Business,
		Conversation:    value.Conversation,
		Customer:        promptCustomerContext{Reference: value.Customer.Reference, LocalePreference: value.Customer.LocalePreference, Status: value.Customer.Status},
		CatalogEvidence: value.CatalogEvidence,
		OfferEvidence:   value.OfferEvidence,
		VariantEvidence: value.VariantEvidence,
		RecentMessages:  value.RecentMessages,
		PolicyEvidence:  value.PolicyEvidence,
		KnowledgeState:  value.KnowledgeState,
		GeneratedAt:     value.GeneratedAt,
		ExpiresAt:       value.ExpiresAt,
	}
}

const defaultSystemPrompt = `أنت طبقة تحليل واقتراح فقط داخل Mujeeb 24. أخرج JSON المطابق للمخطط فقط، ولا تكتب أي شرح خارج JSON. لا تنفذ أدوات ولا تتصل بقاعدة بيانات أو Chatwoot أو SocialAPI. استخدم Verified Mujeeb context كمصدر الأدلة الوحيد. لا تخترع سعرًا أو توفرًا أو موعدًا أو سياسة. إذا كانت المعلومة غير موجودة أو stale أو missing فلا تقل إنها متاحة، واختر requested_action=ask_clarification أو requested_action=no_action. أي إجابة factual عن catalog أو offer أو availability يجب أن تستشهد بمراجع موجودة في evidence_references. لرسالة تحية أو طلب معلومات بسيط يمكن استخدام requested_action=answer مع response_text غير factual. استخدم requires_human=true عند الحاجة. لا تحفظ أو تُخرج chain-of-thought أو أسرارًا أو بيانات لا يحتاجها القرار. القيم المسموحة حرفيًا لـrequested_action هي answer أو ask_clarification أو no_action فقط. القيم المسموحة حرفيًا لـpolicy_decision هي allowed أو requires_approval أو denied فقط. confidence_band يجب أن تكون low أو medium أو high.`

type chatCompletionRequest struct {
	Model               string         `json:"model"`
	Messages            []chatMessage  `json:"messages"`
	ResponseFormat      responseFormat `json:"response_format"`
	MaxCompletionTokens int            `json:"max_completion_tokens,omitempty"`
	MaxTokens           int            `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type       string               `json:"type"`
	JSONSchema jsonSchemaDefinition `json:"json_schema"`
}

type jsonSchemaDefinition struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type chatCompletionResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content string `json:"content"`
			Refusal string `json:"refusal"`
		} `json:"message"`
	} `json:"choices"`
}

type proposalWire struct {
	IntentBase         string                     `json:"intent_base"`
	DomainContext      string                     `json:"domain_context"`
	Entities           map[string]json.RawMessage `json:"entities"`
	EvidenceReferences []string                   `json:"evidence_references"`
	RequestedAction    string                     `json:"requested_action"`
	ResponseText       string                     `json:"response_text"`
	ConfidenceValue    string                     `json:"confidence_value"`
	ConfidenceBand     string                     `json:"confidence_band"`
	RequiresHuman      bool                       `json:"requires_human"`
	MissingInformation []string                   `json:"missing_information"`
	ReasonCodes        []string                   `json:"reason_codes"`
	PolicyDecision     string                     `json:"policy_decision"`
	PolicyVersion      string                     `json:"policy_version"`
	KnowledgeVersion   string                     `json:"knowledge_version"`
	SchemaVersion      int                        `json:"schema_version"`
}

func (w proposalWire) toProposal(input ports.AIDecisionInput, model string) (ports.AIDecisionProposal, error) {
	if w.SchemaVersion != proposalSchemaVersion {
		return ports.AIDecisionProposal{}, fmt.Errorf("unsupported AI proposal schema version %d", w.SchemaVersion)
	}
	if strings.TrimSpace(w.IntentBase) == "" || strings.TrimSpace(w.RequestedAction) == "" || strings.TrimSpace(w.ConfidenceBand) == "" || strings.TrimSpace(w.PolicyDecision) == "" {
		return ports.AIDecisionProposal{}, errors.New("LLM structured proposal is missing required decision fields")
	}
	entities := []byte(`{}`)
	if w.Entities != nil {
		var err error
		entities, err = json.Marshal(w.Entities)
		if err != nil {
			return ports.AIDecisionProposal{}, fmt.Errorf("encode LLM entities: %w", err)
		}
	}
	evidence, err := json.Marshal(w.EvidenceReferences)
	if err != nil {
		return ports.AIDecisionProposal{}, fmt.Errorf("encode LLM evidence: %w", err)
	}
	missing, err := json.Marshal(w.MissingInformation)
	if err != nil {
		return ports.AIDecisionProposal{}, fmt.Errorf("encode LLM missing information: %w", err)
	}
	reasons, err := json.Marshal(w.ReasonCodes)
	if err != nil {
		return ports.AIDecisionProposal{}, fmt.Errorf("encode LLM reason codes: %w", err)
	}
	policyVersion := strings.TrimSpace(w.PolicyVersion)
	if policyVersion == "" {
		policyVersion = strings.TrimSpace(input.PolicyVersion)
	}
	return ports.AIDecisionProposal{
		IntentBase:         strings.TrimSpace(w.IntentBase),
		DomainContext:      strings.TrimSpace(w.DomainContext),
		Entities:           entities,
		EvidenceReferences: evidence,
		RequestedAction:    strings.TrimSpace(w.RequestedAction),
		ResponseText:       strings.TrimSpace(w.ResponseText),
		ConfidenceValue:    strings.TrimSpace(w.ConfidenceValue),
		ConfidenceBand:     strings.TrimSpace(w.ConfidenceBand),
		RequiresHuman:      w.RequiresHuman,
		MissingInformation: missing,
		ReasonCodes:        reasons,
		PolicyDecision:     strings.TrimSpace(w.PolicyDecision),
		PolicyVersion:      policyVersion,
		KnowledgeVersion:   strings.TrimSpace(w.KnowledgeVersion),
		ModelReference:     "openai-compatible/" + strings.TrimSpace(model),
		SchemaVersion:      w.SchemaVersion,
	}, nil
}

func proposalJSONSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"intent_base":    map[string]any{"type": "string"},
			"domain_context": map[string]any{"type": "string"},
			"entities": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"catalog_item_id":  map[string]any{"type": "string"},
					"variant_id":       map[string]any{"type": "string"},
					"quantity":         map[string]any{"type": "string"},
					"origin":           map[string]any{"type": "string"},
					"destination":      map[string]any{"type": "string"},
					"departure_date":   map[string]any{"type": "string"},
					"doctor_specialty": map[string]any{"type": "string"},
					"preferred_time":   map[string]any{"type": "string"},
					"location":         map[string]any{"type": "string"},
					"phone":            map[string]any{"type": "string"},
				},
				"required":             []string{"catalog_item_id", "variant_id", "quantity", "origin", "destination", "departure_date", "doctor_specialty", "preferred_time", "location", "phone"},
				"additionalProperties": false,
			},

			"evidence_references": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"requested_action":    map[string]any{"type": "string"},
			"response_text":       map[string]any{"type": "string"},
			"confidence_value":    map[string]any{"type": "string"},
			"confidence_band":     map[string]any{"type": "string"},
			"requires_human":      map[string]any{"type": "boolean"},
			"missing_information": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"reason_codes":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"policy_decision":     map[string]any{"type": "string"},
			"policy_version":      map[string]any{"type": "string"},
			"knowledge_version":   map[string]any{"type": "string"},
			"schema_version":      map[string]any{"type": "integer"},
		},
		"required":             []string{"intent_base", "domain_context", "entities", "evidence_references", "requested_action", "response_text", "confidence_value", "confidence_band", "requires_human", "missing_information", "reason_codes", "policy_decision", "policy_version", "knowledge_version", "schema_version"},
		"additionalProperties": false,
	}
}

var _ ports.AIRuntime = (*Client)(nil)

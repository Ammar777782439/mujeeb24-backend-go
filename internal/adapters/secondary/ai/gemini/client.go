package gemini

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
	defaultMaxOutputTokens  = 2048
	defaultMaxResponseBytes = 1 << 20
	proposalSchemaVersion   = 1
	defaultBaseURL          = "https://generativelanguage.googleapis.com"
	defaultModel            = "gemini-3.5-flash-lite"
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
	SystemPrompt       string
}

// Client is a Gemini JSON/HTTP implementation of ports.AIRuntime.
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
	systemPrompt       string
}

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
		model = "gemini-3.5-flash-lite"
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
		model:              model,
		httpClient:         client,
		requestTimeout:     requestTimeout,
		maxOutputTokens:    maxOutputTokens,
		maxResponseBytes:   maxResponseBytes,
		maxInputCharacters: maxInputCharacters,
		systemPrompt:       systemPrompt,
	}, nil
}

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
	contents := []geminiContent{
		{Role: "user", Parts: []geminiPart{{Text: userPrompt}}},
	}

	requestCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	maxToolTurns := 10
	for turn := 0; turn < maxToolTurns; turn++ {
		reqBody := geminiRequest{
			SystemInstruction: &geminiContent{
				Parts: []geminiPart{{Text: c.systemPrompt}},
			},
			Contents: contents,
			Tools:    defaultCatalogTools(),
			GenerationConfig: geminiGenerationConfig{
				MaxOutputTokens:  c.maxOutputTokens,
				ResponseMimeType: "application/json",
				ResponseSchema:   proposalJSONSchema(),
			},
		}

		gemResp, err := c.sendRequest(requestCtx, reqBody)
		if err != nil {
			return ports.AIDecisionProposal{}, err
		}
		if len(gemResp.Candidates) == 0 || len(gemResp.Candidates[0].Content.Parts) == 0 {
			return ports.AIDecisionProposal{}, errors.New("Gemini response did not contain a candidate")
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

		// If model requested a tool call, execute it locally and loop back with the result
		if functionCallPart != nil {
			toolResult := executeCatalogDiscovery(input)
			contents = append(contents, geminiContent{
				Role:  "model",
				Parts: candidate.Content.Parts,
			})
			contents = append(contents, geminiContent{
				Role: "function",
				Parts: []geminiPart{
					{
						FunctionResponse: &geminiFunctionResponse{
							Name:     functionCallPart.Name,
							Response: toolResult,
						},
					},
				},
			})
			continue
		}

		if textContent == "" {
			return ports.AIDecisionProposal{}, errors.New("Gemini response did not contain structured content")
		}

		var wire proposalWire
		if err := json.Unmarshal([]byte(textContent), &wire); err != nil {
			return ports.AIDecisionProposal{}, fmt.Errorf("decode structured Gemini proposal: %w", err)
		}
		proposal, err := wire.toProposal(input, c.model)
		if err != nil {
			return ports.AIDecisionProposal{}, err
		}
		return proposal, nil
	}

	return ports.AIDecisionProposal{}, errors.New("Gemini tool execution exceeded maximum turn limit")
}

func (c *Client) sendRequest(requestCtx context.Context, reqBody geminiRequest) (geminiResponse, error) {
	encoded, err := json.Marshal(reqBody)
	if err != nil {
		return geminiResponse{}, fmt.Errorf("encode Gemini request: %w", err)
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
			return geminiResponse{}, fmt.Errorf("create Gemini request: %w", err)
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
			return geminiResponse{}, errors.New("Gemini response exceeds configured size limit")
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
			return geminiResponse{}, fmt.Errorf("Gemini request returned HTTP %d: %s", resp.StatusCode, snippet)
		}

		lastErr = nil
		break
	}

	if lastErr != nil {
		if requestCtx.Err() != nil {
			return geminiResponse{}, requestCtx.Err()
		}
		return geminiResponse{}, fmt.Errorf("Gemini request failed after %d retries: %w", maxRetries, lastErr)
	}

	var gemResp geminiResponse
	if err := json.Unmarshal(body, &gemResp); err != nil {
		return geminiResponse{}, fmt.Errorf("decode Gemini response: %w", err)
	}
	return gemResp, nil
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
	SchemaVersion          int                              `json:"schema_version"`
	Freshness              string                           `json:"freshness"`
	Business               ports.AIContextBusiness          `json:"business"`
	Conversation           ports.AIContextConversation      `json:"conversation"`
	Customer               promptCustomerContext            `json:"customer"`
	CatalogEvidence        []ports.AICatalogEvidence        `json:"catalog_evidence"`
	OfferEvidence          []ports.AIOfferEvidence          `json:"offer_evidence"`
	VariantEvidence        []ports.AIVariantEvidence        `json:"variant_evidence"`
	KnowledgeEvidence      []ports.AIKnowledgeEvidence      `json:"knowledge_evidence"`
	BusinessPolicyEvidence []ports.AIBusinessPolicyEvidence `json:"business_policy_evidence"`
	RecentMessages         []ports.AIRecentMessageEvidence  `json:"recent_messages"`
	PolicyEvidence         ports.AIPolicyEvidence           `json:"policy_evidence"`
	KnowledgeState         string                           `json:"knowledge_state"`
	ConversationState      *ports.ConversationStateRecord   `json:"conversation_state,omitempty"`
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
		GeneratedAt:            value.GeneratedAt,
		ExpiresAt:              value.ExpiresAt,
	}
}

const defaultSystemPrompt = `أنت المساعد الذكي لخدمة العملاء. مهمتك فهم نية العميل بدقة، استكشاف كتالوج وبيانات التاجر عند الحاجة، وتقديم ردود عربية احترافية، دقيقة، ومنسقة تناسب تطبيقات المحادثة.

قواعد التشغيل العامة والتنسيق:
1. التنسيق وجودة النص العربي (RTL):
   - استخدم أسطر جديدة وفواصل واضحة (\n) بين الفقرات والنقاط لتسهيل القراءة على الهاتف.
   - عند المقارنة أو سرد المميزات والأسعار، استخدم النقاط المنظمة (•) أو الأرقام.
   - اكتب باللغة العربية الواضحة، وتجنب حشر الكلمات الإنجليزية بين أقواس داخل النص العربي لتفادي تشويه اتجاه النص.
   - ابدأ بترحيب لطيف واختم بسؤال تفاعلي لمساعدة العميل.
2. الالتزام بالحقائق والأدلة:
   - استخدم بيانات التاجر وسياق الكتالوج الموثق كمصدر وحيد للأدلة، ولا تخترع منتجات أو أسعاراً أو سياسات غير موجودة.
   - استشهد بالمراجع المناسبة في evidence_references.
   - عند البحث أو المقارنة، استكشف عناصر الكتالوج المتاحة حتى إشارة اكتمال الكتالوج.
   - إذا كان المنتج أو الخدمة غير متوفرة بعد استكشاف الكتالوج، وضح ذلك للعميل بلباقة واقترح البدائل المتاحة إن وجدت.
3. مخرجات القرار المنظم:
   - أخرج JSON المطابق للمخطط فقط دون أي نص خارجه.
   - القيم المسموحة لـ requested_action: answer أو ask_clarification أو no_action.
   - القيم المسموحة لـ policy_decision: allowed أو requires_approval أو denied.
   - confidence_band: low أو medium أو high.
4. تتبع حالة المحادثة (state_proposal):
   - إذا كانت الرسالة تشير إلى منتج/عرض محدد في الأدلة، أخرج kind=RESOLVED مع focus المناسب.
   - إذا كانت الرسالة تقارن بين خيارات، أخرج kind=RESOLVED مع comparison المناسب.
   - إذا كانت الرسالة تحتمل أكثر من خيار ولا يمكن الحسم، أخرج kind=AMBIGUOUS واطلب التوضيح.
   - إذا كانت الرسالة تحية أو موضوعاً عاماً جديداً، أخرج kind=NO_REFERENCE.`

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
	MaxOutputTokens  int            `json:"maxOutputTokens,omitempty"`
	ResponseMimeType string         `json:"responseMimeType,omitempty"`
	ResponseSchema   map[string]any `json:"responseSchema,omitempty"`
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
	StateProposal      *stateProposalWire         `json:"state_proposal,omitempty"`
}

type stateProposalWire struct {
	Focus         *ports.ConversationFocus      `json:"focus,omitempty"`
	Comparison    *ports.ConversationComparison `json:"comparison,omitempty"`
	Kind          string                        `json:"kind"`
	ReferenceText string                        `json:"reference_text,omitempty"`
	Alternatives  []ports.ConversationFocus     `json:"alternatives,omitempty"`
}

func (w proposalWire) toProposal(input ports.AIDecisionInput, model string) (ports.AIDecisionProposal, error) {
	if w.SchemaVersion != proposalSchemaVersion {
		return ports.AIDecisionProposal{}, fmt.Errorf("unsupported AI proposal schema version %d", w.SchemaVersion)
	}
	if strings.TrimSpace(w.IntentBase) == "" || strings.TrimSpace(w.RequestedAction) == "" || strings.TrimSpace(w.ConfidenceBand) == "" || strings.TrimSpace(w.PolicyDecision) == "" {
		return ports.AIDecisionProposal{}, errors.New("Gemini structured proposal is missing required decision fields")
	}
	entities := []byte(`{}`)
	if w.Entities != nil {
		var err error
		entities, err = json.Marshal(w.Entities)
		if err != nil {
			return ports.AIDecisionProposal{}, fmt.Errorf("encode Gemini entities: %w", err)
		}
	}
	evidence, err := json.Marshal(w.EvidenceReferences)
	if err != nil {
		return ports.AIDecisionProposal{}, fmt.Errorf("encode Gemini evidence: %w", err)
	}
	missing, err := json.Marshal(w.MissingInformation)
	if err != nil {
		return ports.AIDecisionProposal{}, fmt.Errorf("encode Gemini missing information: %w", err)
	}
	reasons, err := json.Marshal(w.ReasonCodes)
	if err != nil {
		return ports.AIDecisionProposal{}, fmt.Errorf("encode Gemini reason codes: %w", err)
	}
	policyVersion := strings.TrimSpace(w.PolicyVersion)
	if policyVersion == "" {
		policyVersion = strings.TrimSpace(input.PolicyVersion)
	}
	proposal := ports.AIDecisionProposal{
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
		ModelReference:     "gemini/" + strings.TrimSpace(model),
		SchemaVersion:      w.SchemaVersion,
	}
	if w.StateProposal != nil {
		kind := strings.ToUpper(strings.TrimSpace(w.StateProposal.Kind))
		if kind == "" {
			kind = "NO_REFERENCE"
		}
		if kind != "RESOLVED" && kind != "AMBIGUOUS" && kind != "NO_REFERENCE" {
			kind = "NO_REFERENCE"
		}
		proposal.StateProposal = &ports.AIStateProposal{
			Focus:         w.StateProposal.Focus,
			Comparison:    w.StateProposal.Comparison,
			Kind:          kind,
			ReferenceText: strings.TrimSpace(w.StateProposal.ReferenceText),
			Alternatives:  w.StateProposal.Alternatives,
		}
	}
	return proposal, nil
}

func defaultCatalogTools() []geminiTool {
	return []geminiTool{
		{
			FunctionDeclarations: []geminiFunctionDeclaration{
				{
					Name:        "catalog_discovery",
					Description: "Discover and retrieve the complete merchant catalog, including items, variants, offers, prices, and availability across any business sector (retail, clinic/services, tourism/trips, SaaS/packages, etc.). Traverses until all records are returned.",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"query": map[string]any{
								"type":        "string",
								"description": "Optional search term, category, or specific item/offer reference to inspect.",
							},
						},
					},
				},
			},
		},
	}
}

func executeCatalogDiscovery(input ports.AIDecisionInput) map[string]any {
	var items []map[string]any
	var offers []map[string]any
	var variants []map[string]any

	if input.Context != nil {
		for _, item := range input.Context.CatalogEvidence {
			var attrs map[string]any
			if len(item.Attributes) > 0 {
				_ = json.Unmarshal(item.Attributes, &attrs)
			}
			items = append(items, map[string]any{
				"reference":         item.Reference,
				"catalog_reference": item.CatalogReference,
				"item_type":         item.ItemType,
				"name":              item.Name,
				"status":            item.Status,
				"attributes":        attrs,
				"evidence_state":    item.EvidenceState,
			})
		}
		for _, offer := range input.Context.OfferEvidence {
			offers = append(offers, map[string]any{
				"reference":              offer.Reference,
				"catalog_item_reference": offer.CatalogItemReference,
				"variant_reference":      offer.VariantReference,
				"name":                   offer.Name,
				"pricing_mode":           offer.PricingMode,
				"amount":                 offer.Amount,
				"currency":               offer.Currency,
				"availability_state":     offer.AvailabilityState,
				"status":                 offer.Status,
				"evidence_state":         offer.EvidenceState,
			})
		}
		for _, variant := range input.Context.VariantEvidence {
			var attrs map[string]any
			if len(variant.Attributes) > 0 {
				_ = json.Unmarshal(variant.Attributes, &attrs)
			}
			variants = append(variants, map[string]any{
				"reference":              variant.Reference,
				"catalog_item_reference": variant.CatalogItemReference,
				"name":                   variant.Name,
				"status":                 variant.Status,
				"attributes":             attrs,
				"evidence_state":         variant.EvidenceState,
			})
		}
	}

	return map[string]any{
		"items":    items,
		"offers":   offers,
		"variants": variants,
		"pagination": map[string]any{
			"total_items": len(items),
			"has_more":    false,
			"reached_end": true,
		},
		"status_message": "ALL_CATALOG_RECORDS_RETRIEVED_NO_MORE_DATA",
	}
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
				"required": []string{"catalog_item_id", "variant_id", "quantity", "origin", "destination", "departure_date", "doctor_specialty", "preferred_time", "location", "phone"},
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
			"state_proposal": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"focus": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"type":       map[string]any{"type": "string"},
							"id":         map[string]any{"type": "string"},
							"catalog_id": map[string]any{"type": "string"},
							"item_id":    map[string]any{"type": "string"},
							"name":       map[string]any{"type": "string"},
						},
						"required": []string{"type", "id"},
					},
					"comparison": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"type": map[string]any{"type": "string"},
							"ids":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
						"required": []string{"type", "ids"},
					},
					"kind":           map[string]any{"type": "string"},
					"reference_text": map[string]any{"type": "string"},
					"alternatives": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"type": map[string]any{"type": "string"},
								"id":   map[string]any{"type": "string"},
							},
							"required": []string{"type", "id"},
						},
					},
				},
				"required": []string{"kind"},
			},
		},
		"required": []string{"intent_base", "domain_context", "entities", "evidence_references", "requested_action", "response_text", "confidence_value", "confidence_band", "requires_human", "missing_information", "reason_codes", "policy_decision", "policy_version", "knowledge_version", "schema_version"},
	}
}

var _ ports.AIRuntime = (*Client)(nil)

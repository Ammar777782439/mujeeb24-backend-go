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
	defaultSafetyTurnBudget = 25
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
	SafetyTurnBudget   int
	SystemPrompt       string
	Capabilities       ports.AICapabilityDispatcher
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
	safetyTurnBudget   int
	systemPrompt       string
	capabilities       ports.AICapabilityDispatcher
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
	safetyTurnBudget := cfg.SafetyTurnBudget
	if safetyTurnBudget <= 0 {
		safetyTurnBudget = defaultSafetyTurnBudget
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
		safetyTurnBudget:   safetyTurnBudget,
		systemPrompt:       systemPrompt,
		capabilities:       cfg.Capabilities,
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

	var tools []geminiTool
	if c.capabilities != nil {
		defs := c.capabilities.Definitions()
		if len(defs) > 0 {
			var funcDecls []geminiFunctionDeclaration
			for _, def := range defs {
				funcDecls = append(funcDecls, geminiFunctionDeclaration{
					Name:        def.Name,
					Description: def.Description,
					Parameters:  def.Parameters,
				})
			}
			tools = []geminiTool{
				{FunctionDeclarations: funcDecls},
			}
		}
	}

	execCtx := ports.AICapabilityExecutionContext{
		BusinessID:     input.BusinessID,
		ConversationID: input.ConversationID,
	}

	var (
		discoveredCatalog    []ports.AICatalogEvidence
		discoveredOffers     []ports.AIOfferEvidence
		discoveredVariants   []ports.AIVariantEvidence
		session              = ports.NewCatalogRetrievalSession()
		loopFinishedNormally bool
		finalProposal        ports.AIDecisionProposal
	)

	safetyBudget := c.safetyTurnBudget
	for turn := 0; turn < safetyBudget; turn++ {
		reqBody := geminiRequest{
			SystemInstruction: &geminiContent{
				Parts: []geminiPart{{Text: c.systemPrompt}},
			},
			Contents: contents,
			Tools:    tools,
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

		// If model requested a tool call, execute via application capability dispatcher
		if functionCallPart != nil {
			var toolResponse map[string]any
			if c.capabilities != nil {
				capResult, capErr := c.capabilities.Execute(requestCtx, execCtx, functionCallPart.Name, functionCallPart.Args)
				if capErr != nil {
					toolResponse = map[string]any{
						"error": capErr.Error(),
					}
				} else {
					if dataMap, ok := capResult.Data.(map[string]any); ok {
						toolResponse = dataMap
					} else {
						encoded, _ := json.Marshal(capResult.Data)
						var m map[string]any
						if err := json.Unmarshal(encoded, &m); err == nil {
							toolResponse = m
						} else {
							toolResponse = map[string]any{"result": capResult.Data}
						}
					}
					// Record the operation and cursor chain in the session
					streamKey := capResult.StreamKey
					if streamKey == "" {
						streamKey = functionCallPart.Name
					}
					session.RecordOperation(capResult.Operation, streamKey, capResult.HasMore, capResult.NextCursor)

					// Accumulate evidence for the proposal without mutating input.Context directly
					if len(capResult.CatalogEvidence) > 0 {
						discoveredCatalog = append(discoveredCatalog, capResult.CatalogEvidence...)
					}
					if len(capResult.OfferEvidence) > 0 {
						discoveredOffers = append(discoveredOffers, capResult.OfferEvidence...)
					}
					if len(capResult.VariantEvidence) > 0 {
						discoveredVariants = append(discoveredVariants, capResult.VariantEvidence...)
					}
				}
			} else {
				toolResponse = map[string]any{"error": fmt.Sprintf("capability %q not available", functionCallPart.Name)}
			}

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
							Response: toolResponse,
						},
					},
				},
			})
			continue
		}

		if textContent == "" {
			return ports.AIDecisionProposal{}, errors.New("Gemini response did not contain structured content")
		}

		// If model tries to return a proposal while a catalog stream has HasMore=true,
		// prompt the model to continue pagination if turn budget permits.
		if session.HasIncompleteStreams() {
			incomplete := session.IncompleteStreams()
			if turn < safetyBudget-1 && len(incomplete) > 0 {
				contents = append(contents, geminiContent{
					Role:  "model",
					Parts: candidate.Content.Parts,
				})
				contents = append(contents, geminiContent{
					Role: "user",
					Parts: []geminiPart{
						{
							Text: fmt.Sprintf("Notice: Catalog retrieval for stream %q is incomplete (has_more is true, next_cursor is %q). You must continue fetching remaining pages with catalog_data before finalizing your decision.", incomplete[0].StreamKey, incomplete[0].NextCursor),
						},
					},
				})
				continue
			}
		}

		var wire proposalWire
		if err := json.Unmarshal([]byte(textContent), &wire); err != nil {
			return ports.AIDecisionProposal{}, fmt.Errorf("decode structured Gemini proposal: %w", err)
		}
		proposal, err := wire.toProposal(input, c.model)
		if err != nil {
			return ports.AIDecisionProposal{}, err
		}

		proposal.DiscoveredCatalogEvidence = discoveredCatalog
		proposal.DiscoveredOfferEvidence = discoveredOffers
		proposal.DiscoveredVariantEvidence = discoveredVariants
		proposal.CatalogStreams = session.AllStreams()
		proposal.CatalogRetrievalState = session.State(false)
		proposal.CatalogIncomplete = session.HasIncompleteStreams()
		proposal.SafetyBudgetExhausted = false

		if proposal.CatalogIncomplete {
			proposal.RequiresHuman = true
			proposal.PolicyDecision = "requires_approval"
			proposal.ReasonCodes = appendJSONString(proposal.ReasonCodes, "catalog_retrieval_incomplete")
			proposal.MissingInformation = appendJSONString(proposal.MissingInformation, "catalog retrieval stream is incomplete; remaining records not exhausted")
		}

		finalProposal = proposal
		loopFinishedNormally = true
		break
	}

	if loopFinishedNormally {
		return finalProposal, nil
	}

	// Technical safety budget exhausted before model concluded.
	return ports.AIDecisionProposal{
		IntentBase:                "catalog_safety_budget_exhausted",
		DomainContext:             "catalog",
		RequestedAction:           "ask_clarification",
		RequiresHuman:             true,
		PolicyDecision:            "requires_approval",
		ReasonCodes:               []byte(`["safety_budget_exhausted","catalog_retrieval_incomplete"]`),
		MissingInformation:        []byte(`["catalog data retrieval halted by technical safety budget"]`),
		PolicyVersion:             input.PolicyVersion,
		ModelReference:            "gemini/" + strings.TrimSpace(c.model),
		SchemaVersion:             proposalSchemaVersion,
		DiscoveredCatalogEvidence: discoveredCatalog,
		DiscoveredOfferEvidence:   discoveredOffers,
		DiscoveredVariantEvidence: discoveredVariants,
		CatalogRetrievalState:     ports.CatalogRetrievalSafetyBudgetExhausted,
		CatalogStreams:            session.AllStreams(),
		CatalogIncomplete:         true,
		SafetyBudgetExhausted:     true,
	}, nil
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

const defaultSystemPrompt = `أنت المساعد الذكي لخدمة العملاء وإدارة الكتالوج. مهمتك فهم نية المستخدم بدقة، استكشاف كتالوج وبيانات التاجر عند الحاجة، تنفيذ عمليات إضافة وتعديل المنتجات المعتمدة، وتقديم ردود عربية احترافية، دقيقة، ومنسقة تناسب تطبيقات المحادثة.

قواعد التشغيل العامة والتنسيق:
1. التنسيق وجودة النص العربي (RTL):
   - استخدم أسطر جديدة وفواصل واضحة (\n) بين الفقرات والنقاط لتسهيل القراءة على الهاتف.
   - عند المقارنة أو سرد المميزات والأسعار، استخدم النقاط المنظمة (•) أو الأرقام.
   - اكتب باللغة العربية الواضحة، وتجنب حشر الكلمات الإنجليزية بين أقواس داخل النص العربي لتفادي تشويه اتجاه النص.
   - ابدأ بترحيب لطيف واختم بسؤال تفاعلي لمساعدة العميل.
2. الالتزام بالحقائق والأدلة:
   - استخدم بيانات التاجر وسياق الكتالوج الموثق كمصدر وحيد للأدلة، ولا تخترع منتجات أو أسعاراً أو سياسات غير موجودة.
   - استشهد بالمراجع المناسبة في evidence_references.
   - إذا كان المنتج أو الخدمة غير متوفرة، وضح ذلك للعميل بلباقة واقترح البدائل المتاحة إن وجدت.
3. مخرجات القرار المنظم:
   - أخرج JSON المطابق للمخطط فقط دون أي نص خارجه.
   - القيم المسموحة لـ requested_action: answer أو ask_clarification أو no_action.
   - القيم المسموحة لـ policy_decision: allowed أو requires_approval أو denied.
   - confidence_band: low أو medium أو high.
4. تتبع حالة المحادثة (state_proposal):
   - إذا كانت الرسالة تشير إلى منتج/عرض محدد في الأدلة، أخرج kind=RESOLVED مع focus المناسب.
   - إذا كانت الرسالة تقارن بين خيارات، أخرج kind=RESOLVED مع comparison المناسب.
   - إذا كانت الرسالة تحتمل أكثر من خيار ولا يمكن الحسم، أخرج kind=AMBIGUOUS واطلب التوضيح.
   - إذا كانت الرسالة تحية أو موضوعاً عاماً جديداً، أخرج kind=NO_REFERENCE.
5. إضافة وتأليف منتجات الكتالوج (Catalog Authoring):
   - عند طلب التاجر إضافة أو إنشاء منتج جديد، افهم البيانات المذكورة (الاسم، السعر، المقاسات، الخصائص).
   - إذا كانت هناك بيانات تجارية إلزامية ناقصة (مثل السعر أو العرض) ولم تذكر في المحادثة، اطلب فقط المعلومة الناقصة (requested_action: ask_clarification).
   - لا تطلب مجدداً أي معلومة ذكرها التاجر سابقاً في سياق المحادثة.
   - إذا توفرت المعلومات الكافية، نفذ أداة catalog_authoring مباشرة لإنشاء المنتج والعروض والمتغيرات، ثم أكد الإضافة للتاجر بوضوح (requested_action: answer).`

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

func proposalJSONSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"intent_base":    map[string]any{"type": "string"},
			"domain_context": map[string]any{"type": "string"},
			"entities": map[string]any{
				"type":        "object",
				"description": "Extracted business entities and parameters (e.g. item_name, item_id, color, storage, quantity, etc.)",
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

func appendJSONString(raw []byte, value string) []byte {
	var values []string
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &values)
	}
	for _, v := range values {
		if v == value {
			return raw
		}
	}
	values = append(values, value)
	encoded, err := json.Marshal(values)
	if err != nil {
		return raw
	}
	return encoded
}

var _ ports.AIRuntime = (*Client)(nil)

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
	defaultBaseURL          = "https://generativelanguage.googleapis.com"
	defaultModel            = "gemini-2.5-flash"
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

// Client is a Gemini JSON/HTTP implementation of ports.AIRuntime for customer support.
// It executes tool calls via AICapabilityDispatcher and returns natural language responses.
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
	userPart, _ := json.Marshal(geminiPart{Text: userPrompt})
	contents := []geminiContent{
		{Role: "user", Parts: []json.RawMessage{userPart}},
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

		loopFinishedNormally bool
		finalProposal        ports.AIDecisionProposal
	)

	safetyBudget := c.safetyTurnBudget
	for turn := 0; turn < safetyBudget; turn++ {
		systemPart, _ := json.Marshal(geminiPart{Text: c.systemPrompt})
		reqBody := geminiRequest{
			SystemInstruction: &geminiContent{
				Parts: []json.RawMessage{systemPart},
			},
			Contents: contents,
			Tools:    tools,
			GenerationConfig: geminiGenerationConfig{
				MaxOutputTokens:  c.maxOutputTokens,
				ResponseMimeType: "application/json",
				ResponseSchema: &geminiSchema{
					Type: "OBJECT",
					Properties: map[string]geminiSchema{
						"intent_base": {
							Type:        "STRING",
							Description: "The base intent of the user (e.g. information_request, comparison, etc.)",
						},
						"requested_action": {
							Type:        "STRING",
							Description: "The final action to take (must be 'answer' or 'handoff')",
						},
						"requires_human": {
							Type:        "BOOLEAN",
							Description: "True if the question is unrelated to the store or too complex to answer safely.",
						},
						"response_text": {
							Type:        "STRING",
							Description: "The final response to the customer. MUST be based STRICTLY on retrieved catalog evidence. Do not invent products or prices. For exact product requests, state the price if found. For comparisons, explicitly list the names and prices of compared items. For explorations, list actual available items. Do not use generic marketing filler. Do not repeat text. If no evidence is found, state that it is unavailable.",
						},
						"confidence_value": {
							Type:        "STRING",
							Description: "Confidence score between 0.0 and 1.0",
						},
						"confidence_band": {
							Type:        "STRING",
							Description: "Confidence band (high, medium, low)",
						},
						"policy_decision": {
							Type:        "STRING",
							Description: "Policy decision (allowed, denied, handoff)",
						},
					},
					Required: []string{"intent_base", "requested_action", "requires_human", "response_text", "confidence_value", "confidence_band", "policy_decision"},
				},
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

		for _, rawPart := range candidate.Content.Parts {
			var part geminiPart
			if err := json.Unmarshal(rawPart, &part); err == nil {
				if part.FunctionCall != nil && strings.TrimSpace(part.FunctionCall.Name) != "" {
					functionCallPart = part.FunctionCall
					break
				}
				if strings.TrimSpace(part.Text) != "" {
					textContent = strings.TrimSpace(part.Text)
				}
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

			// Append model's function call turn
			contents = append(contents, geminiContent{
				Role:  "model",
				Parts: candidate.Content.Parts,
			})
			funcRespPart, _ := json.Marshal(geminiPart{
				FunctionResponse: &geminiFunctionResponse{
					Name:     functionCallPart.Name,
					Response: toolResponse,
				},
			})
			// Append tool execution result using role "user" (Gemini REST API requires role 'user' for functionResponse)
			contents = append(contents, geminiContent{
				Role:  "user",
				Parts: []json.RawMessage{funcRespPart},
			})
			continue
		}

		if textContent == "" {
			return ports.AIDecisionProposal{}, errors.New("Gemini response did not contain text content")
		}

		policyVersion := strings.TrimSpace(input.PolicyVersion)
		if policyVersion == "" {
			policyVersion = "auto-reply-v1"
		}

		type llmResponse struct {
			IntentBase      string `json:"intent_base"`
			RequestedAction string `json:"requested_action"`
			RequiresHuman   bool   `json:"requires_human"`
			ResponseText    string `json:"response_text"`
			ConfidenceValue string `json:"confidence_value"`
			ConfidenceBand  string `json:"confidence_band"`
			PolicyDecision  string `json:"policy_decision"`
		}
		var llmResp llmResponse

		cleanText := textContent
		if strings.HasPrefix(cleanText, "```json") {
			cleanText = strings.TrimPrefix(cleanText, "```json")
			cleanText = strings.TrimSuffix(strings.TrimSpace(cleanText), "```")
		} else if strings.HasPrefix(cleanText, "```") {
			cleanText = strings.TrimPrefix(cleanText, "```")
			cleanText = strings.TrimSuffix(strings.TrimSpace(cleanText), "```")
		}

		if err := json.Unmarshal([]byte(cleanText), &llmResp); err != nil {
			llmResp.IntentBase = "unrecognized_intent"
			llmResp.RequestedAction = "ask_clarification"
			llmResp.RequiresHuman = true
			llmResp.ResponseText = "عذراً، لم أتمكن من معالجة طلبك بشكل صحيح. سأقوم بتحويلك لممثل خدمة العملاء لمساعدتك."
			llmResp.ConfidenceValue = "0.0"
			llmResp.ConfidenceBand = "low"
			llmResp.PolicyDecision = "requires_approval"
		} else {
			// Semantic validation of Enums (fallback to handoff if model hallucinates an invalid enum)
			if llmResp.RequestedAction != "answer" && llmResp.RequestedAction != "handoff" {
				llmResp.RequestedAction = "handoff"
				llmResp.RequiresHuman = true
			}
			if llmResp.PolicyDecision != "allowed" && llmResp.PolicyDecision != "denied" && llmResp.PolicyDecision != "handoff" {
				llmResp.PolicyDecision = "handoff"
				llmResp.RequiresHuman = true
			}

			// Validate response_text
			if llmResp.RequestedAction == "answer" && strings.TrimSpace(llmResp.ResponseText) == "" {
				llmResp.RequestedAction = "handoff"
				llmResp.RequiresHuman = true
				llmResp.ResponseText = "عذراً، أواجه صعوبة فنية. سيتم تحويلك إلى موظف خدمة العملاء."
			}

			// Validate output length to prevent catastrophic repetition hallucination
			if len(llmResp.ResponseText) > 2000 {
				// Prevent dumping massive hallucinated texts
				llmResp.RequestedAction = "handoff"
				llmResp.RequiresHuman = true
				llmResp.ResponseText = "عذراً، أواجه صعوبة فنية. سيتم تحويلك إلى موظف خدمة العملاء."
			}

			validBands := map[string]bool{"high": true, "medium": true, "low": true}
			if !validBands[llmResp.ConfidenceBand] {
				llmResp.ConfidenceBand = "low"
			}
		}

		finalProposal = ports.AIDecisionProposal{
			IntentBase:                llmResp.IntentBase,
			DomainContext:             "customer_support",
			Entities:                  []byte(`{}`),
			EvidenceReferences:        []byte(`[]`),
			RequestedAction:           llmResp.RequestedAction,
			ResponseText:              llmResp.ResponseText,
			ConfidenceValue:           llmResp.ConfidenceValue,
			ConfidenceBand:            llmResp.ConfidenceBand,
			RequiresHuman:             llmResp.RequiresHuman,
			MissingInformation:        []byte(`[]`),
			ReasonCodes:               []byte(`[]`),
			PolicyDecision:            llmResp.PolicyDecision,
			PolicyVersion:             policyVersion,
			ModelReference:            "gemini/" + strings.TrimSpace(c.model),
			SchemaVersion:             1,
			DiscoveredCatalogEvidence: discoveredCatalog,
			DiscoveredOfferEvidence:   discoveredOffers,
			DiscoveredVariantEvidence: discoveredVariants,
			CatalogRetrievalState:     ports.CatalogRetrievalNoneRequired,
			CatalogIncomplete:         false,
			SafetyBudgetExhausted:     false,
		}

		loopFinishedNormally = true
		break
	}

	if loopFinishedNormally {
		return finalProposal, nil
	}

	policyVersion := strings.TrimSpace(input.PolicyVersion)
	if policyVersion == "" {
		policyVersion = "auto-reply-v1"
	}

	// Technical safety budget exhausted before model concluded.
	return ports.AIDecisionProposal{
		IntentBase:                "safety_budget_exhausted",
		DomainContext:             "customer_support",
		Entities:                  []byte(`{}`),
		EvidenceReferences:        []byte(`[]`),
		RequestedAction:           "ask_clarification",
		ResponseText:              "يرجى توضيح استفسارك لنتمكن من خدمتك بشكل أفضل.",
		ConfidenceValue:           "0.50",
		ConfidenceBand:            "low",
		RequiresHuman:             true,
		PolicyDecision:            "requires_approval",
		ReasonCodes:               []byte(`["safety_budget_exhausted","catalog_retrieval_incomplete"]`),
		MissingInformation:        []byte(`["catalog data retrieval halted by technical safety budget"]`),
		PolicyVersion:             policyVersion,
		ModelReference:            "gemini/" + strings.TrimSpace(c.model),
		SchemaVersion:             1,
		DiscoveredCatalogEvidence: discoveredCatalog,
		DiscoveredOfferEvidence:   discoveredOffers,
		DiscoveredVariantEvidence: discoveredVariants,
		CatalogRetrievalState:     ports.CatalogRetrievalSafetyBudgetExhausted,
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

const defaultSystemPrompt = `أنت Customer AI (مساعد خدمة العملاء) ولست Merchant AI. مهمتك فهم استفسارات العميل والإجابة عليها بناءً على الأدلة (Catalog Evidence) التي تستخرجها عبر الأدوات (Function Calling).

القواعد الصارمة:
1. لا تخترع منتجاً، أو سعراً، أو توفراً، أو سياسة. أي معلومة تجارية يجب أن تكون مستندة حرفياً إلى الأدلة المستلمة.
2. إذا كان السؤال يتطلب مقارنة، يجب سرد المنتجات بوضوح مع أسعارها وخصائصها المستخرجة من الكتالوج. لا تستخدم كلاماً تسويقياً عاماً لملء الفراغ.
3. إذا طلب العميل استكشاف المنتجات، يجب سرد المنتجات الفعلية المتوفرة في الأدلة.
4. إذا سأل العميل عن منتج محدد والسعر موجود في الأدلة، يجب ذكر السعر صراحة.
5. إذا لم توجد أدلة كافية أو أرجعت الأداة has_more: true، قم باستدعاء الأداة مرة أخرى باستخدام next_cursor إذا كنت بحاجة للمزيد (مثلاً في المقارنات أو الاستكشاف).
6. توقف عن استدعاء الأدوات فور حصولك على الأدلة الكافية للإجابة ولا تستمر بلا حاجة.
7. إذا استنفدت الأدلة ولم تجد المنتج، صرّح بوضوح أنه غير متوفر. يُسمح بل يُفضل اقتراح أقرب المنتجات المتاحة من داخل الكتالوج فقط (مثل اقتراح الإصدار الأقدم).
8. لا تمارس الحشو التسويقي الزائد. لا تكرر النصوص أو الجمل أبداً.
9. اعتمد فقط على الأدلة ولا تستخدم معرفتك العامة للإجابة عن توفر المنتجات.

عند الانتهاء من البحث واتخاذ القرار، يجب أن يكون ردك النهائي عبارة عن كائن JSON فقط (بدون أي نصوص إضافية أو علامات Markdown) يحتوي على الحقول التالية:
{
  "intent_base": "information_request",
  "requested_action": "answer",
  "requires_human": false,
  "response_text": "نص الرد النهائي الذي سيتم إرساله للعميل...",
  "confidence_value": "0.95",
  "confidence_band": "high",
  "policy_decision": "allowed"
}`

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
	Role  string            `json:"role,omitempty"`
	Parts []json.RawMessage `json:"parts"`
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

type geminiSchema struct {
	Type        string                  `json:"type"`
	Description string                  `json:"description,omitempty"`
	Properties  map[string]geminiSchema `json:"properties,omitempty"`
	Required    []string                `json:"required,omitempty"`
	Enum        []string                `json:"enum,omitempty"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens  int           `json:"maxOutputTokens,omitempty"`
	ResponseMimeType string        `json:"responseMimeType,omitempty"`
	ResponseSchema   *geminiSchema `json:"responseSchema,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []json.RawMessage `json:"parts"`
			Role  string            `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
}

var _ ports.AIRuntime = (*Client)(nil)

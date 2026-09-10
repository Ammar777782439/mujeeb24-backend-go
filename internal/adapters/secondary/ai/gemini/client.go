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
	// Gemini uses systemInstruction + contents
	reqBody := geminiRequest{
		SystemInstruction: geminiContent{
			Parts: []geminiPart{{Text: c.systemPrompt}},
		},
		Contents: []geminiContent{
			{Role: "user", Parts: []geminiPart{{Text: userPrompt}}},
		},
		GenerationConfig: geminiGenerationConfig{
			MaxOutputTokens:  c.maxOutputTokens,
			ResponseMimeType: "application/json",
			ResponseSchema:   proposalJSONSchema(),
		},
	}
	encoded, err := json.Marshal(reqBody)
	if err != nil {
		return ports.AIDecisionProposal{}, fmt.Errorf("encode Gemini request: %w", err)
	}

	requestCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()
	// Support both API key (?key= & x-goog-api-key) and OAuth access token (Bearer ya29.).
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
				return ports.AIDecisionProposal{}, requestCtx.Err()
			case <-time.After(delay):
			}
		}

		req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, u, bytes.NewReader(encoded))
		if err != nil {
			return ports.AIDecisionProposal{}, fmt.Errorf("create Gemini request: %w", err)
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
			return ports.AIDecisionProposal{}, errors.New("Gemini response exceeds configured size limit")
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
			return ports.AIDecisionProposal{}, fmt.Errorf("Gemini request returned HTTP %d: %s", resp.StatusCode, snippet)
		}

		lastErr = nil
		break
	}

	if lastErr != nil {
		if requestCtx.Err() != nil {
			return ports.AIDecisionProposal{}, requestCtx.Err()
		}
		return ports.AIDecisionProposal{}, fmt.Errorf("Gemini request failed after %d retries: %w", maxRetries, lastErr)
	}
	var gemResp geminiResponse
	if err := json.Unmarshal(body, &gemResp); err != nil {
		return ports.AIDecisionProposal{}, fmt.Errorf("decode Gemini response: %w", err)
	}
	if len(gemResp.Candidates) == 0 || len(gemResp.Candidates[0].Content.Parts) == 0 {
		return ports.AIDecisionProposal{}, errors.New("Gemini response did not contain a candidate")
	}
	textContent := strings.TrimSpace(gemResp.Candidates[0].Content.Parts[0].Text)
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

const defaultSystemPrompt = `أنت المساعد الذكي وممثل خدمة العملاء الرسمي لمنصة Mujeeb 24.
مهمتك تحليل رسائل العملاء، فهم نيتهم بدقة، وتقديم ردود عربية احترافية، منسقة، وجذابة تناسب تطبيقات المحادثة (Facebook Messenger, WhatsApp, Instagram).

قواعد التنسيق وجودة النص العربي (مهمة جداً):
1. التنسيق والترتيب البصري:
   - استخدم أسطر جديدة وفواصل واضحة (\n) بين الفقرات والنقاط بدلاً من كتابة فقرة واحدة مكدسة.
   - عند المقارنة أو سرد المميزات، استخدم النقاط المنظمة (•) أو الأرقام لتسهيل القراءة على الهاتف.
   - ابدأ بترحيب لطيف ومباشر، واختم بسؤال تفاعلي لمساعدة العميل (مثال: "هل تحب نوضح لك أي تفاصيل أخرى؟").
2. نقاء اللغة وتجنب تشويه النص (BiDi & RTL):
   - اكتب باللغة العربية الفصحى الواضحة والجميلة.
   - ممنوع حشر المصطلحات والأسماء الإنجليزية بين أقواس داخل الجمل العربية (مثل: تجنب وضع English words بين أقواس كـ (Unified Inbox) أو (Human Handoff) لأنها تشوه اتجاه النص وتجعله غير مفهوم). استخدم التعبير العربي الواضح فقط (مثل: صندوق الوارد الموحد، التحويل للموظف البشري، إدارة المبيعات والفرص).
3. فهم النية والدقة:
   - إذا سأل العميل عن "مقارنة" أو "الفرق": قارن بين الباقات بنقاط مرتبة توضح ميزة وسعر كل باقة.
   - إذا سأل عن "معلومات/مميزات": اشرح المزايا التشغيلية والقيمة للنشاط.
    - إذا سأل عن "الأسعار": اذكر السعر وطريقة الدفع بوضوح.
    - إذا طلب العميل الاشتراك أو التفعيل أو الشراء: intent_base=subscription_request مع requires_human=true (سيتولى النظام إرسال رسالة تأكيد ثابتة وتحويل المحادثة للموظف البشري).
4. الالتزام بالحقائق والمخرجات:
    - استخدم Verified Mujeeb context كمصدر الأدلة الوحيد ولا تخترع حقائق غير موجودة.
    - استشهد بالمراجع المناسبة في evidence_references.
    - أخرج JSON المطابق للمخطط فقط دون أي كلام خارجه.
    - القيم المسموحة لـ requested_action: answer أو ask_clarification أو no_action.
    - القيم المسموحة لـ policy_decision: allowed أو requires_approval أو denied.
    - confidence_band: low أو medium أو high.
5. تتبع مرجع المحادثة (state_proposal) — إلزامي في كل رد:
    - اقرأ conversation_state (إن وجد) وrecent_messages لفهم ما يتحدث عنه العميل الآن.
    - إذا كانت الرسالة تشير بوضوح إلى entity واحد موجود في catalog_evidence أو offer_evidence (بالاسم أو الضمير أو الإشارة أو سؤال متابعة ناقص)، أخرج state_proposal.kind=RESOLVED مع focus={type,id} باستخدام نفس type وid الظاهرين في الـevidence (type يكون catalog أو item أو offer أو variant، وid هو نفس Reference).
    - إذا كانت الرسالة تقارن entity اثنين أو أكثر، أخرج kind=RESOLVED مع comparison={type,id قائمة المراجع} وfocus لأبرز entity عند الحاجة.
    - اختر دائماً أدق مستوى ممكن: إذا كان السؤال عن عرض/سعر/مدة/توفر محدد استخدم type=offer مع id يساوي Offer Reference الظاهر في offer_evidence. إذا كان عن منتج/خدمة عامة استخدم type=item مع CatalogEvidence Reference. لا تستخدم type=catalog إلا إذا كان السؤال عن الكتالوج كله (مثل "ايش عندكم؟").
    - إذا كانت الرسالة تحتمل أكثر من entity ولا يمكن الحسم من السياق، أخرج kind=AMBIGUOUS مع alternatives (قائمة المرشحين) واطلب clarification عبر requested_action=ask_clarification.
    - إذا كانت الرسالة مستقلة تماماً (تحية أو موضوع جديد بلا مرجع)، أخرج kind=NO_REFERENCE ولا تمسح أي شيء.
    - لا تخترع id غير موجود في الـevidence. لا تستخدم confidence كقرار أمان.
    - ترتيب الـevidence مقصود: conversation_state.focus هو المرجع الحالي المُتحقق، وأول عناصر catalog_evidence/offer_evidence هي أدلة هذا المرجع. العناصر التالية مرشحات بديلة فقط. أجب من أدلة الـfocus إلا إذا أشارت الرسالة الحالية بوضوح إلى بديل، وعندها اقترحه في state_proposal مع evidence_references الخاصة به فقط.`

type geminiRequest struct {
	SystemInstruction geminiContent          `json:"systemInstruction"`
	Contents          []geminiContent        `json:"contents"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens  int            `json:"maxOutputTokens,omitempty"`
	ResponseMimeType string         `json:"responseMimeType,omitempty"`
	ResponseSchema   map[string]any `json:"responseSchema,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
			Role string `json:"role"`
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

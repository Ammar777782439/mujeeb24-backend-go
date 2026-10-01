package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/prompts"
	"github.com/google/uuid"
)

// MerchantCatalogRuntime is the isolated B2B Gemini runtime.
// It uses Google's current Interactions API, not the legacy generateContent
// request/response shape. Mujeeb remains responsible for tool execution,
// validation, authorization, and persistence.
//
// The B2B runtime uses client-side function calling. Gemini requires the
// interaction to be stored so the tool result can be submitted through
// previous_interaction_id. Therefore Store must remain true for this loop.
type MerchantCatalogRuntime struct {
	client         *Client
	httpClient     *http.Client
	apiKey         string
	model          string
	baseURL        string
	timeout        time.Duration
	maxOutput      int
	maxInput       int
	runRepo        ports.AIRunRepository
	lifecycle      ports.AIRunLifecyclePort
	newID          func() string
	configProvider ports.AIConfigurationProvider
}

func NewMerchantCatalogRuntime(client *Client) (*MerchantCatalogRuntime, error) {
	if client == nil {
		return nil, errors.New("gemini client is required")
	}
	httpClient := client.HTTPClient()
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &MerchantCatalogRuntime{
		client:     client,
		httpClient: httpClient,
		apiKey:     client.APIKey(),
		model:      client.Model(),
		baseURL:    client.BaseURL(),
		timeout:    client.RequestTimeout(),
		maxOutput:  client.MaxOutputTokens(),
		maxInput:   client.MaxInputCharacters(),
		newID:      uuid.NewString,
	}, nil
}

func (r *MerchantCatalogRuntime) SetRunRepository(repo ports.AIRunRepository) { r.runRepo = repo }
func (r *MerchantCatalogRuntime) SetLifecycle(lifecycle ports.AIRunLifecyclePort) { r.lifecycle = lifecycle }
func (r *MerchantCatalogRuntime) SetConfigurationProvider(provider ports.AIConfigurationProvider) {
	r.configProvider = provider
}
func (r *MerchantCatalogRuntime) SetNewID(newID func() string) {
	if newID != nil {
		r.newID = newID
	}
}

type merchantCatalogInteractionRequest struct {
	Model                 string                              `json:"model"`
	Store                 bool                                `json:"store"`
	Input                 any                                 `json:"input"`
	SystemInstruction     string                              `json:"system_instruction,omitempty"`
	Tools                 []merchantCatalogInteractionTool    `json:"tools,omitempty"`
	ResponseFormat        *merchantCatalogInteractionFormat  `json:"response_format,omitempty"`
	GenerationConfig      *merchantCatalogGenerationConfig   `json:"generation_config,omitempty"`
	PreviousInteractionID string                              `json:"previous_interaction_id,omitempty"`
}

type merchantCatalogInteractionTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type merchantCatalogInteractionFormat struct {
	Type     string         `json:"type"`
	MimeType string         `json:"mime_type"`
	Schema   map[string]any `json:"schema"`
}

type merchantCatalogGenerationConfig struct {
	MaxOutputTokens int    `json:"max_output_tokens,omitempty"`
	ThinkingLevel   string `json:"thinking_level,omitempty"`
}

type merchantCatalogInteractionResponse struct {
	ID             string                         `json:"id"`
	Status         string                         `json:"status"`
	OutputText     string                         `json:"output_text,omitempty"`
	Steps          []merchantCatalogInteractionStep `json:"steps,omitempty"`
	Usage          merchantCatalogUsage            `json:"usage,omitempty"`
}

type merchantCatalogInteractionStep struct {
	Type       string          `json:"type"`
	ID         string          `json:"id,omitempty"`
	Name       string          `json:"name,omitempty"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
	Content    []merchantCatalogOutputPart `json:"content,omitempty"`
}

type merchantCatalogOutputPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type merchantCatalogUsage struct {
	InputTokens  int `json:"total_input_tokens,omitempty"`
	OutputTokens int `json:"total_output_tokens,omitempty"`
}

type merchantCatalogFunctionCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

func (r *MerchantCatalogRuntime) Decide(ctx context.Context, input merchantcatalogai.RuntimeInput) (merchantcatalogai.Proposal, error) {
	if strings.TrimSpace(input.Message) == "" {
		return merchantcatalogai.Proposal{}, errors.New("merchant message is required")
	}

	apiKey, model, baseURL := r.apiKey, r.model, r.baseURL
	maxOutput, maxInput := r.maxOutput, r.maxInput
	if r.configProvider != nil {
		cfg, err := r.configProvider.GetActiveConfig(ctx)
		if err != nil {
			return merchantcatalogai.Proposal{}, fmt.Errorf("resolve active AI configuration: %w", err)
		}
		apiKey, model, baseURL = cfg.APIKey, cfg.Model, cfg.BaseURL
		maxOutput, maxInput = cfg.MaxOutputTokens, cfg.MaxInputCharacters
	}
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(model) == "" || strings.TrimSpace(baseURL) == "" {
		return merchantcatalogai.Proposal{}, errors.New("merchant catalog AI Gemini configuration is incomplete")
	}
	if maxInput > 0 && len([]rune(input.Message)) > maxInput {
		return merchantcatalogai.Proposal{}, fmt.Errorf("merchant message exceeds %d characters", maxInput)
	}

	contextPayload := map[string]any{
		"business_id":      input.BusinessID,
		"principal_id":     input.PrincipalID,
		"session_id":       input.SessionID,
		"default_currency": input.DefaultCurrency,
		"catalog": map[string]any{
			"id":     input.SelectedCatalog.ID,
			"name":   input.SelectedCatalog.Name,
			"status": input.SelectedCatalog.Status,
		},
		"conversation_history": input.History,
		"merchant_message":     input.Message,
	}
	encodedContext, err := json.Marshal(contextPayload)
	if err != nil {
		return merchantcatalogai.Proposal{}, fmt.Errorf("encode merchant catalog AI context: %w", err)
	}

	systemText := prompts.MerchantCatalogAIV2SystemPrompt
	if len(input.EntityContract) > 0 {
		systemText += "\n\n# Catalog Entity Contract\n" + string(input.EntityContract)
	}

	tools := merchantCatalogInteractionTools(input.Capabilities)
	req := merchantCatalogInteractionRequest{
		Model:             model,
		Store:             true,
		Input:             string(encodedContext),
		SystemInstruction: systemText,
		Tools:             tools,
		ResponseFormat: &merchantCatalogInteractionFormat{
			Type:     "text",
			MimeType: "application/json",
			Schema:   merchantCatalogProposalSchema(),
		},
		GenerationConfig: &merchantCatalogGenerationConfig{
			MaxOutputTokens: maxOutput,
			ThinkingLevel:   merchantCatalogThinkingLevel(model),
		},
	}

	log.Printf("[MerchantCatalogAI] START business=%s session=%s catalog=%s model=%s",
		input.BusinessID, input.SessionID, input.SelectedCatalog.ID, model)

	var previousInteractionID string
	for {
		if err := ctx.Err(); err != nil {
			return merchantcatalogai.Proposal{}, fmt.Errorf("merchant catalog AI cancelled: %w", err)
		}
		req.PreviousInteractionID = previousInteractionID

		resp, err := r.sendInteraction(ctx, req, apiKey, baseURL)
		if err != nil {
			log.Printf("[MerchantCatalogAI] GEMINI_ERROR business=%s session=%s err=%v", input.BusinessID, input.SessionID, err)
			return merchantcatalogai.Proposal{}, err
		}

		log.Printf("[MerchantCatalogAI] INTERACTION business=%s session=%s id=%s status=%s steps=%d input_tokens=%d output_tokens=%d",
			input.BusinessID, input.SessionID, resp.ID, resp.Status, len(resp.Steps), resp.Usage.InputTokens, resp.Usage.OutputTokens)
		log.Printf("[MerchantCatalogAI][INTERACTION_DETAIL] business=%s session=%s step_types=%v output_chars=%d",
			input.BusinessID, input.SessionID, merchantCatalogStepTypes(resp), len([]rune(strings.TrimSpace(resp.OutputText))))

		calls := extractMerchantCatalogFunctionCalls(resp)
		if len(calls) > 0 {
			log.Printf("[MerchantCatalogAI][TOOL_BATCH] business=%s session=%s count=%d tools=%v",
				input.BusinessID, input.SessionID, len(calls), merchantCatalogCallNames(calls))
		}
		if len(calls) == 0 {
			if strings.EqualFold(strings.TrimSpace(resp.Status), "incomplete") {
				return merchantcatalogai.Proposal{}, incompleteMerchantCatalogInteractionError(resp, maxOutput)
			}
			proposal, err := parseMerchantCatalogProposal(resp)
			if err != nil {
				return merchantcatalogai.Proposal{}, err
			}
			proposal = proposal.Normalize()
			if validator, ok := input.Capabilities.(interface {
				ValidateProposalReferences(merchantcatalogai.Proposal) error
			}); ok {
				if err := validator.ValidateProposalReferences(proposal); err != nil {
					return merchantcatalogai.Proposal{}, fmt.Errorf("validate merchant catalog proposal references: %w", err)
				}
			}
			if err := proposal.Validate(); err != nil {
				return merchantcatalogai.Proposal{}, fmt.Errorf("validate merchant catalog proposal: %w", err)
			}
			log.Printf("[MerchantCatalogAI] PROPOSAL business=%s session=%s status=%s operation=%s evidence=%d",
				input.BusinessID, input.SessionID, proposal.Status, proposal.Operation, len(proposal.EvidenceReferences))
			logMerchantCatalogProposalDetail(input, proposal)
			return proposal, nil
		}

		if input.Capabilities == nil {
			return merchantcatalogai.Proposal{}, errors.New("Gemini requested a tool but B2B read capabilities are not configured")
		}
		if strings.TrimSpace(resp.ID) == "" {
			return merchantcatalogai.Proposal{}, errors.New("Gemini returned function calls without an interaction id")
		}

		resultInput, err := r.executeInteractionTools(ctx, resp, calls, input)
		if err != nil {
			return merchantcatalogai.Proposal{}, err
		}
		previousInteractionID = resp.ID
		req.Input = resultInput
	}
}

func merchantCatalogStepTypes(resp merchantCatalogInteractionResponse) []string {
	types := make([]string, 0, len(resp.Steps))
	for _, step := range resp.Steps {
		types = append(types, step.Type)
	}
	return types
}

func merchantCatalogCallNames(calls []merchantCatalogFunctionCall) []string {
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.Name)
	}
	return names
}

func derefString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func logMerchantCatalogProposalDetail(input merchantcatalogai.RuntimeInput, proposal merchantcatalogai.Proposal) {
	detail := map[string]any{
		"business": input.BusinessID,
		"session": input.SessionID,
		"status": proposal.Status,
		"operation": proposal.Operation,
		"schema_version": proposal.SchemaVersion,
		"evidence_count": len(proposal.EvidenceReferences),
		"missing_count": len(proposal.MissingInformation),
	}

	if proposal.Create != nil {
		detail["create"] = map[string]any{
			"name": proposal.Create.Name,
			"item_type": proposal.Create.ItemType,
			"pricing_mode": proposal.Create.PricingMode,
			"availability": proposal.Create.AvailabilityMode,
			"fulfillment": proposal.Create.FulfillmentMode,
			"variant_count": len(proposal.Create.Variants),
			"offer_count": len(proposal.Create.Offers),
			"variants": func() []map[string]string {
				out := make([]map[string]string, 0, len(proposal.Create.Variants))
				for _, v := range proposal.Create.Variants {
					out = append(out, map[string]string{"ref": v.Ref, "name": v.Name})
				}
				return out
			}(),
			"offers": func() []map[string]any {
				out := make([]map[string]any, 0, len(proposal.Create.Offers))
				for _, o := range proposal.Create.Offers {
					out = append(out, map[string]any{
						"variant_ref": derefString(o.VariantRef),
						"name": o.Name,
						"name_source": o.NameSource,
						"pricing_mode": o.PricingMode,
						"amount": derefString(o.Amount),
						"price_source": o.PriceSource,
						"currency": derefString(o.Currency),
						"availability_status": o.AvailabilityStatus,
						"fulfillment_mode": o.FulfillmentMode,
					})
				}
				return out
			}(),
		}
	}

	if proposal.Update != nil {
		detail["update"] = map[string]any{
			"item_id": proposal.Update.ItemID,
			"existing_variants": len(proposal.Update.ExistingVariants),
			"new_variants": len(proposal.Update.NewVariants),
			"existing_offers": len(proposal.Update.ExistingOffers),
			"new_offers": len(proposal.Update.NewOffers),
		}
	}

	log.Printf("[MerchantCatalogAI][PROPOSAL_DETAIL] %+v", detail)
}

func (r *MerchantCatalogRuntime) sendInteraction(ctx context.Context, reqBody merchantCatalogInteractionRequest, apiKey, baseURL string) (merchantCatalogInteractionResponse, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return merchantCatalogInteractionResponse{}, fmt.Errorf("encode Gemini interaction request: %w", err)
	}

	requestCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	url := fmt.Sprintf("%s/v1beta/interactions", strings.TrimRight(baseURL, "/"))

	const maxAttempts = 4
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return merchantCatalogInteractionResponse{}, fmt.Errorf("build Gemini interaction request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-goog-api-key", apiKey)

		resp, err := r.httpClient.Do(req)
		if err != nil {
			return merchantCatalogInteractionResponse{}, fmt.Errorf("merchant catalog Gemini interaction: %w", err)
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			return merchantCatalogInteractionResponse{}, fmt.Errorf("read Gemini interaction response: %w", readErr)
		}

		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("merchant catalog Gemini HTTP %d: %s", resp.StatusCode, string(body))
			if !isRetryableGeminiStatus(resp.StatusCode) || attempt == maxAttempts {
				return merchantCatalogInteractionResponse{}, lastErr
			}

			delay := time.Duration(1<<(attempt-1)) * time.Second
			log.Printf("[MerchantCatalogAI] GEMINI_RETRY status=%d attempt=%d/%d delay=%s", resp.StatusCode, attempt, maxAttempts, delay)
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return merchantCatalogInteractionResponse{}, fmt.Errorf("merchant catalog Gemini retry cancelled: %w", ctx.Err())
			case <-timer.C:
			}
			continue
		}

		var out merchantCatalogInteractionResponse
		if err := json.Unmarshal(body, &out); err != nil {
			return merchantCatalogInteractionResponse{}, fmt.Errorf("decode Gemini interaction response: %w", err)
		}
		return out, nil
	}
	return merchantCatalogInteractionResponse{}, lastErr
}

func isRetryableGeminiStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func merchantCatalogInteractionTools(caps ports.AICapabilityDispatcher) []merchantCatalogInteractionTool {
	if caps == nil {
		return nil
	}
	defs := caps.Definitions()
	out := make([]merchantCatalogInteractionTool, 0, len(defs))
	for _, definition := range defs {
		out = append(out, merchantCatalogInteractionTool{
			Type:        "function",
			Name:        definition.Name,
			Description: definition.Description,
			Parameters:  definition.Parameters,
		})
	}
	return out
}

func extractMerchantCatalogFunctionCalls(resp merchantCatalogInteractionResponse) []merchantCatalogFunctionCall {
	var calls []merchantCatalogFunctionCall
	for _, step := range resp.Steps {
		if step.Type != "function_call" {
			continue
		}
		calls = append(calls, merchantCatalogFunctionCall{
			ID:        step.ID,
			Name:      step.Name,
			Arguments: append(json.RawMessage(nil), step.Arguments...),
		})
	}
	return calls
}

func (r *MerchantCatalogRuntime) executeInteractionTools(
	ctx context.Context,
	resp merchantCatalogInteractionResponse,
	calls []merchantCatalogFunctionCall,
	input merchantcatalogai.RuntimeInput,
) ([]map[string]any, error) {
	toolResults := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		if strings.TrimSpace(call.Name) == "" || strings.TrimSpace(call.ID) == "" {
			return nil, errors.New("Gemini returned an invalid function call")
		}
		if len(call.Arguments) == 0 {
			call.Arguments = []byte("{}")
		}

		started := time.Now()
		log.Printf("[MerchantCatalogAI][TOOL] START business=%s session=%s tool=%s call_id=%s",
			input.BusinessID, input.SessionID, call.Name, call.ID)

		execCtx := ports.AICapabilityExecutionContext{
			BusinessID:     input.BusinessID,
			ConversationID: input.SessionID,
			PrincipalID:    input.PrincipalID,
		}
		result, err := input.Capabilities.Execute(ctx, execCtx, call.Name, call.Arguments)
		if err != nil {
			log.Printf("[MerchantCatalogAI][TOOL] ERROR business=%s session=%s tool=%s latency_ms=%d err=%v",
				input.BusinessID, input.SessionID, call.Name, time.Since(started).Milliseconds(), err)
			return nil, fmt.Errorf("B2B catalog read tool %s failed: %w", call.Name, err)
		}

		resultJSON, err := json.Marshal(result.Data)
		if err != nil {
			return nil, fmt.Errorf("encode result from tool %s: %w", call.Name, err)
		}
		log.Printf("[MerchantCatalogAI][TOOL] OK business=%s session=%s tool=%s latency_ms=%d",
			input.BusinessID, input.SessionID, call.Name, time.Since(started).Milliseconds())

		toolResults = append(toolResults, map[string]any{
			"type":    "function_result",
			"name":    call.Name,
			"call_id": call.ID,
			"result": []map[string]any{
				{"type": "text", "text": string(resultJSON)},
			},
		})
	}
	return toolResults, nil
}

func merchantCatalogThinkingLevel(model string) string {
	name := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(name, "gemini-3.5-flash-lite"),
		strings.HasPrefix(name, "gemini-3.5-flash"),
		strings.HasPrefix(name, "gemini-3.6-flash"),
		strings.HasPrefix(name, "gemini-3-flash-preview"):
		return "minimal"
	default:
		return ""
	}
}

func incompleteMerchantCatalogInteractionError(resp merchantCatalogInteractionResponse, maxOutput int) error {
	return fmt.Errorf(
		"Gemini merchant catalog interaction incomplete before structured proposal: status=%s steps=%d output_chars=%d max_output_tokens=%d; increase the active AI output-token limit",
		resp.Status,
		len(resp.Steps),
		len([]rune(strings.TrimSpace(resp.OutputText))),
		maxOutput,
	)
}

func parseMerchantCatalogProposal(resp merchantCatalogInteractionResponse) (merchantcatalogai.Proposal, error) {
	raw := strings.TrimSpace(resp.OutputText)
	if raw == "" {
		for i := len(resp.Steps) - 1; i >= 0; i-- {
			if resp.Steps[i].Type != "model_output" {
				continue
			}
			for _, part := range resp.Steps[i].Content {
				if strings.TrimSpace(part.Text) != "" {
					raw = strings.TrimSpace(part.Text)
					break
				}
			}
			if raw != "" {
				break
			}
		}
	}
	if raw == "" {
		return merchantcatalogai.Proposal{}, errors.New("Gemini returned no merchant catalog proposal")
	}

	var proposal merchantcatalogai.Proposal
	if err := json.Unmarshal([]byte(raw), &proposal); err != nil {
		return merchantcatalogai.Proposal{}, fmt.Errorf("decode merchant catalog proposal: %w", err)
	}
	return proposal, nil
}

func merchantCatalogProposalSchema() map[string]any {
	stringField := func() map[string]any { return map[string]any{"type": "string"} }
	optionalString := func() map[string]any { return map[string]any{"type": "string"} }
	attributeObjectField := func() map[string]any {
		return map[string]any{
			"type": "object",
			"propertyNames": map[string]any{
				"pattern": "^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$",
			},
			"additionalProperties": true,
			"description": "Dynamic attribute object. Keys are English snake_case; values may be any valid JSON value. Keys do not need to exist in the database.",
		}
	}

	offerNameSource := map[string]any{
		"type": "string",
		"enum": []string{merchantcatalogai.OfferNameSourceSystemDefault, merchantcatalogai.OfferNameSourceMerchantStated},
	}
	offerPriceSource := map[string]any{
		"type": "string",
		"enum": []string{merchantcatalogai.OfferPriceSourceMerchantStated, merchantcatalogai.OfferPriceSourceNotStated},
	}
	offerPricingMode := map[string]any{
		"type": "string",
		"enum": []string{"fixed", "starting_from", "per_unit", "per_person", "per_day", "quote_required", "dynamic"},
	}
	offerName := map[string]any{
		"type": "string",
		"description": fmt.Sprintf("Commercial offer name. If the merchant did not explicitly provide a distinct commercial label, name_source must be system_default and name must be exactly %q. Never append or derive the variant name, color, option, or attribute to the default offer name.", merchantcatalogai.DefaultOfferName),
	}
	offerAmount := map[string]any{
		"type": []string{"string", "null"},
		"description": "Exact merchant-supplied amount when price_source is merchant_stated. Null is allowed only when price_source is not_stated or pricing_mode is dynamic/quote_required according to the contract.",
	}
	offerPricingSemantics := []map[string]any{
		{"properties": map[string]any{
			"name_source": map[string]any{"enum": []string{merchantcatalogai.OfferNameSourceSystemDefault}},
			"name": map[string]any{"enum": []string{merchantcatalogai.DefaultOfferName}},
		}},
		{"properties": map[string]any{
			"name_source": map[string]any{"enum": []string{merchantcatalogai.OfferNameSourceMerchantStated}},
		}},
		{"properties": map[string]any{
			"price_source": map[string]any{"enum": []string{merchantcatalogai.OfferPriceSourceMerchantStated}},
			"amount": map[string]any{"type": "string"},
			"pricing_mode": map[string]any{"enum": []string{"fixed", "starting_from", "per_unit", "per_person", "per_day", "dynamic"}},
		}},
		{"properties": map[string]any{
			"price_source": map[string]any{"enum": []string{merchantcatalogai.OfferPriceSourceNotStated}},
			"amount": map[string]any{"type": "null"},
		}},
	}

	missingField := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": stringField(), "display_name": stringField(), "data_type": stringField(), "reason": stringField(),
		},
		"required": []string{"path", "display_name", "data_type", "reason"},
	}
	itemCreate := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": stringField(), "attribute_schema_id": optionalString(), "item_type": stringField(),
			"short_description": optionalString(), "long_description": optionalString(), "pricing_mode": stringField(),
			"availability_mode": stringField(), "fulfillment_mode": stringField(),
			"requires_confirmation": map[string]any{"type": "boolean"},
			"attributes": attributeObjectField(),
			"variants": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ref": stringField(),
						"name": stringField(),
						"attributes": attributeObjectField(),
					},
					"required": []string{"ref", "name"},
				},
				"description": "New variants are not persisted yet. Each variant requires a unique proposal-local ref used by offers to establish relationships before database IDs exist.",
			},
			"offers": map[string]any{
				"type": "array",
				"minItems": 1,
				"description": "A resolved create proposal must include offer data. Provenance is contractual: merchant_stated means the merchant explicitly supplied the price; system_default means Mujeeb supplied the canonical offer name. Never derive a commercial offer name from a variant.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"variant_ref": optionalString(),
						"name": offerName,
						"name_source": offerNameSource,
						"pricing_mode": offerPricingMode,
						"amount": offerAmount,
						"price_source": offerPriceSource,
						"currency": stringField(),
						"pricing_unit": optionalString(),
						"availability_mode": stringField(),
						"availability_status": stringField(),
						"fulfillment_mode": stringField(),
						"status": stringField(),
					},
					"required": []string{"name", "name_source", "pricing_mode", "amount", "price_source", "availability_mode", "availability_status", "fulfillment_mode", "status"},
					"anyOf": offerPricingSemantics,
				},
			},
		},
		"required": []string{"name", "item_type", "pricing_mode", "availability_mode", "fulfillment_mode", "requires_confirmation", "offers"},
	}
	update := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"item_id": stringField(),
			"changes": map[string]any{"type": "object", "properties": map[string]any{
				"name": optionalString(), "status": optionalString(), "attributes": attributeObjectField(),
				"requires_confirmation": map[string]any{"type": "boolean"},
			}},
			"existing_variants": map[string]any{"type": "array", "items": map[string]any{
				"type": "object", "properties": map[string]any{
					"id": stringField(), "name": optionalString(), "attributes": attributeObjectField(), "status": optionalString(),
				}, "required": []string{"id"},
			}},
			"new_variants": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ref": stringField(),
						"name": stringField(),
						"attributes": attributeObjectField(),
					},
					"required": []string{"ref", "name"},
				},
			},
			"existing_offers": map[string]any{"type": "array", "items": map[string]any{
				"type": "object", "properties": map[string]any{
					"id": stringField(), "name": optionalString(), "amount": optionalString(),
					"availability_status": optionalString(), "status": optionalString(),
				}, "required": []string{"id"},
			}},
			"new_offers": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"variant_id": map[string]any{
							"type": "string",
							"description": "Existing Variant database ID returned by a catalog read tool. Use this only when the new offer targets an already-existing variant.",
						},
						"variant_ref": map[string]any{
							"type": "string",
							"description": "Proposal-local ref of a new variant in update.new_variants. Use this before the new variant has a database ID.",
						},
						"name": offerName,
						"name_source": offerNameSource,
						"pricing_mode": offerPricingMode,
						"amount": offerAmount,
						"price_source": offerPriceSource,
						"currency": optionalString(),
						"pricing_unit": optionalString(),
						"availability_mode": stringField(),
						"availability_status": stringField(),
						"fulfillment_mode": stringField(),
						"status": stringField(),
					},
					"required": []string{"name", "name_source", "pricing_mode", "amount", "price_source", "availability_mode", "availability_status", "fulfillment_mode", "status"},
					"anyOf": offerPricingSemantics,
				},
			},
		},
		"required": []string{"item_id", "changes"},
	}
	deletePayload := map[string]any{
		"type": "object",
		"properties": map[string]any{"item_id": stringField(), "reason_given": optionalString()},
		"required": []string{"item_id"},
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"schema_version": map[string]any{"type": "integer", "enum": []int{merchantcatalogai.ProposalSchemaVersion}, "description": "Merchant Catalog AI Proposal Contract version."},
			"status": map[string]any{"type": "string", "enum": []string{"resolved", "ambiguous", "not_found", "needs_more_data"}},
			"operation": map[string]any{"type": "string", "enum": []string{"create", "update", "delete", "ask_merchant"}},
			"response_text": stringField(),
			"evidence_references": map[string]any{"type": "array", "items": stringField()},
			"missing_information": map[string]any{"type": "array", "items": missingField},
			"create": map[string]any{
				"anyOf": []map[string]any{
					itemCreate,
					{"type": "null"},
				},
			},
			"update": map[string]any{
				"anyOf": []map[string]any{
					update,
					{"type": "null"},
				},
			},
			"delete": map[string]any{
				"anyOf": []map[string]any{
					deletePayload,
					{"type": "null"},
				},
			},
		},
		"required": []string{"schema_version", "status", "operation", "response_text", "create", "update", "delete"},
	}
}

var _ merchantcatalogai.Runtime = (*MerchantCatalogRuntime)(nil)

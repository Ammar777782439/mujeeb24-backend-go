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

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/prompts"
	"github.com/google/uuid"
)

type MerchantCatalogRuntime struct {
	client      *Client
	httpClient  *http.Client
	apiKey      string
	model       string
	baseURL     string
	timeout     time.Duration
	maxOutput   int
	maxInput    int
	runRepo     ports.AIRunRepository
	lifecycle   ports.AIRunLifecyclePort
	newID       func() string
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
		client: client,
		httpClient: httpClient,
		apiKey: client.APIKey(),
		model: client.Model(),
		baseURL: client.BaseURL(),
		timeout: client.RequestTimeout(),
		maxOutput: client.MaxOutputTokens(),
		maxInput: client.MaxInputCharacters(),
		newID: uuid.NewString,
	}, nil
}

func (r *MerchantCatalogRuntime) SetRunRepository(repo ports.AIRunRepository) {
	r.runRepo = repo
}

func (r *MerchantCatalogRuntime) SetLifecycle(lifecycle ports.AIRunLifecyclePort) {
	r.lifecycle = lifecycle
}

func (r *MerchantCatalogRuntime) SetConfigurationProvider(provider ports.AIConfigurationProvider) {
	r.configProvider = provider
}

func (r *MerchantCatalogRuntime) SetNewID(newID func() string) {
	if newID != nil {
		r.newID = newID
	}
}

func (r *MerchantCatalogRuntime) Decide(ctx context.Context, input merchantcatalogai.RuntimeInput) (merchantcatalogai.Proposal, error) {
	if strings.TrimSpace(input.Message) == "" {
		return merchantcatalogai.Proposal{}, errors.New("merchant message is required")
	}
	maxInput := r.maxInput
	if r.configProvider != nil {
		cfg, err := r.configProvider.GetActiveConfig(ctx)
		if err != nil {
			return merchantcatalogai.Proposal{}, fmt.Errorf("resolve active AI configuration: %w", err)
		}
		if strings.TrimSpace(cfg.APIKey) == "" {
			return merchantcatalogai.Proposal{}, errors.New("active AI configuration has no valid API key")
		}
		r.apiKey = cfg.APIKey
		r.model = cfg.Model
		r.baseURL = cfg.BaseURL
		r.maxOutput = cfg.MaxOutputTokens
		maxInput = cfg.MaxInputCharacters
	}
	if maxInput > 0 && len([]rune(input.Message)) > maxInput {
		return merchantcatalogai.Proposal{}, fmt.Errorf("merchant message exceeds %d characters", maxInput)
	}

	contextPayload := map[string]any{
		"business_id": input.BusinessID,
		"principal_id": input.PrincipalID,
		"session_id": input.SessionID,
		"catalog": map[string]any{
			"id": input.SelectedCatalog.ID,
			"name": input.SelectedCatalog.Name,
			"status": input.SelectedCatalog.Status,
		},
		"conversation_history": input.History,
		"merchant_message": input.Message,
	}
	encodedContext, err := json.Marshal(contextPayload)
	if err != nil {
		return merchantcatalogai.Proposal{}, fmt.Errorf("encode merchant catalog AI context: %w", err)
	}

	systemText := prompts.MerchantCatalogAIV2SystemPrompt
	if len(input.EntityContract) > 0 {
		systemText += "

# Catalog Entity Contract
" + string(input.EntityContract)
	}

	reqBody := contractGeminiRequest{
		Model: input.SelectedCatalog.ID,
		Store: false,
		SystemInstruction: &contractContent{
			Role: "system",
			Parts: []contractPart{{Text: systemText}},
		},
		Contents: []contractContent{{
			Role: "user",
			Parts: []contractPart{{Text: string(encodedContext)}},
		}},
		GenerationConfig: contractGenerationConfig{
			ResponseMimeType: "application/json",
			ResponseSchema: merchantCatalogProposalSchema(),
			MaxOutputTokens: r.maxOutput,
		},
	}

	reqBody.Model = r.model
	declarations := merchantCatalogToolDeclarations(input.Capabilities)
	if len(declarations) > 0 {
		reqBody.Tools = []contractTools{{FunctionDeclarations: declarations}}
	}

	for {
		if err := ctx.Err(); err != nil {
			return merchantcatalogai.Proposal{}, fmt.Errorf("merchant catalog AI tool loop cancelled: %w", err)
		}
		resp, err := r.send(ctx, reqBody)
		if err != nil {
			return merchantcatalogai.Proposal{}, err
		}

		if !hasFunctionCall(resp) {
			proposal, err := parseMerchantCatalogProposal(resp)
			if err != nil {
				return merchantcatalogai.Proposal{}, err
			}
			return proposal, nil
		}

		if input.Capabilities == nil {
			return merchantcatalogai.Proposal{}, errors.New("Gemini requested a tool but B2B read capabilities are not configured")
		}

		if r.lifecycle != nil && strings.TrimSpace(input.SessionID) != "" {
			// Lifecycle is optional until the B2B AI Run is created by the application layer.
			// No state transition is attempted without an actual AIRun identifier.
		}

		toolContents, err := r.executeTools(ctx, resp, input)
		if err != nil {
			return merchantcatalogai.Proposal{}, err
		}
		reqBody.Contents = append(reqBody.Contents, toolContents...)
		reqBody.PreviousInteractionID = ""
		reqBody.Store = false
	}
}

func (r *MerchantCatalogRuntime) send(ctx context.Context, reqBody contractGeminiRequest) (contractGeminiResponse, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return contractGeminiResponse{}, fmt.Errorf("encode merchant catalog Gemini request: %w", err)
	}

	requestCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", strings.TrimRight(r.baseURL, "/"), r.model)
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return contractGeminiResponse{}, fmt.Errorf("build merchant catalog Gemini request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", r.apiKey)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return contractGeminiResponse{}, fmt.Errorf("merchant catalog Gemini request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return contractGeminiResponse{}, fmt.Errorf("read merchant catalog Gemini response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return contractGeminiResponse{}, fmt.Errorf("merchant catalog Gemini HTTP %d: %s", resp.StatusCode, string(body))
	}

	var out contractGeminiResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return contractGeminiResponse{}, fmt.Errorf("decode merchant catalog Gemini response: %w", err)
	}
	return out, nil
}

func merchantCatalogToolDeclarations(caps ports.AICapabilityDispatcher) []contractFunctionDeclaration {
	if caps == nil {
		return nil
	}
	defs := caps.Definitions()
	out := make([]contractFunctionDeclaration, 0, len(defs))
	for _, definition := range defs {
		out = append(out, contractFunctionDeclaration{
			Name: definition.Name,
			Description: definition.Description,
			Parameters: definition.Parameters,
		})
	}
	return out
}

func (r *MerchantCatalogRuntime) executeTools(ctx context.Context, resp contractGeminiResponse, input merchantcatalogai.RuntimeInput) ([]contractContent, error) {
	modelContent := contractContent{}
	if len(resp.Candidates) > 0 {
		modelContent = resp.Candidates[0].Content
		modelContent.Role = "model"
	}

	toolParts := make([]contractPart, 0)
	for _, call := range extractFunctionCalls(resp) {
		args, err := json.Marshal(call.Args)
		if err != nil {
			return nil, fmt.Errorf("encode tool arguments for %s: %w", call.Name, err)
		}

		execCtx := ports.AICapabilityExecutionContext{
			BusinessID: input.BusinessID,
			ConversationID: input.SessionID,
			PrincipalID: input.PrincipalID,
		}
		result, err := input.Capabilities.Execute(ctx, execCtx, call.Name, args)
		if err != nil {
			return nil, fmt.Errorf("B2B catalog read tool %s failed: %w", call.Name, err)
		}

		responseBytes, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("encode result from tool %s: %w", call.Name, err)
		}
		var response map[string]any
		if err := json.Unmarshal(responseBytes, &response); err != nil {
			return nil, fmt.Errorf("decode result from tool %s: %w", call.Name, err)
		}

		toolParts = append(toolParts, contractPart{
			FunctionResponse: &contractFunctionResponse{
				Name: call.Name,
				ID: call.ID,
				Response: response,
			},
		})
	}

	return []contractContent{
		modelContent,
		{Role: "user", Parts: toolParts},
	}, nil
}

func parseMerchantCatalogProposal(resp contractGeminiResponse) (merchantcatalogai.Proposal, error) {
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return merchantcatalogai.Proposal{}, errors.New("Gemini returned no merchant catalog proposal")
	}
	raw := strings.TrimSpace(resp.Candidates[0].Content.Parts[0].Text)
	if raw == "" {
		return merchantcatalogai.Proposal{}, errors.New("Gemini returned an empty merchant catalog proposal")
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
	objectField := func() map[string]any { return map[string]any{"type": "object"} }

	missingField := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": stringField(),
			"display_name": stringField(),
			"data_type": stringField(),
			"reason": stringField(),
		},
		"required": []string{"path", "display_name", "data_type", "reason"},
	}
	itemCreate := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": stringField(), "item_type": stringField(),
			"pricing_mode": stringField(), "availability_mode": stringField(),
			"fulfillment_mode": stringField(), "requires_confirmation": map[string]any{"type":"boolean"},
			"attributes": objectField(),
			"variants": map[string]any{"type":"array","items":map[string]any{
				"type":"object","properties":map[string]any{"name":stringField(),"attributes":objectField()},
				"required":[]string{"name"},
			}},
			"offers": map[string]any{"type":"array","items":map[string]any{
				"type":"object","properties":map[string]any{
					"variant_id": optionalString(), "variant_name": optionalString(), "name":stringField(),
					"pricing_mode":stringField(), "amount":optionalString(), "currency":optionalString(),
					"pricing_unit":optionalString(), "availability_mode":stringField(),
					"availability_status":stringField(), "fulfillment_mode":stringField(), "status":stringField(),
				},
				"required":[]string{"name","pricing_mode","availability_mode","availability_status","fulfillment_mode","status"},
			}},
		},
		"required":[]string{"name","item_type","pricing_mode","availability_mode","fulfillment_mode","requires_confirmation"},
	}
	update := map[string]any{
		"type":"object",
		"properties":map[string]any{
			"item_id":stringField(),
			"changes":map[string]any{"type":"object","properties":map[string]any{
				"name":optionalString(),"status":optionalString(),"attributes":objectField(),
				"requires_confirmation":map[string]any{"type":"boolean"},
			}},
			"existing_variants":map[string]any{"type":"array","items":map[string]any{
				"type":"object","properties":map[string]any{
					"id":stringField(),"name":optionalString(),"attributes":objectField(),"status":optionalString(),
				},"required":[]string{"id"},
			}},
			"new_variants":map[string]any{"type":"array","items":map[string]any{
				"type":"object","properties":map[string]any{"name":stringField(),"attributes":objectField()},
				"required":[]string{"name"},
			}},
			"existing_offers":map[string]any{"type":"array","items":map[string]any{
				"type":"object","properties":map[string]any{
					"id":stringField(),"name":optionalString(),"amount":optionalString(),
					"availability_status":optionalString(),"status":optionalString(),
				},"required":[]string{"id"},
			}},
			"new_offers":map[string]any{"type":"array","items":itemCreate["properties"].(map[string]any)["offers"].(map[string]any)["items"]},
		},
		"required":[]string{"item_id","changes"},
	}
	deletePayload := map[string]any{
		"type":"object",
		"properties":map[string]any{"item_id":stringField(),"reason_given":optionalString()},
		"required":[]string{"item_id"},
	}

	return map[string]any{
		"type":"object",
		"properties":map[string]any{
			"schema_version":map[string]any{"type":"integer","minimum":1},
			"status":map[string]any{"type":"string","enum":[]string{"resolved","ambiguous","not_found","needs_more_data"}},
			"operation":map[string]any{"type":"string","enum":[]string{"create","update","delete","ask_merchant"}},
			"response_text":stringField(),
			"evidence_references":map[string]any{"type":"array","items":stringField()},
			"missing_information":map[string]any{"type":"array","items":missingField},
			"create":itemCreate,
			"update":update,
			"delete":deletePayload,
		},
		"required":[]string{"schema_version","status","operation","response_text"},
	}
}

var _ merchantcatalogai.Runtime = (*MerchantCatalogRuntime)(nil)

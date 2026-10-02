package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/prompts"
)

type GeminiMerchantCatalogAuthoringAdapter struct {
	httpClient     *http.Client
	apiKey         string
	model          string
	baseURL        string
	timeout        time.Duration
	maxOutput      int
	maxInput       int
	configProvider ports.AIConfigurationProvider
}

func NewGeminiMerchantCatalogAuthoringAdapter(client *Client) (*GeminiMerchantCatalogAuthoringAdapter, error) {
	if client == nil {
		return nil, errors.New("gemini client is required")
	}
	httpClient := client.HTTPClient()
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &GeminiMerchantCatalogAuthoringAdapter{
		httpClient: httpClient,
		apiKey:     client.APIKey(),
		model:      client.Model(),
		baseURL:    client.BaseURL(),
		timeout:    client.RequestTimeout(),
		maxOutput:  client.MaxOutputTokens(),
		maxInput:   client.MaxInputCharacters(),
	}, nil
}

func (r *GeminiMerchantCatalogAuthoringAdapter) SetConfigurationProvider(provider ports.AIConfigurationProvider) {
	r.configProvider = provider
}

func (r *GeminiMerchantCatalogAuthoringAdapter) Propose(ctx context.Context, input merchantcatalogai.MerchantCatalogAuthoringInput) (merchantcatalogai.Proposal, error) {
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

var _ merchantcatalogai.MerchantCatalogAuthoringPort = (*GeminiMerchantCatalogAuthoringAdapter)(nil)

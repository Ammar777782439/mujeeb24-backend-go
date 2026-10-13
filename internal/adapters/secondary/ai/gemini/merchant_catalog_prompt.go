package gemini

import (
	"encoding/json"
	"fmt"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/ai/prompts"
)

// buildMerchantCatalogAuthoringPrompt is shared by the direct Gemini
// implementation and the LiteLLM implementation. Neither provider owns the
// prompt or merchant-scoped context contract.
func buildMerchantCatalogAuthoringPrompt(input merchantcatalogai.MerchantCatalogAuthoringInput) (string, string, error) {
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
	contextJSON, err := json.Marshal(contextPayload)
	if err != nil {
		return "", "", fmt.Errorf("encode merchant catalog AI context: %w", err)
	}
	systemText := prompts.MerchantCatalogAIV2SystemPrompt
	if len(input.EntityContract) > 0 {
		systemText += "\n\n# Catalog Entity Contract\n" + string(input.EntityContract)
	}
	return systemText, string(contextJSON), nil
}

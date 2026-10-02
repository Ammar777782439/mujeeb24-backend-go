package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/merchantcatalogai"
)

func merchantCatalogInteractionTools(caps merchantcatalogai.MerchantCatalogDiscoveryPort) []merchantCatalogInteractionTool {
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

func (r *GeminiMerchantCatalogAuthoringAdapter) executeInteractionTools(
	ctx context.Context,
	resp merchantCatalogInteractionResponse,
	calls []merchantCatalogFunctionCall,
	input merchantcatalogai.MerchantCatalogAuthoringInput,
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

		execCtx := merchantcatalogai.MerchantCatalogDiscoveryExecutionContext{
			BusinessID: input.BusinessID,
			ConversationID: input.SessionID,
			PrincipalID: input.PrincipalID,
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

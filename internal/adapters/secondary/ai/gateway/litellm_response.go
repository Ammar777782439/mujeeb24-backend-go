package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// normalizedLiteLLMResponse is the narrow provider-neutral wire result.
// No tool is executed at this boundary. Strict Mujeeb parsers decide safety.
type normalizedLiteLLMResponse struct {
	Status        string
	Text          string
	FunctionCalls []interactionStep
	Usage         interactionUsage
}

func normalizeLiteLLMResponse(raw []byte) (normalizedLiteLLMResponse, error) {
	var response struct {
		Status  string            `json:"status"`
		Steps   []interactionStep `json:"steps"`
		Outputs []struct {
			Type      string               `json:"type"`
			ID        string               `json:"id"`
			CallID    string               `json:"call_id"`
			Name      string               `json:"name"`
			Arguments json.RawMessage      `json:"arguments"`
			Text      string               `json:"text"`
			Content   []interactionContent `json:"content"`
		} `json:"outputs"`
		Usage interactionUsage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return normalizedLiteLLMResponse{}, errors.New("invalid LiteLLM response")
	}
	out := normalizedLiteLLMResponse{Status: response.Status, Usage: response.Usage}
	textParts := make([]string, 0)
	addContent := func(parts []interactionContent) {
		for _, part := range parts {
			if (part.Type == "text" || part.Type == "output_text") && strings.TrimSpace(part.Text) != "" {
				textParts = append(textParts, part.Text)
			}
		}
	}
	for _, step := range response.Steps {
		switch step.Type {
		case "function_call":
			out.FunctionCalls = append(out.FunctionCalls, step)
		case "model_output":
			addContent(step.Content)
		}
	}
	for _, item := range response.Outputs {
		switch item.Type {
		case "function_call":
			id := item.ID
			if id == "" {
				id = item.CallID
			}
			args := item.Arguments
			if len(args) > 0 && args[0] == '"' {
				var encoded string
				if err := json.Unmarshal(args, &encoded); err != nil {
					return normalizedLiteLLMResponse{}, fmt.Errorf("invalid tool argument encoding: %w", err)
				}
				args = json.RawMessage(encoded)
			}
			out.FunctionCalls = append(out.FunctionCalls, interactionStep{Type: "function_call", ID: id, Name: item.Name, Arguments: args})
		case "message", "model_output":
			addContent(item.Content)
			if strings.TrimSpace(item.Text) != "" {
				textParts = append(textParts, item.Text)
			}
		case "text", "output_text":
			if strings.TrimSpace(item.Text) != "" {
				textParts = append(textParts, item.Text)
			}
		}
	}
	out.Text = strings.TrimSpace(strings.Join(textParts, "\n"))
	return out, nil
}

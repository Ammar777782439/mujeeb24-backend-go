package gemini

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Shared strict interaction contracts retained by the LiteLLM adapters.
func decodeStrictStructuredJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("structured output contains multiple JSON values")
		}
		return fmt.Errorf("decode trailing structured output: %w", err)
	}
	return nil
}

func interactionOutputText(resp interactionResponse) (string, error) {
	for i := len(resp.Steps) - 1; i >= 0; i-- {
		step := resp.Steps[i]
		if step.Type != "model_output" {
			continue
		}
		for _, content := range step.Content {
			if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
				return content.Text, nil
			}
		}
	}
	return "", errors.New("AI interaction completed without model text output")
}

type interactionRequest struct {
	Model                 string                      `json:"model"`
	Input                 string                      `json:"input"`
	SystemInstruction     string                      `json:"system_instruction,omitempty"`
	PreviousInteractionID string                      `json:"previous_interaction_id,omitempty"`
	Store                 bool                        `json:"store"`
	ResponseFormat        *interactionResponseFormat  `json:"response_format,omitempty"`
	Tools                 []customerSalesTool         `json:"tools,omitempty"`
	GenerationConfig      interactionGenerationConfig `json:"generation_config,omitempty"`
}

type interactionResponseFormat struct {
	Type     string         `json:"type"`
	MimeType string         `json:"mime_type,omitempty"`
	Schema   map[string]any `json:"schema,omitempty"`
}

type interactionGenerationConfig struct {
	MaxOutputTokens int    `json:"max_output_tokens,omitempty"`
	ToolChoice      string `json:"tool_choice,omitempty"`
}

type interactionResponse struct {
	ID     string             `json:"id"`
	Status string             `json:"status"`
	Steps  []interactionStep  `json:"steps"`
	Usage  interactionUsage   `json:"usage"`
	Errors []interactionError `json:"errors,omitempty"`
}

type interactionStep struct {
	ID        string               `json:"id,omitempty"`
	Name      string               `json:"name,omitempty"`
	Arguments json.RawMessage      `json:"arguments,omitempty"`
	Type      string               `json:"type"`
	Content   []interactionContent `json:"content,omitempty"`
}

type interactionContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type interactionUsage struct {
	TotalInputTokens  int `json:"total_input_tokens,omitempty"`
	TotalCachedTokens int `json:"total_cached_tokens,omitempty"`
	TotalOutputTokens int `json:"total_output_tokens,omitempty"`
	TotalTokens       int `json:"total_tokens,omitempty"`
}

type interactionError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}


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
)

type merchantCatalogInteractionRequest struct {
	Model                 string                            `json:"model"`
	Store                 bool                              `json:"store"`
	Input                 any                               `json:"input"`
	SystemInstruction     string                            `json:"system_instruction,omitempty"`
	Tools                 []merchantCatalogInteractionTool  `json:"tools,omitempty"`
	ResponseFormat        *merchantCatalogInteractionFormat `json:"response_format,omitempty"`
	GenerationConfig      *merchantCatalogGenerationConfig  `json:"generation_config,omitempty"`
	PreviousInteractionID string                            `json:"previous_interaction_id,omitempty"`
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
	MaxOutputTokens   int    `json:"max_output_tokens,omitempty"`
	ThinkingLevel     string `json:"thinking_level,omitempty"`
	ThinkingSummaries string `json:"thinking_summaries,omitempty"`
}

type merchantCatalogInteractionResponse struct {
	ID         string                           `json:"id"`
	Status     string                           `json:"status"`
	OutputText string                           `json:"output_text,omitempty"`
	Steps      []merchantCatalogInteractionStep `json:"steps,omitempty"`
	Usage      merchantCatalogUsage             `json:"usage,omitempty"`
}

type merchantCatalogInteractionStep struct {
	Type      string                      `json:"type"`
	ID        string                      `json:"id,omitempty"`
	Name      string                      `json:"name,omitempty"`
	Arguments json.RawMessage             `json:"arguments,omitempty"`
	Result    json.RawMessage             `json:"result,omitempty"`
	Content   []merchantCatalogOutputPart `json:"content,omitempty"`
	Summary   []merchantCatalogOutputPart `json:"summary,omitempty"`
}

type merchantCatalogOutputPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type merchantCatalogUsage struct {
	InputTokens   int `json:"total_input_tokens,omitempty"`
	OutputTokens  int `json:"total_output_tokens,omitempty"`
	ThoughtTokens int `json:"total_thought_tokens,omitempty"`
}

type merchantCatalogFunctionCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

func (r *GeminiMerchantCatalogAuthoringAdapter) sendInteraction(ctx context.Context, reqBody merchantCatalogInteractionRequest, apiKey, baseURL string) (merchantCatalogInteractionResponse, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return merchantCatalogInteractionResponse{}, fmt.Errorf("encode Gemini interaction request: %w", err)
	}

	log.Printf("[MerchantCatalogAI][INTERACTION_REQUEST] json=%s", payload)

	requestCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	url := fmt.Sprintf("%s/v1/interactions", strings.TrimRight(baseURL, "/"))

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

		log.Printf("[MerchantCatalogAI][INTERACTION_RESPONSE] status=%d json=%s", resp.StatusCode, body)

		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("merchant catalog Gemini HTTP %d: %s", resp.StatusCode, string(body))
			if !isRetryableGeminiStatus(resp.StatusCode) || attempt == maxAttempts {
				return merchantCatalogInteractionResponse{}, lastErr
			}

			delay := time.Duration(1<<(attempt-1)) * time.Second
			logGeminiRetry(resp.StatusCode, attempt, maxAttempts, delay)
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
		len([]rune(merchantCatalogOutputText(resp))),
		maxOutput,
	)
}

func merchantCatalogOutputText(resp merchantCatalogInteractionResponse) string {
	if raw := strings.TrimSpace(resp.OutputText); raw != "" {
		return raw
	}
	for i := len(resp.Steps) - 1; i >= 0; i-- {
		if resp.Steps[i].Type != "model_output" {
			continue
		}
		var output strings.Builder
		for _, part := range resp.Steps[i].Content {
			if part.Type == "text" {
				output.WriteString(part.Text)
			}
		}
		if raw := strings.TrimSpace(output.String()); raw != "" {
			return raw
		}
	}
	return ""
}

// logMerchantCatalogInteractionContent logs the provider's exposed summary,
// not encrypted signatures or private internal reasoning.
func logMerchantCatalogInteractionContent(resp merchantCatalogInteractionResponse) {
	for i, step := range resp.Steps {
		if step.Type != "thought" {
			continue
		}
		for _, part := range step.Summary {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				log.Printf("[MerchantCatalogAI][THOUGHT_SUMMARY] interaction=%s step=%d text=%s", resp.ID, i, part.Text)
			}
		}
	}
	raw := merchantCatalogOutputText(resp)
	if raw == "" {
		return
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, []byte(raw), "", "  "); err == nil {
		raw = pretty.String()
	}
	log.Printf("[MerchantCatalogAI][MODEL_OUTPUT] interaction=%s json=%s", resp.ID, raw)
}

func parseMerchantCatalogProposal(resp merchantCatalogInteractionResponse) (merchantcatalogai.Proposal, error) {
	raw := merchantCatalogOutputText(resp)
	if raw == "" {
		return merchantcatalogai.Proposal{}, errors.New("Gemini returned no merchant catalog proposal")
	}

	var proposal merchantcatalogai.Proposal
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proposal); err != nil {
		return merchantcatalogai.Proposal{}, fmt.Errorf("decode merchant catalog proposal: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return merchantcatalogai.Proposal{}, errors.New("Gemini returned trailing data after merchant catalog proposal")
	}
	return proposal, nil
}

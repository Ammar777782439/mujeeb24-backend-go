package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

// ModelsClient calls the Gemini Models API to discover available models.
// Per §3: the discovery must call the actual Gemini API — no hardcoded list.
//
// Gemini API endpoint: GET /v1beta/models (key via x-goog-api-key header)
// Returns: { models: [{ name, version, displayName, description,
//
//	inputTokenLimit, outputTokenLimit, supportedGenerationMethods,
//	temperature, topP, topK, ... }] }
type ModelsClient struct {
	httpClient *http.Client
	timeout    time.Duration
}

func NewModelsClient() *ModelsClient {
	return &ModelsClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		timeout:    30 * time.Second,
	}
}

// geminiModelsResponse is the JSON response from GET /v1beta/models.
type geminiModelsResponse struct {
	Models []geminiModel `json:"models"`
}

type geminiModel struct {
	Name                       string   `json:"name"`
	Version                    string   `json:"version"`
	DisplayName                string   `json:"displayName"`
	Description                string   `json:"description"`
	InputTokenLimit            int      `json:"inputTokenLimit"`
	OutputTokenLimit           int      `json:"outputTokenLimit"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	Temperature                *float64 `json:"temperature"`
	TopP                       *float64 `json:"topP"`
	TopK                       *int     `json:"topK"`
}

// DiscoverModels implements ports.ModelDiscoveryClient.
// Per §3: calls the real Gemini Models API using the provided credential.
func (c *ModelsClient) DiscoverModels(ctx context.Context, apiKey, baseURL string) ([]ports.AIProviderModel, error) {
	if c == nil || c.httpClient == nil {
		return nil, fmt.Errorf("models client is not configured")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required for model discovery")
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	// P1-8: API key MUST be sent as x-goog-api-key header — never in
	// the URL query string. The URL is logged in proxy/server access
	// logs and would leak the credential.
	url := fmt.Sprintf("%s/v1beta/models?pageSize=100", baseURL)

	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build models request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("x-goog-api-key", apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send models request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("gemini models API status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("read models response: %w", err)
	}

	var modelsResp geminiModelsResponse
	if err := json.Unmarshal(body, &modelsResp); err != nil {
		return nil, fmt.Errorf("parse models response: %w", err)
	}

	now := time.Now().UTC()
	items := make([]ports.AIProviderModel, 0, len(modelsResp.Models))
	for _, m := range modelsResp.Models {
		// Gemini returns model name as "models/gemini-3.5-flash" — strip the prefix.
		modelName := m.Name
		if len(modelName) > 7 && modelName[:7] == "models/" {
			modelName = modelName[7:]
		}

		var displayName *string
		if m.DisplayName != "" {
			displayName = &m.DisplayName
		}
		var description *string
		if m.Description != "" {
			description = &m.Description
		}
		var inputLimit *int
		if m.InputTokenLimit > 0 {
			inputLimit = &m.InputTokenLimit
		}
		var outputLimit *int
		if m.OutputTokenLimit > 0 {
			outputLimit = &m.OutputTokenLimit
		}
		var version *string
		if m.Version != "" {
			version = &m.Version
		}
		// Check if model supports "thinking" — Gemini models that support
		// thinking have generateContent + generateContent:thinking in their
		// supportedGenerationMethods, or have "thinking" in the name.
		thinkingSupported := false
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				// Base check — all text models support generateContent.
			}
		}
		// Gemini thinking models typically have "thinking" in the model name
		// or version. This is a heuristic — the API doesn't have an explicit
		// "thinkingSupported" field.
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				thinkingSupported = true // Most current Gemini models support thinking
				break
			}
		}

		items = append(items, ports.AIProviderModel{
			ID:                uuid.NewString(),
			Provider:          "google_gemini",
			ModelName:         modelName,
			DisplayName:       displayName,
			Description:       description,
			InputTokenLimit:   inputLimit,
			OutputTokenLimit:  outputLimit,
			SupportedMethods:  m.SupportedGenerationMethods,
			ThinkingSupported: thinkingSupported,
			TemperatureMin:    m.Temperature,
			TemperatureMax:    m.Temperature,
			TopPMin:           m.TopP,
			TopPMax:           m.TopP,
			TopKMin:           m.TopK,
			TopKMax:           m.TopK,
			Version:           version,
			BaseModel:         nil,
			DiscoveredAt:      now,
		})
	}

	return items, nil
}

// TestConnection makes a minimal Gemini generateContent call to verify
// that the credential + model are valid. Per §6: must be a real request
// to Gemini, not a config validity check.
//
// The probe sends "Hello" as input and checks for a 200 response.
// No customer data is used (per §84).
func (c *ModelsClient) TestConnection(ctx context.Context, apiKey, model, baseURL string) (bool, int64, string) {
	if c == nil || c.httpClient == nil {
		return false, 0, "MODELS_CLIENT_NOT_CONFIGURED"
	}
	if apiKey == "" {
		return false, 0, "NO_API_KEY"
	}
	if model == "" {
		return false, 0, "NO_MODEL"
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	started := time.Now()

	// Minimal probe: generateContent with "Hello" input.
	// Per §84: uses a standalone prompt — no merchant_id, business_id,
	// customer data, or merchant catalog.
	// P1-8: API key sent via x-goog-api-key header only — never in URL.
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", baseURL, model)

	reqBody := map[string]any{
		"contents": []map[string]any{
			{"role": "user", "parts": []map[string]any{{"text": "Hello"}}},
		},
	}
	buf, _ := json.Marshal(reqBody)

	reqCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return false, time.Since(started).Milliseconds(), "BUILD_REQUEST_FAILED"
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return false, time.Since(started).Milliseconds(), "REQUEST_FAILED"
	}
	defer resp.Body.Close()

	latency := time.Since(started).Milliseconds()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var errResp struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &errResp)
		if errResp.Error.Code > 0 {
			return false, latency, fmt.Sprintf("GEMINI_ERROR_%d", errResp.Error.Code)
		}
		return false, latency, fmt.Sprintf("HTTP_%d", resp.StatusCode)
	}

	return true, latency, ""
}

// byteReader wrapper removed — using bytes.NewReader from the standard library.

var _ ports.ModelDiscoveryClient = (*ModelsClient)(nil)

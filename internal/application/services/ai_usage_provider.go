package services

import "strings"

// usageProvider keeps historical Gemini-only test adapters and telemetry
// compatible, while preserving the actual provider label from direct model calls.
// The Native Gemini/DeepSeek model name is a priced route alias until upstream per-attempt
// accounting is wired; do not assume it identifies the underlying company.
func usageProvider(provider string) string {
	if strings.TrimSpace(provider) == "" {
		return "google_gemini"
	}
	return strings.TrimSpace(provider)
}

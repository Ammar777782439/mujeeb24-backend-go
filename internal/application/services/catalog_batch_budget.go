package services

import "fmt"

// LiteLLMCatalogTokenBudget bounds EVERY catalog request, including candidate
// reduction and final evaluation. contextWindow is the minimum effective
// context capacity among ALL LiteLLM catalog deployments and fallbacks.
// Budgeting is deliberately conservative: no catalog facts may be truncated.
func LiteLLMCatalogTokenBudget(contextWindow, maxOutputTokens int) (int, error) {
	if contextWindow <= 0 || maxOutputTokens <= 0 {
		return 0, fmt.Errorf("catalog context window and output reserve must be positive")
	}
	safetyTokens := contextWindow / 10
	if safetyTokens < 2048 {
		safetyTokens = 2048
	}
	if contextWindow-maxOutputTokens <= safetyTokens {
		return 0, fmt.Errorf("catalog context window %d cannot reserve %d output and %d safety tokens", contextWindow, maxOutputTokens, safetyTokens)
	}
	return contextWindow - maxOutputTokens - safetyTokens, nil
}

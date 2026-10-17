package services

import (
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// nativeProviderSpend never multiplies token quantities by homemade model prices.
// LiteLLM owns USD spend; PostgreSQL converts accumulated native USD to YER
// using an explicitly configured exchange rate. Missing is NOT equivalent to $0.
func nativeProviderSpend(usage ports.CustomerSalesUsageTelemetry, fxRate string) (string, string, string, bool) {
	if (usage.Provider != "gemini" && usage.Provider != "deepseek") || !usage.GatewayCostKnown ||
		strings.TrimSpace(usage.GatewayCostUSD) == "" || strings.TrimSpace(fxRate) == "" {
		return "unknown", "", "", true
	}
	return "native_provider_usd", usage.GatewayCostUSD, fxRate, false
}

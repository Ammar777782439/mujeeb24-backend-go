package services

import (
	"context"
	"math/big"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// nativeProviderSpend never multiplies token quantities by homemade model prices.
// Provider-verified USD spend, when present, is converted to YER
// using an explicitly configured exchange rate. Missing is NOT equivalent to $0.
func nativeProviderSpend(usage ports.CustomerSalesUsageTelemetry, fxRate string) (string, string, string, bool) {
	if (usage.Provider != "gemini" && usage.Provider != "deepseek") || !usage.GatewayCostKnown ||
		strings.TrimSpace(usage.GatewayCostUSD) == "" || strings.TrimSpace(fxRate) == "" {
		return "unknown", "", "", true
	}
	return "native_provider_usd", usage.GatewayCostUSD, fxRate, false
}

// nativeTokenPricedSpend values the provider-reported token usage using an
// explicitly approved, immutable pricing version in the platform database.
// It never guesses a model rate or claims to have a provider invoice.
// A missing version stays pricing_failed; it is NOT zero provider spend.
func nativeTokenPricedSpend(ctx context.Context, pricing ports.AIProviderPricingRepository, usage ports.CustomerSalesUsageTelemetry) (int, string, bool) {
	if pricing == nil || strings.TrimSpace(usage.Provider) == "" || strings.TrimSpace(usage.Model) == "" ||
		usage.InputTokens < 0 || usage.CachedTokens < 0 || usage.CachedTokens > usage.InputTokens || usage.OutputTokens < 0 {
		return 0, "unknown", true
	}
	version, err := pricing.GetCurrentForProvider(ctx, usageProvider(usage.Provider), usage.Model)
	if err != nil || strings.TrimSpace(version.PricingVersion) == "" ||
		version.InputPerMillionYER <= 0 || version.CachedInputPerMillionYER < 0 ||
		version.OutputPerMillionYER <= 0 {
		return 0, "unknown", true
	}
	uncached := int64(usage.InputTokens - usage.CachedTokens)
	cached := int64(usage.CachedTokens)
	output := int64(usage.OutputTokens)
	total := new(big.Int)
	total.Add(total, new(big.Int).Mul(big.NewInt(uncached), big.NewInt(int64(version.InputPerMillionYER))))
	total.Add(total, new(big.Int).Mul(big.NewInt(cached), big.NewInt(int64(version.CachedInputPerMillionYER))))
	total.Add(total, new(big.Int).Mul(big.NewInt(output), big.NewInt(int64(version.OutputPerMillionYER))))
	if total.Sign() < 0 {
		return 0, "unknown", true
	}
	// The persisted budget has integer YER precision; round each recorded
	// inference cost up to avoid silently erasing non-zero provider spend.
	total.Add(total, big.NewInt(999999))
	total.Div(total, big.NewInt(1000000))
	maxInt := int64(^uint(0) >> 1)
	if !total.IsInt64() || total.Int64() > maxInt {
		return 0, "unknown", true
	}
	return int(total.Int64()), version.PricingVersion, false
}

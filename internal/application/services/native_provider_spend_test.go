package services

import (
	"context"
	"errors"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type verifiedPricingStub struct {
	version  ports.AIProviderPricingVersion
	provider string
	model    string
}

func (p *verifiedPricingStub) GetCurrentForProvider(_ context.Context, provider, model string) (ports.AIProviderPricingVersion, error) {
	p.provider, p.model = provider, model
	return p.version, nil
}

func (*verifiedPricingStub) CreatePricingVersion(context.Context, ports.AIProviderPricingCreate) (ports.AIProviderPricingVersion, error) {
	return ports.AIProviderPricingVersion{}, errors.New("not implemented for read-only cost test")
}

func (*verifiedPricingStub) GetByID(context.Context, string) (ports.AIProviderPricingVersion, error) {
	return ports.AIProviderPricingVersion{}, errors.New("not implemented for read-only cost test")
}

func (*verifiedPricingStub) ListByProvider(context.Context, string) ([]ports.AIProviderPricingVersion, error) {
	return nil, errors.New("not implemented for read-only cost test")
}

func TestNativeTokenPricedSpendUsesImmutableModelVersion(t *testing.T) {
	pricing := &verifiedPricingStub{version: ports.AIProviderPricingVersion{
		PricingVersion:           "verified-example-1",
		InputPerMillionYER:       1000000,
		CachedInputPerMillionYER: 500000,
		OutputPerMillionYER:      2000000,
	}}
	cost, version, failed := nativeTokenPricedSpend(context.Background(), pricing, ports.CustomerSalesUsageTelemetry{
		Provider: "gemini", Model: "gemini-3.1-flash-lite",
		InputTokens: 5000, CachedTokens: 1000, OutputTokens: 1000,
	})
	if failed || cost != 6500 || version != "verified-example-1" {
		t.Fatalf("bad configured-rate calculation cost=%d version=%s failed=%t", cost, version, failed)
	}
	if pricing.provider != "gemini" || pricing.model != "gemini-3.1-flash-lite" {
		t.Fatalf("wrong priced route provider=%s model=%s", pricing.provider, pricing.model)
	}
}

func TestNativeTokenPricedSpendFailsWithoutPricingAndRejectsInvalidUsage(t *testing.T) {
	usage := ports.CustomerSalesUsageTelemetry{Provider: "gemini", Model: "gemini-3.1-flash-lite", InputTokens: 500, OutputTokens: 100}
	if cost, _, failed := nativeTokenPricedSpend(context.Background(), nil, usage); !failed || cost != 0 {
		t.Fatalf("missing price falsely succeeded: %d", cost)
	}
	pricing := &verifiedPricingStub{version: ports.AIProviderPricingVersion{
		PricingVersion: "p", InputPerMillionYER: 1000, OutputPerMillionYER: 1000,
	}}
	usage.CachedTokens = 600
	if _, _, failed := nativeTokenPricedSpend(context.Background(), pricing, usage); !failed {
		t.Fatal("cached tokens greater than total were accepted")
	}
	usage.CachedTokens = 0
	usage.InputTokens = 1
	usage.OutputTokens = 0
	if cost, _, failed := nativeTokenPricedSpend(context.Background(), pricing, usage); failed || cost != 1 {
		t.Fatalf("small positive native charge rounded down: cost=%d failed=%t", cost, failed)
	}
}

func TestNativeProviderSpendAcceptsVerifiedCostForAnyNamedProvider(t *testing.T) {
	for _, provider := range []string{"deepseek", "future-provider", "another-model-host"} {
		t.Run(provider, func(t *testing.T) {
			version, usd, rate, failed := nativeProviderSpend(ports.CustomerSalesUsageTelemetry{
				Provider: provider, Model: "model-v1", GatewayCostKnown: true, GatewayCostUSD: "0.0125",
			}, "245.50")
			if failed || version != "native_provider_usd" || usd != "0.0125" || rate != "245.50" {
				t.Fatalf("valid verified spend rejected for provider=%s: version=%s usd=%s rate=%s failed=%t", provider, version, usd, rate, failed)
			}
		})
	}
}

func TestNativeProviderSpendFailsClosedForMissingEvidence(t *testing.T) {
	cases := []struct {
		name, provider, usd, rate string
		known                     bool
	}{
		{"missing provider", "", "0.0125", "245", true},
		{"blank provider", "  ", "0.0125", "245", true},
		{"unverified cost", "future-provider", "0.0125", "245", false},
		{"missing cost", "future-provider", "", "245", true},
		{"missing conversion", "future-provider", "0.0125", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			version, usd, rate, failed := nativeProviderSpend(ports.CustomerSalesUsageTelemetry{
				Provider: tc.provider, Model: "model-v1", GatewayCostKnown: tc.known, GatewayCostUSD: tc.usd,
			}, tc.rate)
			if !failed || version != "unknown" || usd != "" || rate != "" {
				t.Fatalf("missing or unverified spend was accepted: version=%s usd=%s rate=%s failed=%t", version, usd, rate, failed)
			}
		})
	}
}

func TestNativeTokenPricedSpendUsesAnyConfiguredProviderModel(t *testing.T) {
	pricing := &verifiedPricingStub{version: ports.AIProviderPricingVersion{
		PricingVersion: "approved-future-v1",
		InputPerMillionYER: 1000000, CachedInputPerMillionYER: 500000, OutputPerMillionYER: 2000000,
	}}
	cost, version, failed := nativeTokenPricedSpend(context.Background(), pricing, ports.CustomerSalesUsageTelemetry{
		Provider: "future-provider", Model: "model-v1",
		InputTokens: 5000, CachedTokens: 1000, OutputTokens: 1000,
	})
	if failed || cost != 6500 || version != "approved-future-v1" {
		t.Fatalf("approved provider-neutral pricing failed: cost=%d version=%s failed=%t", cost, version, failed)
	}
	if pricing.provider != "future-provider" || pricing.model != "model-v1" {
		t.Fatalf("pricing lookup changed adapter identity: provider=%s model=%s", pricing.provider, pricing.model)
	}
}

func TestUsageProviderPreservesIdentityWithoutVendorFallback(t *testing.T) {
	for _, tc := range []struct{ incoming, want string }{
		{"", ""}, {"  ", ""}, {"future-provider", "future-provider"}, {" future-provider ", "future-provider"},
	} {
		if got := usageProvider(tc.incoming); got != tc.want {
			t.Fatalf("provider identity altered: incoming=%q got=%q want=%q", tc.incoming, got, tc.want)
		}
	}
}

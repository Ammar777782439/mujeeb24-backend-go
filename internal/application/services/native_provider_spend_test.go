package services

import (
	"context"
	"errors"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type verifiedPricingStub struct {
	version ports.AIProviderPricingVersion
	provider string
	model string
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
		PricingVersion: "verified-example-1",
		InputPerMillionYER: 1000000,
		CachedInputPerMillionYER: 500000,
		OutputPerMillionYER: 2000000,
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

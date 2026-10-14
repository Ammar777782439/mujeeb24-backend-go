package services

import (
	"context"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type gatewayRegistryProbe struct{}
func (gatewayRegistryProbe) Probe(context.Context) (ports.ProviderHealthState, int64, *string) {
	return ports.ProviderHealthHealthy, 0, nil
}

func TestPlatformGatewayRegistryDoesNotExposeLegacyProvider(t *testing.T) {
	r := NewInMemoryPlatformOperationsRepository(true, true)
	r.SetAIModel("mujeeb-customer")
	ctx := context.Background()
	provider, err := r.GetProvider(ctx, "litellm")
	if err != nil || provider.ProviderType != ports.ProviderTypeAI ||
		provider.Model != "mujeeb-customer" || provider.DisplayName != "LiteLLM Gateway" {
		t.Fatalf("gateway provider incorrect: %+v %v", provider, err)
	}
	if _, err := r.GetProvider(ctx, "google_gemini"); err == nil {
		t.Fatal("obsolete Gemini provider is still registered")
	}
	r.RegisterProbe("litellm", gatewayRegistryProbe{})
	if _, err := r.DisableRuntime(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	result, err := r.RunHealthCheck(ctx, "litellm", time.Now())
	if err != nil || result.HealthState != ports.ProviderHealthHealthy {
		t.Fatalf("gateway health must use registered probe: %+v %v", result, err)
	}
	state, err := r.GetRuntimeState(ctx)
	if err != nil || state.AdminState != ports.ProviderAdminDisabled {
		t.Fatalf("health probe re-enabled kill switch: %+v %v", state, err)
	}
}

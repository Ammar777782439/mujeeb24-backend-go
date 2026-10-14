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

type blockingGatewayRegistryProbe struct {
	started chan struct{}
	release chan struct{}
}

func (p blockingGatewayRegistryProbe) Probe(context.Context) (ports.ProviderHealthState, int64, *string) {
	close(p.started)
	<-p.release
	return ports.ProviderHealthHealthy, 0, nil
}

func TestGatewayHealthCheckDoesNotBlockEmergencyDisable(t *testing.T) {
	r := NewInMemoryPlatformOperationsRepository(true, false)
	probe := blockingGatewayRegistryProbe{started: make(chan struct{}), release: make(chan struct{})}
	r.RegisterProbe("litellm", probe)
	checkFinished := make(chan struct{})
	go func() {
		defer close(checkFinished)
		_, _ = r.RunHealthCheck(context.Background(), "litellm", time.Now())
	}()
	select {
	case <-probe.started:
	case <-time.After(2 * time.Second):
		close(probe.release)
		t.Fatal("gateway probe did not start")
	}
	disableFinished := make(chan struct{})
	go func() {
		defer close(disableFinished)
		_, _ = r.DisableRuntime(context.Background(), time.Now())
	}()
	select {
	case <-disableFinished:
	case <-time.After(2 * time.Second):
		close(probe.release)
		<-checkFinished
		t.Fatal("emergency AI disable blocked on slow LiteLLM health probe")
	}
	close(probe.release)
	<-checkFinished
	state, err := r.GetRuntimeState(context.Background())
	if err != nil || state.AdminState != ports.ProviderAdminDisabled {
		t.Fatalf("gateway probe changed emergency disable: %+v %v", state, err)
	}
}

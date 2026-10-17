package services

import (
	"context"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type nativeRegistryProbe struct{}

func (nativeRegistryProbe) Probe(context.Context) (ports.ProviderHealthState, int64, *string) {
	return ports.ProviderHealthHealthy, 0, nil
}

func TestPlatformNativeRegistryDoesNotExposeLegacyProvider(t *testing.T) {
	r := NewInMemoryPlatformOperationsRepository(true, true)
	r.SetAIModel("mujeeb-customer")
	ctx := context.Background()
	provider, err := r.GetProvider(ctx, "gemini")
	if err != nil || provider.ProviderType != ports.ProviderTypeAI ||
		provider.Model != "mujeeb-customer" || provider.DisplayName != "Google Gemini primary" {
		t.Fatalf("native provider incorrect: %+v %v", provider, err)
	}
	if _, err := r.GetProvider(ctx, "deepseek"); err != nil {
		t.Fatal("standby DeepSeek provider missing")
	}
	r.RegisterProbe("gemini", nativeRegistryProbe{})
	if _, err := r.DisableRuntime(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	result, err := r.RunHealthCheck(ctx, "gemini", time.Now())
	if err != nil || result.HealthState != ports.ProviderHealthHealthy {
		t.Fatalf("native health must use registered probe: %+v %v", result, err)
	}
	state, err := r.GetRuntimeState(ctx)
	if err != nil || state.AdminState != ports.ProviderAdminDisabled {
		t.Fatalf("health probe re-enabled kill switch: %+v %v", state, err)
	}
}

type blockingNativeRegistryProbe struct {
	started chan struct{}
	release chan struct{}
}

func (p blockingNativeRegistryProbe) Probe(context.Context) (ports.ProviderHealthState, int64, *string) {
	close(p.started)
	<-p.release
	return ports.ProviderHealthHealthy, 0, nil
}

func TestNativeHealthCheckDoesNotBlockEmergencyDisable(t *testing.T) {
	r := NewInMemoryPlatformOperationsRepository(true, false)
	probe := blockingNativeRegistryProbe{started: make(chan struct{}), release: make(chan struct{})}
	r.RegisterProbe("gemini", probe)
	checkFinished := make(chan struct{})
	go func() {
		defer close(checkFinished)
		_, _ = r.RunHealthCheck(context.Background(), "gemini", time.Now())
	}()
	select {
	case <-probe.started:
	case <-time.After(2 * time.Second):
		close(probe.release)
		t.Fatal("native probe did not start")
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
		t.Fatal("emergency AI disable blocked on slow native Gemini health probe")
	}
	close(probe.release)
	<-checkFinished
	state, err := r.GetRuntimeState(context.Background())
	if err != nil || state.AdminState != ports.ProviderAdminDisabled {
		t.Fatalf("native probe changed emergency disable: %+v %v", state, err)
	}
}

package bootstrap

import (
	"context"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type fakeGatewayDiscovery struct {
	success bool
	code string
	observedModel string
	observedKey string
}
func (f *fakeGatewayDiscovery) DiscoverModels(context.Context, string, string) ([]ports.AIProviderModel, error) {
	return nil, nil
}
func (f *fakeGatewayDiscovery) TestConnection(_ context.Context, key, model, _ string) (bool, int64, string) {
	f.observedModel, f.observedKey = model, key
	return f.success, 17, f.code
}

func TestGatewayHealthProbeTargetsDeployedCustomerAlias(t *testing.T) {
	f := &fakeGatewayDiscovery{success: true}
	p := gatewayHealthProbe{settings: &ports.LiteLLMPlatformConfig{
		APIKey: "virtual-key", BaseURL: "http://private-gateway", CustomerModel: "mujeeb-customer",
	}, discovery: f}
	state, latency, code := p.Probe(context.Background())
	if state != ports.ProviderHealthHealthy || latency != 17 || code != nil {
		t.Fatalf("healthy gateway probe: state=%s latency=%d code=%v", state, latency, code)
	}
	if f.observedModel != "mujeeb-customer" || f.observedKey != "virtual-key" {
		t.Fatalf("gateway health checked wrong route or key")
	}
	f.success, f.code = false, "RATE_LIMIT"
	state, _, code = p.Probe(context.Background())
	if state != ports.ProviderHealthDown || code == nil || *code != "RATE_LIMIT" {
		t.Fatalf("down gateway was reported healthy: state=%s code=%v", state, code)
	}
}

func TestGatewayHealthProbeWithoutSettingsIsUnknown(t *testing.T) {
	state, _, code := (gatewayHealthProbe{}).Probe(context.Background())
	if state != ports.ProviderHealthUnknown || code == nil {
		t.Fatalf("unconfigured gateway probe must be unknown, got %s %v", state, code)
	}
}

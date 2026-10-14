package bootstrap

import (
	"context"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// gatewayHealthProbe uses the already-configured LiteLLM model discovery
// connection tester; it does not issue merchant-bound AI requests.
type gatewayHealthProbe struct {
	settings *ports.LiteLLMPlatformConfig
	discovery ports.ModelDiscoveryClient
}

func (p gatewayHealthProbe) Probe(ctx context.Context) (ports.ProviderHealthState, int64, *string) {
	if p.settings == nil || p.discovery == nil {
		code := "GATEWAY_NOT_CONFIGURED"
		return ports.ProviderHealthUnknown, 0, &code
	}
	ok, latency, code := p.discovery.TestConnection(
		ctx, p.settings.APIKey, p.settings.CustomerModel, p.settings.BaseURL,
	)
	if ok {
		return ports.ProviderHealthHealthy, latency, nil
	}
	if code == "" {
		code = "GATEWAY_PROBE_FAILED"
	}
	return ports.ProviderHealthDown, latency, &code
}

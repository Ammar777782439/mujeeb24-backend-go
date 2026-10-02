package socialapi

import (
	"context"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type healthProbe struct {
	client *Client
}

func NewHealthCheckProbe(client *Client) ports.HealthCheckProbe {
	return &healthProbe{client: client}
}

func (p *healthProbe) Probe(ctx context.Context) (ports.ProviderHealthState, int64, *string) {
	start := time.Now()
	if p == nil || p.client == nil {
		code := "NOT_CONFIGURED"
		return ports.ProviderHealthDown, 0, &code
	}
	if err := p.client.HealthCheck(ctx); err != nil {
		code := "SOCIALAPI_REQUEST_FAILED"
		return ports.ProviderHealthDown, time.Since(start).Nanoseconds(), &code
	}
	return ports.ProviderHealthHealthy, time.Since(start).Nanoseconds(), nil
}

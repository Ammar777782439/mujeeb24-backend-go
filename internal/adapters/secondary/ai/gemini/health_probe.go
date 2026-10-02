package gemini

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type healthProbe struct {
	client *GeminiHTTPClient
}

func NewHealthCheckProbe(client *GeminiHTTPClient) ports.HealthCheckProbe {
	return &healthProbe{client: client}
}

func (p *healthProbe) Probe(ctx context.Context) (ports.ProviderHealthState, int64, *string) {
	start := time.Now()
	if p == nil || p.client == nil {
		code := "NOT_CONFIGURED"
		return ports.ProviderHealthDown, 0, &code
	}
	model := url.PathEscape(strings.TrimSpace(p.client.Model()))
	reqCtx, cancel := context.WithTimeout(ctx, p.client.RequestTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, p.client.BaseURL()+"/v1beta/models/"+model, nil)
	if err != nil {
		code := "REQUEST_BUILD_FAILED"
		return ports.ProviderHealthDown, time.Since(start).Nanoseconds(), &code
	}
	req.Header.Set("x-goog-api-key", p.client.APIKey())
	resp, err := p.client.HTTPClient().Do(req)
	latency := time.Since(start).Nanoseconds()
	if err != nil {
		code := "PROVIDER_UNREACHABLE"
		return ports.ProviderHealthDown, latency, &code
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return ports.ProviderHealthHealthy, latency, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		code := "RATE_LIMITED"
		return ports.ProviderHealthDegraded, latency, &code
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		code := "AUTH_FAILED"
		return ports.ProviderHealthDown, latency, &code
	}
	code := fmt.Sprintf("HTTP_%d", resp.StatusCode)
	if resp.StatusCode >= 500 {
		return ports.ProviderHealthDown, latency, &code
	}
	return ports.ProviderHealthDegraded, latency, &code
}

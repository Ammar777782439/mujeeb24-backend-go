package services

import (
        "context"
        "fmt"
        "net/http"
        "strings"
        "time"

        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// GeminiHealthProbe probes the Google Gemini API by calling the lightweight
// ListModels endpoint (GET /v1beta/models). This endpoint:
//   - Does NOT consume tokens (no generateContent call).
//   - Verifies the API key is valid (returns 200 for valid key).
//   - Verifies the service is reachable (network succeeds).
//   - Returns 401/403 for invalid/revoked keys.
//
// Per Contract §84: the probe uses a standalone request — no merchant data,
// no business_id, no customer data. The probe is a platform-level health check.
type GeminiHealthProbe struct {
        BaseURL string
        APIKey  string
        Model   string
        Client  *http.Client
}

// Compile-time assertion that GeminiHealthProbe satisfies HealthCheckProbe.
var _ ports.HealthCheckProbe = (*GeminiHealthProbe)(nil)

// Probe calls GET {baseURL}/v1beta/models and returns the health state.
func (p *GeminiHealthProbe) Probe(ctx context.Context) (ports.ProviderHealthState, int64, *string) {
        if p == nil || strings.TrimSpace(p.BaseURL) == "" || strings.TrimSpace(p.APIKey) == "" {
                return ports.ProviderHealthUnknown, 0, stringPtr("PROBE_NOT_CONFIGURED")
        }
        client := p.Client
        if client == nil {
                client = &http.Client{Timeout: 10 * time.Second}
        }
        start := time.Now()
        url := fmt.Sprintf("%s/v1beta/models", strings.TrimRight(p.BaseURL, "/"))
        reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
        defer cancel()
        req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
        if err != nil {
                return ports.ProviderHealthDown, time.Since(start).Nanoseconds(), stringPtr("PROBE_REQUEST_BUILD_FAILED")
        }
        req.Header.Set("x-goog-api-key", p.APIKey)
        resp, err := client.Do(req)
        if err != nil {
                return ports.ProviderHealthDown, time.Since(start).Nanoseconds(), stringPtr("PROBE_NETWORK_ERROR")
        }
        defer resp.Body.Close()
        latency := time.Since(start).Nanoseconds()
        switch {
        case resp.StatusCode >= 200 && resp.StatusCode < 300:
                return ports.ProviderHealthHealthy, latency, nil
        case resp.StatusCode == 401 || resp.StatusCode == 403:
                code := "PROBE_AUTH_FAILED"
                return ports.ProviderHealthDown, latency, &code
        case resp.StatusCode == 429:
                code := "PROBE_RATE_LIMITED"
                return ports.ProviderHealthDegraded, latency, &code
        default:
                code := fmt.Sprintf("PROBE_HTTP_%d", resp.StatusCode)
                return ports.ProviderHealthDegraded, latency, &code
        }
}

// SocialAPIHealthProbe probes the SocialAPI service by calling the
// GET /v1/accounts endpoint. This endpoint:
//   - Verifies the API key is valid (returns 200 for valid key).
//   - Verifies the service is reachable (network succeeds).
//   - Returns 401/403 for invalid/revoked keys.
//
// Per Contract §90: SocialAPI is the channel transport provider. The probe
// checks platform-level connectivity, not any merchant's specific connection.
type SocialAPIHealthProbe struct {
        BaseURL string
        APIKey  string
        Client  *http.Client
}

// Compile-time assertion that SocialAPIHealthProbe satisfies HealthCheckProbe.
var _ ports.HealthCheckProbe = (*SocialAPIHealthProbe)(nil)

// Probe calls GET {baseURL}/v1/accounts with Bearer token and returns health.
func (p *SocialAPIHealthProbe) Probe(ctx context.Context) (ports.ProviderHealthState, int64, *string) {
        if p == nil || strings.TrimSpace(p.BaseURL) == "" || strings.TrimSpace(p.APIKey) == "" {
                return ports.ProviderHealthUnknown, 0, stringPtr("PROBE_NOT_CONFIGURED")
        }
        client := p.Client
        if client == nil {
                client = &http.Client{Timeout: 10 * time.Second}
        }
        start := time.Now()
        url := fmt.Sprintf("%s/v1/accounts", strings.TrimRight(p.BaseURL, "/"))
        reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
        defer cancel()
        req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
        if err != nil {
                return ports.ProviderHealthDown, time.Since(start).Nanoseconds(), stringPtr("PROBE_REQUEST_BUILD_FAILED")
        }
        req.Header.Set("Authorization", "Bearer "+p.APIKey)
        resp, err := client.Do(req)
        if err != nil {
                return ports.ProviderHealthDown, time.Since(start).Nanoseconds(), stringPtr("PROBE_NETWORK_ERROR")
        }
        defer resp.Body.Close()
        latency := time.Since(start).Nanoseconds()
        switch {
        case resp.StatusCode >= 200 && resp.StatusCode < 300:
                return ports.ProviderHealthHealthy, latency, nil
        case resp.StatusCode == 401 || resp.StatusCode == 403:
                code := "PROBE_AUTH_FAILED"
                return ports.ProviderHealthDown, latency, &code
        case resp.StatusCode == 429:
                code := "PROBE_RATE_LIMITED"
                return ports.ProviderHealthDegraded, latency, &code
        default:
                code := fmt.Sprintf("PROBE_HTTP_%d", resp.StatusCode)
                return ports.ProviderHealthDegraded, latency, &code
        }
}

// stringPtr is defined in ai_commands.go — reused here (same package).

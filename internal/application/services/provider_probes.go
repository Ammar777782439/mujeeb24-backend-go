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
//
// DYNAMIC CONFIG (per spec §1-5): the probe holds a ConfigProvider (the SAME
// AIConfigurationProvider that the Production Gemini Runtime ContractClient
// uses). At probe time, it calls GetActiveConfig(ctx) to read the current
// active credential + model + base URL. This ensures:
//   - After credential rotation, the probe uses the NEW credential (not the
//     old one captured at bootstrap).
//   - After model switching, the probe reflects the new active model.
//   - The probe NEVER stores a static API key — it reads it fresh on every
//     call + never logs it.
type GeminiHealthProbe struct {
        // ConfigProvider is the source of truth for the active AI configuration.
        // Per §1-2: the probe reads the ACTIVE config at probe time — NOT a
        // static copy from bootstrap. This is the SAME provider wired into the
        // ContractClient (cc.SetConfigurationProvider) + BatchClient + TokenCounter.
        ConfigProvider ports.AIConfigurationProvider
        // Client is the HTTP client used for the probe (optional — defaults to
        // a 10s-timeout client if nil).
        Client *http.Client
}

// Compile-time assertion that GeminiHealthProbe satisfies HealthCheckProbe.
var _ ports.HealthCheckProbe = (*GeminiHealthProbe)(nil)

// Probe calls GET {baseURL}/v1beta/models and returns the health state.
//
// Per spec §1-5: reads the active config at probe time via ConfigProvider.
// Per spec §6: NEVER logs the API key — only logs the model + credential_id
// (which is a safe identifier, not a secret).
func (p *GeminiHealthProbe) Probe(ctx context.Context) (ports.ProviderHealthState, int64, *string) {
        if p == nil || p.ConfigProvider == nil {
                return ports.ProviderHealthUnknown, 0, stringPtr("PROBE_NOT_CONFIGURED")
        }
        // Read the active config at probe time — this is the SAME source the
        // Production Gemini Runtime (ContractClient.resolveConfig) uses.
        cfg, err := p.ConfigProvider.GetActiveConfig(ctx)
        if err != nil {
                return ports.ProviderHealthUnknown, 0, stringPtr("PROBE_CONFIG_READ_FAILED")
        }
        if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" {
                // No active credential configured — the probe cannot run.
                return ports.ProviderHealthUnknown, 0, stringPtr("PROBE_NOT_CONFIGURED")
        }
        client := p.Client
        if client == nil {
                client = &http.Client{Timeout: 10 * time.Second}
        }
        start := time.Now()
        url := fmt.Sprintf("%s/v1beta/models", strings.TrimRight(cfg.BaseURL, "/"))
        reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
        defer cancel()
        req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
        if err != nil {
                return ports.ProviderHealthDown, time.Since(start).Nanoseconds(), stringPtr("PROBE_REQUEST_BUILD_FAILED")
        }
        // Per P1-8: API key via x-goog-api-key header only — never in URL.
        // Per spec §6: the API key is NEVER logged.
        req.Header.Set("x-goog-api-key", cfg.APIKey)
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

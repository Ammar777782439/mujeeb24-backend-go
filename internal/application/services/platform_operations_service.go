package services

import (
        "context"
        "errors"
        "fmt"
        "log"
        "strings"
        "sync"
        "time"

        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// InMemoryPlatformOperationsRepository implements ports.PlatformOperationsPort
// with an in-memory provider registry + AI runtime state.
//
// Per Platform Administration Contract §78-81:
//   - Provider state is in-memory; persistence of state transitions is via
//     platform_audit_events (the caller's responsibility).
//   - Secrets (API keys, etc.) are NEVER stored here — only the `configured`
//     boolean derived from env validation at bootstrap time.
//   - The AI Runtime is the master gate — even when providers are
//     ENABLED + HEALTHY, no AI execution can start with Runtime = DISABLED.
//
// Per Contract §76: the production AI provider is Gemini with model
// gemini-3.1-flash-lite. Per Contract §88: OpenAI-compatible adapter
// exists but is NOT marked as an active production provider — it doesn't
// appear in ListProviders() as active.
type InMemoryPlatformOperationsRepository struct {
        mu          sync.RWMutex
        runtime     ports.AIRuntimeState
        providers   map[string]ports.ProviderRecord
        probes      map[string]ports.HealthCheckProbe
}

// NewInMemoryPlatformOperationsRepository seeds the registry with the
// baseline providers per Contract §76 + §90:
//   - AI: Google Gemini (model: gemini-3.1-flash-lite)
//   - Channel Transport: SocialAPI (Facebook + Instagram + WhatsApp)
//
// `aiConfigured` and `channelConfigured` are determined by the bootstrap
// layer based on whether env vars are present (NEVER the actual values).
func NewInMemoryPlatformOperationsRepository(aiConfigured, channelConfigured bool) *InMemoryPlatformOperationsRepository {
        registry := &InMemoryPlatformOperationsRepository{
                runtime: ports.AIRuntimeState{
                        AdminState:  ports.ProviderAdminEnabled,
                        HealthState: ports.ProviderHealthUnknown,
                },
                providers: map[string]ports.ProviderRecord{},
                probes:    map[string]ports.HealthCheckProbe{},
        }
        // Seed the AI provider (Contract §76)
        registry.providers["google_gemini"] = ports.ProviderRecord{
                ProviderID:   "google_gemini",
                ProviderType: ports.ProviderTypeAI,
                DisplayName:  "Google Gemini",
                AdminState:   ports.ProviderAdminEnabled,
                HealthState:  ports.ProviderHealthUnknown,
                Configured:  aiConfigured,
                Model:        "gemini-3.1-flash-lite",
        }
        // Seed the channel transport provider (Contract §90)
        registry.providers["socialapi"] = ports.ProviderRecord{
                ProviderID:   "socialapi",
                ProviderType: ports.ProviderTypeChannelTransport,
                DisplayName:  "SocialAPI",
                AdminState:   ports.ProviderAdminEnabled,
                HealthState:  ports.ProviderHealthUnknown,
                Configured:  channelConfigured,
        }
        return registry
}

// RegisterProbe attaches a HealthCheckProbe to a provider. Probes are
// implementation-specific (Gemini probe, SocialAPI probe) and are wired
// by the bootstrap layer.
func (r *InMemoryPlatformOperationsRepository) RegisterProbe(providerID string, probe ports.HealthCheckProbe) {
        r.mu.Lock()
        defer r.mu.Unlock()
        if r.probes == nil {
                r.probes = map[string]ports.HealthCheckProbe{}
        }
        r.probes[providerID] = probe
}

// GetRuntimeState returns the current AI runtime master-gate state.
func (r *InMemoryPlatformOperationsRepository) GetRuntimeState(_ context.Context) (ports.AIRuntimeState, error) {
        r.mu.RLock()
        defer r.mu.RUnlock()
        return r.runtime, nil
}

// DisableRuntime flips the AI runtime master gate to DISABLED. Per Contract
// §81: this stops Auto Reply + Automated Sales Decisions + AI-driven
// Automation but preserves Human Replies, Merchant Dashboard, Customer
// Data, Leads, Orders, Channel Reception.
//
// Idempotent — calling Disable on an already-DISABLED runtime is a no-op
// (the audit layer can decide to log it as a no-op or skip).
func (r *InMemoryPlatformOperationsRepository) DisableRuntime(_ context.Context, _ time.Time) (ports.AIRuntimeState, error) {
        r.mu.Lock()
        defer r.mu.Unlock()
        r.runtime.AdminState = ports.ProviderAdminDisabled
        return r.runtime, nil
}

// EnableRuntime flips the AI runtime master gate back to ENABLED.
// Idempotent — calling Enable on an already-ENABLED runtime is a no-op.
func (r *InMemoryPlatformOperationsRepository) EnableRuntime(_ context.Context, _ time.Time) (ports.AIRuntimeState, error) {
        r.mu.Lock()
        defer r.mu.Unlock()
        r.runtime.AdminState = ports.ProviderAdminEnabled
        return r.runtime, nil
}

// ListProviders returns all registered providers. Per Contract §88: the
// OpenAI-compatible adapter is intentionally NOT registered as an active
// provider.
func (r *InMemoryPlatformOperationsRepository) ListProviders(_ context.Context) ([]ports.ProviderRecord, error) {
        r.mu.RLock()
        defer r.mu.RUnlock()
        items := make([]ports.ProviderRecord, 0, len(r.providers))
        for _, p := range r.providers {
                items = append(items, p)
        }
        return items, nil
}

// GetProvider returns a single provider by ID. Returns NotFound if unknown.
func (r *InMemoryPlatformOperationsRepository) GetProvider(_ context.Context, providerID string) (ports.ProviderRecord, error) {
        r.mu.RLock()
        defer r.mu.RUnlock()
        provider, ok := r.providers[strings.TrimSpace(providerID)]
        if !ok {
                return ports.ProviderRecord{}, fmt.Errorf("provider %q not found", providerID)
        }
        return provider, nil
}

// RunHealthCheck invokes the registered probe for a provider + updates the
// provider's health state. Per Contract §84: the probe MUST NOT use any
// merchant data. If no probe is registered, the health state is set to
// UNKNOWN with a failure_code of "NO_PROBE_REGISTERED".
//
// Per agreement: "ANY log should be recorded no matter what" — this method
// logs the probe attempt + the result (success, no-probe-registered, auth
// failure, network error, etc.) to stdout so operators can see every
// health check in the backend logs.
func (r *InMemoryPlatformOperationsRepository) RunHealthCheck(ctx context.Context, providerID string, now time.Time) (ports.ProviderRecord, error) {
        r.mu.Lock()
        defer r.mu.Unlock()
        providerID = strings.TrimSpace(providerID)
        provider, ok := r.providers[providerID]
        if !ok {
                log.Printf("[PlatformOperations] HEALTH_CHECK provider=%s result=NOT_FOUND (provider not in registry)", providerID)
                return ports.ProviderRecord{}, fmt.Errorf("provider %q not found", providerID)
        }
        probe, hasProbe := r.probes[providerID]
        if !hasProbe || probe == nil {
                // No probe registered — the health check cannot run. Log so
                // operators know the probe was never wired (this is a bootstrap
                // gap, not a runtime failure).
                noProbeCode := "NO_PROBE_REGISTERED"
                provider.HealthState = ports.ProviderHealthUnknown
                provider.LastHealthCheck = &now
                provider.LastFailureCode = &noProbeCode
                r.providers[providerID] = provider
                log.Printf("[PlatformOperations] HEALTH_CHECK provider=%s type=%s result=NO_PROBE_REGISTERED (no probe wired for this provider — bootstrap gap)", providerID, provider.ProviderType)
                return provider, nil
        }
        healthState, latencyNanos, failureCode := probe.Probe(ctx)
        provider.LastHealthCheck = &now
        provider.HealthState = healthState
        switch healthState {
        case ports.ProviderHealthHealthy:
                provider.LastSuccess = &now
                provider.LastFailureCode = nil
                log.Printf("[PlatformOperations] HEALTH_CHECK provider=%s type=%s result=HEALTHY latency_ms=%d", providerID, provider.ProviderType, latencyNanos/1e6)
        case ports.ProviderHealthDegraded, ports.ProviderHealthDown:
                provider.LastFailure = &now
                provider.LastFailureCode = failureCode
                code := ""
                if failureCode != nil {
                        code = *failureCode
                }
                log.Printf("[PlatformOperations] HEALTH_CHECK provider=%s type=%s result=%s latency_ms=%d failure_code=%s", providerID, provider.ProviderType, healthState, latencyNanos/1e6, code)
        default:
                log.Printf("[PlatformOperations] HEALTH_CHECK provider=%s type=%s result=%s (unhandled state)", providerID, provider.ProviderType, healthState)
        }
        r.providers[providerID] = provider
        return provider, nil
}

// Compile-time assertion that the in-memory implementation satisfies the port.
var _ ports.PlatformOperationsPort = (*InMemoryPlatformOperationsRepository)(nil)

// ErrProviderNotFound is returned by GetProvider / RunHealthCheck when the
// requested providerID is not in the registry.
var ErrProviderNotFound = errors.New("provider not found")

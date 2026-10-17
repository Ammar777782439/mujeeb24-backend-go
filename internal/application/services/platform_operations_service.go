package services

import (
	"context"
	"errors"
	"fmt"
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
// The AI provider is the native Gemini and DeepSeek gateway; upstream model routing belongs to native Gemini and DeepSeek.
type InMemoryPlatformOperationsRepository struct {
	mu        sync.RWMutex
	runtime   ports.AIRuntimeState
	providers map[string]ports.ProviderRecord
	probes    map[string]ports.HealthCheckProbe
}

// NewInMemoryPlatformOperationsRepository seeds the registry with the
// active deployment providers:
//   - AI: Gemini + DeepSeek native providers (configured customer model alias)
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
	for _, ai := range []struct{id,name string}{
		{"gemini","Google Gemini primary"},
		{"deepseek","DeepSeek standby"},
	} {
		registry.providers[ai.id] = ports.ProviderRecord{
			ProviderID:ai.id, ProviderType:ports.ProviderTypeAI,
			DisplayName:ai.name,AdminState:ports.ProviderAdminEnabled,
			HealthState:ports.ProviderHealthUnknown,Configured:aiConfigured,
		}
	}
	// Seed the channel transport provider (Contract §90)
	registry.providers["socialapi"] = ports.ProviderRecord{
		ProviderID:   "socialapi",
		ProviderType: ports.ProviderTypeChannelTransport,
		DisplayName:  "SocialAPI",
		AdminState:   ports.ProviderAdminEnabled,
		HealthState:  ports.ProviderHealthUnknown,
		Configured:   channelConfigured,
	}
	return registry
}

// RegisterProbe attaches a HealthCheckProbe to a provider. Probes are
// implementation-specific (native Gemini and DeepSeek probe, SocialAPI probe) and are wired
// by the bootstrap layer.
// SetAIModel reports the deployment-defined customer route alias without
// implying that any single upstream provider is permanently selected.
func (r *InMemoryPlatformOperationsRepository) SetAIModel(model string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.providers["gemini"]
	p.Model = strings.TrimSpace(model)
	r.providers["gemini"] = p
}

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
func (r *InMemoryPlatformOperationsRepository) RunHealthCheck(ctx context.Context, providerID string, now time.Time) (ports.ProviderRecord, error) {
	providerID = strings.TrimSpace(providerID)
	r.mu.RLock()
	_, exists := r.providers[providerID]
	probe := r.probes[providerID]
	r.mu.RUnlock()
	if !exists {
		return ports.ProviderRecord{}, fmt.Errorf("provider %q not found", providerID)
	}

	// Network I/O must NEVER hold the runtime registry lock: an emergency AI
	// disable must work even when the native Gemini and DeepSeek provider is timing out.
	healthState := ports.ProviderHealthUnknown
	var failureCode *string
	if probe == nil {
		code := "NO_PROBE_REGISTERED"
		failureCode = &code
	} else {
		var latencyMs int64
		healthState, latencyMs, failureCode = probe.Probe(ctx)
		_ = latencyMs // Reserved for dashboard latency telemetry.
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	provider, ok := r.providers[providerID]
	if !ok {
		return ports.ProviderRecord{}, fmt.Errorf("provider %q not found", providerID)
	}
	// Do not overwrite a more recent concurrent health check.
	if provider.LastHealthCheck != nil && provider.LastHealthCheck.After(now) {
		return provider, nil
	}
	provider.LastHealthCheck = &now
	provider.HealthState = healthState
	switch healthState {
	case ports.ProviderHealthHealthy:
		provider.LastSuccess = &now
		provider.LastFailureCode = nil
	case ports.ProviderHealthDegraded, ports.ProviderHealthDown:
		provider.LastFailure = &now
		provider.LastFailureCode = failureCode
	default:
		provider.LastFailureCode = failureCode
	}
	r.providers[providerID] = provider
	return provider, nil
}

// Compile-time assertion that the in-memory implementation satisfies the port.
var _ ports.PlatformOperationsPort = (*InMemoryPlatformOperationsRepository)(nil)

// ErrProviderNotFound is returned by GetProvider / RunHealthCheck when the
// requested providerID is not in the registry.
var ErrProviderNotFound = errors.New("provider not found")

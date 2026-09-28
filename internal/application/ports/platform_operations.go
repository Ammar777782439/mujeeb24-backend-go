package ports

import (
	"context"
	"time"
)

// ----------------------------------------------------------------------------
// Platform AI Operations — Contract §73-108
// ----------------------------------------------------------------------------
//
// Contract rules (CLOSED):
//
//   §75 — /admin/ai view exposes: AI Runtime + AI Provider + Model +
//         Health + Usage + Failures.
//
//   §76 — Production AI Provider: Google Gemini. Model: gemini-3.1-flash-lite.
//
//   §77 — Layered architecture: Mujeeb AI Runtime → AI Provider Port →
//         Gemini Adapter → Google Gemini. The Domain does NOT bind directly
//         to Gemini.
//
//   §78 — Provider Registry: provider_id, provider_type, display_name,
//         enabled, health_state, last_health_check, last_success,
//         last_failure. The Dashboard NEVER stores API Key / Secret /
//         Private Credential.
//
//   §79 — Two independent state dimensions:
//          - Administrative State: ENABLED | DISABLED
//          - Health State: HEALTHY | DEGRADED | DOWN | UNKNOWN
//          DISABLED ≠ DOWN — DISABLED is admin-intent; DOWN is observed
//          operational failure. They can coexist (e.g., admin disabled +
//          health = UNKNOWN).
//
//   §80 — AI Runtime has its own ENABLED | DISABLED + HEALTHY | DEGRADED |
//         DOWN | UNKNOWN. The Runtime is the master gate — even if a
//         provider is ENABLED + HEALTHY, AI executions cannot start when
//         the Runtime is DISABLED.
//
//   §81 — Kill Switch: Platform Admin can Disable/Enable the AI Runtime.
//          DISABLED stops Auto Reply + Automated Sales Decisions +
//          AI-driven Automation. It does NOT stop Human Replies, the
//          Merchant Dashboard, Customer Data, Leads, Orders, or Channel
//          Reception. Every Disable/Enable is Platform-Audited.
//
//   §82 — Platform Admin sees aggregated usage on the platform level:
//          AI Replies + Model Requests + Provider Requests + Tool Calls +
//          Input Usage + Output Usage + Failures. This is Platform
//          Operational Usage — separate from Merchant Billing Usage (which
//          bills per Final AI Reply per §31).
//
//   §83 — Monitoring shows: counts / rates / latency / errors / provider+model.
//          NEVER: customer message, full AI prompt, private business
//          knowledge, merchant secret.
//
//   §84 — Health Check uses a standalone probe — does NOT depend on a
//          merchant conversation. Never uses merchant_id / business_id /
//          customer data / merchant catalog.
//
//   §86 — V1 does NOT allow admin to switch models from the dashboard.
//          Model selection is a Deployment Configuration Change.
//
//   §88 — OpenAI-Compatible adapter exists but is NOT an active production
//          AI provider. Dashboard shows ONLY Gemini as ACTIVE.
//
//   §102 — Operational Events audited: ai.disabled, ai.enabled,
//          provider.health_checked, channel.health_checked,
//          platform.channel_action.
//
//   §104 — Provider Secrets Boundary: Gemini API Key, SocialAPI Secret,
//          Signing Keys, DB Credentials are NEVER exposed via Platform UI.
//          Dashboard sees only `configured = true` (boolean), not the secret.
// ----------------------------------------------------------------------------

// ProviderType is the category of external dependency.
type ProviderType string

const (
	ProviderTypeAI              ProviderType = "AI"
	ProviderTypeChannelTransport ProviderType = "CHANNEL_TRANSPORT"
)

// ProviderAdminState is the administrative on/off switch (Contract §79).
type ProviderAdminState string

const (
	ProviderAdminEnabled  ProviderAdminState = "ENABLED"
	ProviderAdminDisabled ProviderAdminState = "DISABLED"
)

// ProviderHealthState is the observed health (Contract §79).
type ProviderHealthState string

const (
	ProviderHealthHealthy   ProviderHealthState = "HEALTHY"
	ProviderHealthDegraded  ProviderHealthState = "DEGRADED"
	ProviderHealthDown      ProviderHealthState = "DOWN"
	ProviderHealthUnknown  ProviderHealthState = "UNKNOWN"
)

// ProviderRecord is the in-memory provider registry entry. No API Key /
// Secret / Private Credential is stored here — only `configured` (boolean).
type ProviderRecord struct {
	ProviderID         string
	ProviderType       ProviderType
	DisplayName        string
	AdminState         ProviderAdminState
	HealthState        ProviderHealthState
	Configured         bool // true if a secret exists in env / secrets manager
	LastHealthCheck    *time.Time
	LastSuccess        *time.Time
	LastFailure        *time.Time
	LastFailureCode    *string
	Model              string // for AI providers — empty for transport
}

// AIRuntimeState is the runtime-level master gate (Contract §80).
type AIRuntimeState struct {
	AdminState  ProviderAdminState
	HealthState ProviderHealthState
}

// HealthCheckProbe is the interface for performing a standalone health check
// against a provider. Per Contract §84: the probe MUST NOT use merchant data.
//
// Implementations:
//   - AIHealthProbe: sends a minimal prompt to Gemini with no business context.
//   - ChannelHealthProbe: verifies SocialAPI reachability.
type HealthCheckProbe interface {
	// Probe returns the observed health state + latency + failure code (if any).
	Probe(ctx context.Context) (ProviderHealthState, int64, *string)
}

// PlatformOperationsPort is the Platform Operations port. It manages the
// in-memory provider registry + AI runtime state. State changes are
// audited via the existing PlatformAuditRepository (caller's responsibility).
//
// Per Contract §81: every Disable/Enable + every health check is audited.
type PlatformOperationsPort interface {
	// AI Runtime
	GetRuntimeState(ctx context.Context) (AIRuntimeState, error)
	DisableRuntime(ctx context.Context, now time.Time) (AIRuntimeState, error)
	EnableRuntime(ctx context.Context, now time.Time) (AIRuntimeState, error)

	// Providers
	ListProviders(ctx context.Context) ([]ProviderRecord, error)
	GetProvider(ctx context.Context, providerID string) (ProviderRecord, error)
	RunHealthCheck(ctx context.Context, providerID string, now time.Time) (ProviderRecord, error)
}

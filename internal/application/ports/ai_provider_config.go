package ports

import (
        "context"
        "time"
)

// ----------------------------------------------------------------------------
// AI Provider Configuration — dynamic runtime configuration
// Per Platform Administration Contract §75-88 + AIUsageTokenTelemetry.md
// ----------------------------------------------------------------------------

// AIActiveConfig is the runtime-facing configuration that the Gemini client
// reads at call time. This replaces the static struct fields (apiKey, model,
// maxOutputTokens, maxInputCharacters) that were set at bootstrap from env.
//
// Per §2: the runtime gets the active provider credential from Configuration
// abstraction, not from a hardcoded env var.
//
// Per §6: effective_max_output_tokens = min(Mujeeb operational limit, provider
// model output limit). The runtime must NOT send maxOutputTokens larger than
// the model's capability.
type AIActiveConfig struct {
        Provider              string
        Model                 string
        APIKey                string // NEVER returned to frontend
        BaseURL               string
        MaxOutputTokens       int // effective = min(Mujeeb, Provider)
        MaxInputCharacters    int // Mujeeb operational limit
        PricingVersion        string
        CredentialStatus      string // CONFIGURED / VALID / INVALID / UNKNOWN
        CredentialID          string
        ConfigurationVersion  int
}

// AIConfigurationProvider is the runtime-facing interface that the Gemini
// ContractClient + BatchClient use to get the active configuration at call
// time. Per §9: the cache is in-memory; invalidation happens on config change.
//
// Per §2: absence of a valid credential must prevent AI execution with a
// clear error message, not a panic.
type AIConfigurationProvider interface {
        GetActiveConfig(ctx context.Context) (AIActiveConfig, error)
        Invalidate()
}

// ----------------------------------------------------------------------------
// Credential Management (§1)
// ----------------------------------------------------------------------------

type AICredentialRecord struct {
        ID              string
        Provider        string
        DisplayName    string
        KeyHint        string // last 4 chars only — never the full key
        Status         string // CONFIGURED / VALID / INVALID / REVOKED
        ValidatedAt    *time.Time
        ValidationError *string
        CreatedBy      string
        CreatedAt      time.Time
        UpdatedAt      time.Time
        RevokedAt      *time.Time
}

type AICredentialCreate struct {
        ID           string
        Provider     string
        DisplayName string
        EncryptedKey string
        KeyHint      string
        CreatedBy    string
        Now          time.Time
}

type AICredentialRepository interface {
        StoreCredential(ctx context.Context, create AICredentialCreate) (AICredentialRecord, error)
        GetActiveCredential(ctx context.Context, provider string) (AICredentialRecord, string, error) // returns (record, decryptedKey, error)
        GetCredentialByID(ctx context.Context, id string) (AICredentialRecord, error)
        UpdateCredentialStatus(ctx context.Context, id, status string, validationError *string, now time.Time) (AICredentialRecord, error)
        RevokeCredential(ctx context.Context, id string, now time.Time) error
        ListCredentials(ctx context.Context, provider string) ([]AICredentialRecord, error)
}

// ----------------------------------------------------------------------------
// Model Discovery (§3)
// ----------------------------------------------------------------------------

type AIProviderModel struct {
        ID                 string
        Provider           string
        ModelName          string
        DisplayName        *string
        Description        *string
        InputTokenLimit    *int
        OutputTokenLimit   *int
        SupportedMethods   []string
        ThinkingSupported  bool
        TemperatureMin     *float64
        TemperatureMax     *float64
        TopPMin            *float64
        TopPMax            *float64
        TopKMin            *int
        TopKMax            *int
        Version            *string
        BaseModel          *string
        DiscoveredAt       time.Time
}

type ModelDiscoveryClient interface {
        DiscoverModels(ctx context.Context, apiKey, baseURL string) ([]AIProviderModel, error)
}

type AIModelRepository interface {
        UpsertDiscoveredModels(ctx context.Context, models []AIProviderModel) error
        ListModels(ctx context.Context, provider string) ([]AIProviderModel, error)
        GetModel(ctx context.Context, provider, modelName string) (AIProviderModel, error)
}

// ----------------------------------------------------------------------------
// Configuration Versioning (§5)
// ----------------------------------------------------------------------------

type AIConfigurationVersion struct {
        ID                      string
        Version                 int
        Provider                string
        Model                   string
        CredentialID            string
        MujeebMaxInputChars     int
        MujeebMaxOutputTokens   int
        EffectiveMaxOutputTokens int
        PricingVersion          *string
        Status                  string // DRAFT / ACTIVE / DEACTIVATED
        CreatedBy               string
        CreatedAt               time.Time
        ActivatedAt             *time.Time
        DeactivatedAt           *time.Time
        Reason                  *string
}

type AIConfigurationCreate struct {
        ID                    string
        Provider              string
        Model                 string
        CredentialID          string
        MujeebMaxInputChars   int
        MujeebMaxOutputTokens int
        PricingVersion        *string
        CreatedBy             string
        Now                   time.Time
}

type AIConfigurationRepository interface {
        CreateVersion(ctx context.Context, create AIConfigurationCreate) (AIConfigurationVersion, error)
        ActivateVersion(ctx context.Context, id string, now time.Time) (AIConfigurationVersion, error)
        GetActiveVersion(ctx context.Context, provider string) (AIConfigurationVersion, error)
        GetVersionByID(ctx context.Context, id string) (AIConfigurationVersion, error)
        ListVersions(ctx context.Context, provider string, limit int) ([]AIConfigurationVersion, error)
}

// AIProviderConfigService is the combined interface that all three AI
// provider config repositories implement. Used in PlatformDeps so the
// handler facades can access credential, model, and configuration methods
// through a single field.
type AIProviderConfigService interface {
        AICredentialRepository
        AIModelRepository
        AIConfigurationRepository
}

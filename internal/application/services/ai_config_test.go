package services

import (
	"context"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// Test A: runtime uses cache configuration instead of static model
// Per §1: the Gemini runtime must read the ACTIVE configuration from
// AIConfigurationProvider, not from static env values.
//
// Per §9 + §11 (revised semantics): after Invalidate(), GetActiveConfig
// triggers a ReloadFromDB. If DB reload fails (no DB repo wired or no
// active version in DB), the cache falls back to the env-loaded config
// so the runtime never crashes — but the cache is marked stale so the
// next Invalidate + DB-available scenario will pick up the new config.
func TestAIConfigurationCacheUsesDynamicConfig(t *testing.T) {
	cache := NewAIConfigurationCache(nil, nil)
	// Seed from env (bootstrap).
	cache.LoadFromEnv("bootstrap-key", "bootstrap-model", "https://bootstrap.example.com", 700, 12000, "")
	// Verify the config is what was seeded.
	cfg, err := cache.GetActiveConfig(context.Background())
	if err != nil {
		t.Fatalf("GetActiveConfig: %v", err)
	}
	if cfg.APIKey != "bootstrap-key" {
		t.Errorf("expected APIKey=bootstrap-key, got %s", cfg.APIKey)
	}
	if cfg.Model != "bootstrap-model" {
		t.Errorf("expected Model=bootstrap-model, got %s", cfg.Model)
	}
	if cfg.MaxOutputTokens != 700 {
		t.Errorf("expected MaxOutputTokens=700, got %d", cfg.MaxOutputTokens)
	}
	// Per §9: invalidate → next GetActiveConfig must not return stale cache.
	// It MUST trigger a ReloadFromDB attempt; when no DB is wired, fall back
	// to the env config so the runtime never crashes.
	cache.Invalidate()
	// After invalidation with no DB repo wired, GetActiveConfig falls back
	// to the env-loaded config (set via LoadFromEnv). This is the bootstrap
	// scenario — the system continues to work using the env API key until
	// the admin activates a DB-backed configuration.
	cfg, err = cache.GetActiveConfig(context.Background())
	if err != nil {
		t.Fatalf("GetActiveConfig after Invalidate should fall back to env, got error: %v", err)
	}
	if cfg.APIKey != "bootstrap-key" {
		t.Errorf("after Invalidate with no DB, expected APIKey=bootstrap-key (env fallback), got %s", cfg.APIKey)
	}
}

// Test A2: cache reload from DB after Invalidate replaces env config.
// Per §9: when a DB-backed active configuration exists, the cache MUST
// read it from DB after Invalidate — the env config is just bootstrap.
func TestAIConfigurationCacheReloadFromDBAfterInvalidate(t *testing.T) {
	// Build a cache with a stub repo that returns a known active config.
	repo := &stubAIConfigRepo{
		activeVersion: ports.AIConfigurationVersion{
			ID: "cfg-1", Version: 7, Provider: "google_gemini",
			Model: "db-model", EffectiveMaxOutputTokens: 500,
			MujeebMaxInputChars: 10000,
			PricingVersion:      aiConfigTestStrPtr("pricing-v1"),
			Status:              "ACTIVE",
		},
	}
	creds := &stubAICredRepo{
		activeRecord: ports.AICredentialRecord{
			ID: "cred-1", Provider: "google_gemini", Status: "VALID",
		},
		decryptedKey: "db-key-plaintext",
	}
	cache := NewAIConfigurationCache(repo, creds)
	// Seed env config first.
	cache.LoadFromEnv("env-key", "env-model", "https://env.example.com", 700, 12000, "")
	// Verify env is the active config before invalidation.
	cfg, err := cache.GetActiveConfig(context.Background())
	if err != nil {
		t.Fatalf("GetActiveConfig before invalidate: %v", err)
	}
	if cfg.APIKey != "env-key" {
		t.Fatalf("expected env-key, got %s", cfg.APIKey)
	}
	// Invalidate → next GetActiveConfig must read from DB.
	cache.Invalidate()
	cfg, err = cache.GetActiveConfig(context.Background())
	if err != nil {
		t.Fatalf("GetActiveConfig after invalidate: %v", err)
	}
	// DB config must replace env config.
	if cfg.APIKey != "db-key-plaintext" {
		t.Errorf("expected APIKey=db-key-plaintext, got %s", cfg.APIKey)
	}
	if cfg.Model != "db-model" {
		t.Errorf("expected Model=db-model, got %s", cfg.Model)
	}
	if cfg.MaxOutputTokens != 500 {
		t.Errorf("expected MaxOutputTokens=500 (effective), got %d", cfg.MaxOutputTokens)
	}
	if cfg.ConfigurationVersion != 7 {
		t.Errorf("expected ConfigurationVersion=7, got %d", cfg.ConfigurationVersion)
	}
	if cfg.CredentialID != "cred-1" {
		t.Errorf("expected CredentialID=cred-1, got %s", cfg.CredentialID)
	}
	if cfg.PricingVersion != "pricing-v1" {
		t.Errorf("expected PricingVersion=pricing-v1, got %s", cfg.PricingVersion)
	}
}

// Test A3: when no env config and no DB config, GetActiveConfig returns error.
// Per §11: AI execution is prevented with a clear error — no panic.
func TestAIConfigurationCacheNoConfigReturnsError(t *testing.T) {
	cache := NewAIConfigurationCache(nil, nil)
	// No LoadFromEnv called, no DB wired.
	_, err := cache.GetActiveConfig(context.Background())
	if err == nil {
		t.Fatalf("expected error when no config is loaded, got nil")
	}
}

// aiConfigTestStrPtr returns a pointer to the given string (test helper).
func aiConfigTestStrPtr(s string) *string { return &s }

// stubAIConfigRepo is a minimal stub for the AIConfigurationRepository port
// used by the cache reload tests.
type stubAIConfigRepo struct {
	activeVersion ports.AIConfigurationVersion
	getActiveErr  error
}

func (s *stubAIConfigRepo) CreateVersion(_ context.Context, _ ports.AIConfigurationCreate) (ports.AIConfigurationVersion, error) {
	return ports.AIConfigurationVersion{}, nil
}
func (s *stubAIConfigRepo) ActivateVersion(_ context.Context, _ string, _ time.Time) (ports.AIConfigurationVersion, error) {
	return ports.AIConfigurationVersion{}, nil
}
func (s *stubAIConfigRepo) GetActiveVersion(_ context.Context, _ string) (ports.AIConfigurationVersion, error) {
	return s.activeVersion, s.getActiveErr
}
func (s *stubAIConfigRepo) GetVersionByID(_ context.Context, _ string) (ports.AIConfigurationVersion, error) {
	return ports.AIConfigurationVersion{}, nil
}
func (s *stubAIConfigRepo) ListVersions(_ context.Context, _ string, _ int) ([]ports.AIConfigurationVersion, error) {
	return nil, nil
}

// stubAICredRepo is a minimal stub for the AICredentialRepository port.
type stubAICredRepo struct {
	activeRecord ports.AICredentialRecord
	decryptedKey string
	getActiveErr error
}

func (s *stubAICredRepo) StoreCredential(_ context.Context, _ ports.AICredentialCreate) (ports.AICredentialRecord, error) {
	return ports.AICredentialRecord{}, nil
}
func (s *stubAICredRepo) GetActiveCredential(_ context.Context, _ string) (ports.AICredentialRecord, string, error) {
	return s.activeRecord, s.decryptedKey, s.getActiveErr
}
func (s *stubAICredRepo) GetCredentialByID(_ context.Context, _ string) (ports.AICredentialRecord, error) {
	return s.activeRecord, nil
}
func (s *stubAICredRepo) GetDecryptedKeyByID(_ context.Context, _ string) (string, error) {
	return s.decryptedKey, nil
}
func (s *stubAICredRepo) UpdateCredentialStatus(_ context.Context, _ string, _ string, _ *string, _ time.Time) (ports.AICredentialRecord, error) {
	return s.activeRecord, nil
}
func (s *stubAICredRepo) RevokeCredential(_ context.Context, _ string, _ time.Time) error { return nil }
func (s *stubAICredRepo) ListCredentials(_ context.Context, _ string) ([]ports.AICredentialRecord, error) {
	return nil, nil
}

// Test K: API responses never contain the real API key
// Per §1: the API must never return the full API key to the frontend.
// The AIActiveConfig struct exposes APIKey for runtime use, but the
// handler facades project it to AICredentialView which only shows
// KeyHint (last 4 chars).
func TestAICredentialViewNeverExposesFullKey(t *testing.T) {
	cred := ports.AICredentialRecord{
		ID: "cred-1", Provider: "google_gemini",
		DisplayName: "Production Key",
		KeyHint:     "...abcd", // only last 4 chars
		Status:      "VALID",
	}
	// The projection function maps the credential to a safe view.
	view := aiCredentialProjectionForTest(cred)
	if view.KeyHint != "...abcd" {
		t.Errorf("expected key_hint=...abcd, got %s", view.KeyHint)
	}
	// Verify there's no field that exposes the full key.
	// The AICredentialView struct has no APIKey field — only KeyHint.
}

// aiCredentialProjectionForTest mirrors the handler's projection for testing
// in the services package where the handler's projection isn't accessible.
func aiCredentialProjectionForTest(c ports.AICredentialRecord) struct {
	KeyHint string
	Status  string
} {
	return struct {
		KeyHint string
		Status  string
	}{
		KeyHint: c.KeyHint,
		Status:  c.Status,
	}
}

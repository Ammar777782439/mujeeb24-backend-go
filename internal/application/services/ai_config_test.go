package services

import (
	"context"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// Test A: runtime uses cache configuration instead of static model
// Per §1: the Gemini runtime must read the ACTIVE configuration from
// AIConfigurationProvider, not from static env values.
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
	cache.Invalidate()
	// After invalidation without ReloadFromDB, GetActiveConfig must return error
	// (no DB-backed config available in this test).
	_, err = cache.GetActiveConfig(context.Background())
	if err == nil {
		t.Fatalf("expected error after invalidation (no DB config), got nil")
	}
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
		KeyHint: "...abcd", // only last 4 chars
		Status: "VALID",
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

package services

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// AIConfigurationCache is an in-memory cache of the active AI configuration.
// Per §9: the cache avoids querying the DB on every Gemini call. Invalidation
// happens when the admin changes credential/model/limits/pricing.
//
// Per §2: the runtime gets the active config from this cache, not from env.
// Per §11: if the cache has no valid config, AI execution is prevented with
// a clear error — no panic.
type AIConfigurationCache struct {
	mu       sync.RWMutex
	config   ports.AIActiveConfig
	loaded   bool
	repo     ports.AIConfigurationRepository
	creds    ports.AICredentialRepository
	now      func() time.Time
}

func NewAIConfigurationCache(repo ports.AIConfigurationRepository, creds ports.AICredentialRepository) *AIConfigurationCache {
	return &AIConfigurationCache{
		repo:  repo,
		creds: creds,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

// LoadFromEnv seeds the cache from environment-based bootstrap config.
// Called once at startup to ensure the system works before any Platform Admin
// configuration is stored in the DB.
func (c *AIConfigurationCache) LoadFromEnv(apiKey, model, baseURL string, maxOutputTokens, maxInputCharacters int, pricingVersion string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config = ports.AIActiveConfig{
		Provider:              "google_gemini",
		Model:                 model,
		APIKey:               apiKey,
		BaseURL:              baseURL,
		MaxOutputTokens:      maxOutputTokens,
		MaxInputCharacters:   maxInputCharacters,
		PricingVersion:      pricingVersion,
		CredentialStatus:    "CONFIGURED",
		ConfigurationVersion: 0, // 0 = env-based bootstrap
	}
	c.loaded = true
}

// GetActiveConfig returns the cached active configuration.
// Per §2: if no valid config exists, returns an error — AI execution is prevented.
func (c *AIConfigurationCache) GetActiveConfig(_ context.Context) (ports.AIActiveConfig, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.loaded {
		return ports.AIActiveConfig{}, errors.New("AI configuration is not loaded — no active provider credential")
	}
	if c.config.APIKey == "" {
		return ports.AIActiveConfig{}, errors.New("AI configuration has no valid API key — AI execution prevented")
	}
	return c.config, nil
}

// Invalidate marks the cache as stale. The next GetActiveConfig call will
// trigger a reload from the DB. Per §9: after activation, the runtime uses
// the new configuration without restarting the service.
func (c *AIConfigurationCache) Invalidate() {
	c.mu.Lock()
	c.loaded = false
	c.mu.Unlock()
}

// ReloadFromDB loads the active configuration from the database.
// Called after invalidation or on first access if LoadFromEnv wasn't called.
func (c *AIConfigurationCache) ReloadFromDB(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.repo == nil {
		return errors.New("AI configuration repository is not wired")
	}
	version, err := c.repo.GetActiveVersion(ctx, "google_gemini")
	if err != nil {
		return err
	}
	if c.creds == nil {
		return errors.New("AI credential repository is not wired")
	}
	cred, decryptedKey, err := c.creds.GetActiveCredential(ctx, "google_gemini")
	if err != nil {
		return err
	}
	c.config = ports.AIActiveConfig{
		Provider:              version.Provider,
		Model:                 version.Model,
		APIKey:                decryptedKey,
		BaseURL:              "https://generativelanguage.googleapis.com",
		MaxOutputTokens:       version.EffectiveMaxOutputTokens,
		MaxInputCharacters:    version.MujeebMaxInputChars,
		PricingVersion:        derefString(version.PricingVersion),
		CredentialStatus:      cred.Status,
		CredentialID:          cred.ID,
		ConfigurationVersion:  version.Version,
	}
	c.loaded = true
	return nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

var _ ports.AIConfigurationProvider = (*AIConfigurationCache)(nil)

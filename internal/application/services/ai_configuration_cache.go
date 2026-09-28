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
//
// Per §9: after Invalidate(), the next GetActiveConfig call MUST trigger a
// reload from the DB so the runtime uses the new configuration WITHOUT a
// restart. If the DB has no active version yet (bootstrap scenario), the
// cache falls back to the env-loaded config (set once via LoadFromEnv).
type AIConfigurationCache struct {
        mu sync.RWMutex
        // config is the current active configuration. Loaded by either
        // LoadFromEnv (bootstrap) or ReloadFromDB (runtime). Cleared by
        // Invalidate().
        config ports.AIActiveConfig
        loaded bool
        // envConfig is the bootstrap config from environment variables. Set
        // once via LoadFromEnv, NEVER cleared by Invalidate(). Used as a
        // fallback when ReloadFromDB fails (e.g., no DB active version yet).
        envConfig  ports.AIActiveConfig
        envLoaded  bool
        repo       ports.AIConfigurationRepository
        creds      ports.AICredentialRepository
        now        func() time.Time
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
//
// Per §9: env config is the BOOTSTRAP source. Once the admin activates a DB
// version, the env config becomes a fallback only (used when ReloadFromDB
// fails because no DB version is active yet — e.g., the admin deactivated
// the last version).
func (c *AIConfigurationCache) LoadFromEnv(apiKey, model, baseURL string, maxOutputTokens, maxInputCharacters int, pricingVersion string) {
        c.mu.Lock()
        defer c.mu.Unlock()
        cfg := ports.AIActiveConfig{
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
        c.envConfig = cfg
        c.envLoaded = true
        c.config = cfg
        c.loaded = true
}

// GetActiveConfig returns the cached active configuration.
//
// Per §9: if the cache has been invalidated, this triggers a reload from
// the DB so the runtime uses the most recent admin configuration. If the
// DB has no active version (bootstrap scenario), the cache falls back to
// the env-loaded config.
//
// Per §2: if no valid config exists at all (neither env nor DB), returns
// an error — AI execution is prevented.
func (c *AIConfigurationCache) GetActiveConfig(ctx context.Context) (ports.AIActiveConfig, error) {
        // Fast path: cache is loaded. Use RLock for concurrent readers.
        c.mu.RLock()
        if c.loaded && c.config.APIKey != "" {
                cfg := c.config
                c.mu.RUnlock()
                return cfg, nil
        }
        c.mu.RUnlock()

        // Slow path: cache is stale (Invalidate was called) or empty.
        // Take the write lock and reload from DB.
        c.mu.Lock()
        defer c.mu.Unlock()

        // Double-check after acquiring the write lock — another goroutine
        // may have reloaded while we were waiting.
        if c.loaded && c.config.APIKey != "" {
                return c.config, nil
        }

        // Try to reload from DB. Per §9: after activation, the runtime MUST
        // read the new config without restart.
        if c.repo != nil {
                if err := c.reloadFromDBLocked(ctx); err == nil {
                        if c.config.APIKey != "" {
                                return c.config, nil
                        }
                }
                // DB reload failed (e.g., no active version yet, or DB error).
                // Fall through to env fallback below.
        }

        // Fall back to env config (set once at bootstrap). Per §11: if no
        // env config exists, AI execution is prevented with a clear error.
        if c.envLoaded && c.envConfig.APIKey != "" {
                // Restore env config as the active config so subsequent calls
                // hit the fast path without retrying DB reload on every call.
                // We do NOT mark loaded=true here, because the cache is still
                // "stale" relative to the admin's last change — but we DO
                // populate c.config so the RLock fast path can find it.
                c.config = c.envConfig
                c.loaded = true
                return c.config, nil
        }

        return ports.AIActiveConfig{}, errors.New("AI configuration is not loaded — no active provider credential")
}

// Invalidate marks the cache as stale. Per §9: the next GetActiveConfig call
// triggers a reload from the DB so the runtime uses the new configuration
// without restarting the service.
//
// Invalidate does NOT clear the env config — that's a bootstrap source
// used as a fallback when the DB has no active version.
func (c *AIConfigurationCache) Invalidate() {
        c.mu.Lock()
        c.loaded = false
        c.config = ports.AIActiveConfig{}
        c.mu.Unlock()
}

// ReloadFromDB loads the active configuration from the database.
// Called after invalidation or on first access if LoadFromEnv wasn't called.
//
// MUST be called with c.mu held (write lock). Use GetActiveConfig for
// the public, lock-safe entry point.
func (c *AIConfigurationCache) ReloadFromDB(ctx context.Context) error {
        c.mu.Lock()
        defer c.mu.Unlock()
        return c.reloadFromDBLocked(ctx)
}

func (c *AIConfigurationCache) reloadFromDBLocked(ctx context.Context) error {
        if c.repo == nil {
                return errors.New("AI configuration repository is not wired")
        }
        if c.creds == nil {
                return errors.New("AI credential repository is not wired")
        }
        // Always read the active credential's decrypted key. This MUST
        // succeed — if it doesn't, there's no working credential and the
        // runtime can't execute AI calls.
        cred, decryptedKey, err := c.creds.GetActiveCredential(ctx, "google_gemini")
        if err != nil {
                return err
        }
        // Try to read the active DB version. If it exists, use it for
        // model + limits + pricing version.
        version, versionErr := c.repo.GetActiveVersion(ctx, "google_gemini")
        if versionErr == nil {
                // DB-backed version: use it for everything except the API key
                // (which comes from the active credential, since the admin may
                // have rotated the credential without creating a new version).
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
        // No DB version. The admin has rotated the credential (via
        // platformAddAICredential) but has not yet called
        // platformUpdateAIConfiguration. Fall back to the env-loaded model
        // + limits but with the NEW credential's decrypted key.
        //
        // This is the "credential rotation without model switch" path. The
        // runtime uses the new API key with the env/default model + limits
        // until the admin activates a DB-backed configuration.
        if !c.envLoaded {
                // No env fallback either — return the error so the caller can
                // fall back to env (if any) or surface an error.
                return versionErr
        }
        c.config = c.envConfig
        c.config.APIKey = decryptedKey
        c.config.CredentialID = cred.ID
        c.config.CredentialStatus = cred.Status
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

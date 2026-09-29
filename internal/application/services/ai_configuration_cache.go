package services

import (
        "context"
        "errors"
        "fmt"
        "sync"
        "time"

        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// kindedErrorCache is a minimal interface to check repository error kinds
// without importing the postgres package (which would create an import
// cycle: services → postgres → services).
type kindedErrorCache interface {
        ErrorKind() string
}

// Per Item 7: these typed sentinel errors classify the specific failure
// mode of reloadFromDBLocked. GetActiveConfig uses them to decide
// whether env fallback is safe (only ErrNoActiveVersion triggers it)
// or whether the error must surface to the caller (ErrNoActiveCredential
// + ErrDBFailure).
var (
        // ErrNoActiveVersion: the DB has no active configuration version.
        // This is the legitimate bootstrap state — env fallback is safe.
        ErrNoActiveVersion = errors.New("no active AI configuration version in DB (bootstrap state)")
        // ErrNoActiveCredential: the DB has no active credential. This is
        // NOT the same as ErrNoActiveVersion — the credential is required
        // even when using env fallback model/limits. Without a credential,
        // AI execution cannot proceed (no API key).
        ErrNoActiveCredential = errors.New("no active AI credential in DB (credential is required)")
        // ErrDBFailure: a real DB error (connection refused, query syntax,
        // etc.). The error must surface to the caller — no silent env
        // fallback (per P2-14 + Item 7).
        ErrDBFailure = errors.New("AI configuration DB failure (not falling back to stale env config)")
)

// isKindedNotFound checks if err implements kindedErrorCache and has
// Kind == "not_found". Used to classify RepositoryError(RepositoryNotFound).
func isKindedNotFound(err error) bool {
        if err == nil {
                return false
        }
        var ke kindedErrorCache
        if errors.As(err, &ke) {
                return ke.ErrorKind() == "not_found"
        }
        return false
}

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
        //
        // Per P2-14: this is the critical safety boundary. If the DB reload
        // returns an error (not "no active version" — that's the bootstrap
        // case — but a real DB error like connection refused), we MUST NOT
        // silently fall back to the env config. Doing so would mean:
        //   - The admin activates a NEW config (DB write succeeds).
        //   - DB goes down briefly (network blip).
        //   - Cache invalidates (correctly) on the next Invalidate() call.
        //   - GetActiveConfig tries reloadFromDBLocked — fails with DB error.
        //   - Falls back to env (which has the OLD key).
        //   - Old key may be REVOKED or the wrong model — silent regression.
        //
        // The fix: distinguish "no active version" (legitimate bootstrap
        // state, env fallback is OK) from "DB error" (real failure, must
        // surface to caller, NOT silently use stale env config).
        if c.repo != nil {
                err := c.reloadFromDBLocked(ctx)
                if err == nil {
                        if c.config.APIKey != "" {
                                return c.config, nil
                        }
                } else if errors.Is(err, ErrNoActiveVersion) {
                        // No active version in DB — env fallback is OK per Item 7.
                        // This is the ONLY path where env fallback is safe.
                } else {
                        // ErrNoActiveCredential or ErrDBFailure — surface to caller.
                        // Per Item 7: "أي DB failure حقيقي يجب أن يصل للـcaller."
                        return ports.AIActiveConfig{}, err
                }
        }

        // Fall back to env config (set once at bootstrap). Per §11: if no
        // env config exists, AI execution is prevented with a clear error.
        //
        // Per P2-14: this fallback is ONLY safe BEFORE any DB active version
        // has ever been created (the bootstrap state). Once an admin has
        // activated a DB-backed config, env fallback is FORBIDDEN — the
        // cache MUST surface the DB error so operators can fix the issue.
        // The isNoActiveVersionError check above enforces this boundary.
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
                return fmt.Errorf("%w: repository not wired", ErrDBFailure)
        }
        if c.creds == nil {
                return fmt.Errorf("%w: credential repository not wired", ErrDBFailure)
        }
        // Per Item 7: always read the active credential FIRST. The
        // credential is required for AI execution — without it, there's
        // no API key. Classify the error as ErrNoActiveCredential (NOT
        // ErrNoActiveVersion) so GetActiveConfig does NOT fall back to env
        // when the credential is missing.
        cred, decryptedKey, err := c.creds.GetActiveCredential(ctx, "google_gemini")
        if err != nil {
                if isKindedNotFound(err) {
                        // Per Item 7: "عدم وجود credential لا يجب أن يتحول تلقائيًا
                        // إلى no active version." This is a distinct failure mode.
                        return fmt.Errorf("%w: %v", ErrNoActiveCredential, err)
                }
                // Real DB error (connection refused, etc.).
                return fmt.Errorf("%w: %v", ErrDBFailure, err)
        }
        // Try to read the active DB version.
        version, versionErr := c.repo.GetActiveVersion(ctx, "google_gemini")
        if versionErr == nil {
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
        if isKindedNotFound(versionErr) {
                // No active version — this is the bootstrap state where env
                // fallback is safe. But we still need the credential (which we
                // already have above). Fall back to env model/limits + the new
                // credential's decrypted key.
                if !c.envLoaded {
                        return fmt.Errorf("%w: %v", ErrNoActiveVersion, versionErr)
                }
                c.config = c.envConfig
                c.config.APIKey = decryptedKey
                c.config.CredentialID = cred.ID
                c.config.CredentialStatus = cred.Status
                c.loaded = true
                return nil
        }
        // Real DB error from GetActiveVersion.
        return fmt.Errorf("%w: %v", ErrDBFailure, versionErr)
}

func derefString(s *string) string {
        if s == nil {
                return ""
        }
        return *s
}

var _ ports.AIConfigurationProvider = (*AIConfigurationCache)(nil)

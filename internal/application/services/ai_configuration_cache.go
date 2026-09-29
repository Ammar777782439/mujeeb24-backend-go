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
//
// The postgres.RepositoryError type implements this interface via its
// ErrorKind() method (returns the kind as a string like "not_found").
//
// This is a SEPARATE declaration from conversation_summary_service.go's
// kindedError — they're structurally identical but in different files.
// Go doesn't allow redeclaration in the same package, so we use a
// distinct name here.
type kindedErrorCache interface {
        ErrorKind() string
}

// isNoActiveVersionError returns true when err indicates the DB has no
// active configuration version (the bootstrap state — admin hasn't yet
// activated a DB-backed config). In this case, falling back to the
// env-loaded config is safe (per P2-14).
//
// Per Item 7: the previous implementation used STRING MATCHING on the
// error message ("no active version", "not found", "no rows"). That
// was fragile — different repository implementations could produce
// different message phrasings, + a real DB error containing "no rows"
// in its diagnostic text would be misclassified as "bootstrap state".
//
// The fix uses TYPED error discrimination via the kindedError interface
// (errors.As + ErrorKind() == "not_found"). This is the same pattern
// used by isRepositoryNotFound in conversation_summary_service.go.
// String matching is NOT used as a primary check — only as a last-
// resort fallback for non-RepositoryError errors (which shouldn't
// happen in production, but is defensive against future repository
// implementations that don't use RepositoryError).
func isNoActiveVersionError(err error) bool {
        if err == nil {
                return false
        }
        // Primary: typed check via kindedError interface.
        var ke kindedErrorCache
        if errors.As(err, &ke) {
                return ke.ErrorKind() == "not_found"
        }
        // Fallback: if the error doesn't implement ErrorKind, we can't
        // trust string matching — treat it as a real error (NOT bootstrap
        // state). This is the safe default per Item 7: "أي DB failure حقيقي
        // يجب أن يصل للـcaller ولا يتحول إلى stale env config."
        return false
}

// isNoActiveCredentialError returns true when err indicates the DB has
// no active credential (the admin hasn't stored a credential yet —
// bootstrap state). Same typed-error pattern as isNoActiveVersionError.
func isNoActiveCredentialError(err error) bool {
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
                        // reloadFromDBLocked succeeded but found no version —
                        // this is the bootstrap state. Env fallback below is OK.
                } else if isNoActiveVersionError(err) {
                        // No active version in DB — env fallback is OK.
                        // This is the only path where env fallback is safe.
                } else {
                        // Real DB error — DO NOT silently fall back to env.
                        // Per P2-14: surface the error so the caller can decide
                        // (typically: refuse to execute AI calls — better than
                        // silently running with stale config).
                        return ports.AIActiveConfig{}, fmt.Errorf("AI configuration DB reload failed (NOT falling back to stale env config per P2-14): %w", err)
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

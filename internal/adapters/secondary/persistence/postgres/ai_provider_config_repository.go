package postgres

import (
        "context"
        "crypto/aes"
        "crypto/cipher"
        "crypto/rand"
        "encoding/base64"
        "encoding/json"
        "errors"
        "fmt"
        "io"
        "time"

        "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
        "github.com/google/uuid"
        "github.com/jackc/pgx/v5"
)

// AIProviderConfigRepository implements ports.AICredentialRepository +
// ports.AIModelRepository + ports.AIConfigurationRepository against Postgres.
//
// Per §1: API keys are stored encrypted (AES-GCM) in the encrypted_key column.
// The encryption key comes from env (bootstrap secret). The key_hint stores
// only the last 4 characters for UI display — the full key is NEVER returned
// to the frontend.
type AIProviderConfigRepository struct {
        adapter         *Adapter
        encryptionKey   []byte // AES-256 key from env (32 bytes)
}

func NewAIProviderConfigRepository(adapter *Adapter, encryptionKey []byte) *AIProviderConfigRepository {
        return &AIProviderConfigRepository{adapter: adapter, encryptionKey: encryptionKey}
}

// ErrEncryptionKeyNotConfigured is returned when the AIProviderConfigRepository
// is constructed without an encryption key. Per P1-9: storing API credentials
// as plain base64 (reversible) is FORBIDDEN in production — the repository
// refuses to encrypt or decrypt when the key is missing.
//
// The bootstrap (api.go) MUST fail-fast at startup when AI_CONFIG_ENCRYPTION_KEY
// is unset. This error is the defense-in-depth guard at the repository boundary
// in case the bootstrap check is bypassed (e.g., tests, ad-hoc scripts).
var ErrEncryptionKeyNotConfigured = errors.New("AI_CONFIG_ENCRYPTION_KEY is not configured — encrypted credential storage is mandatory, refusing to encrypt/decrypt with base64 fallback")

// encryptKey encrypts the API key using AES-GCM (32-byte key).
// Per P1-9: the base64 fallback is REMOVED. Without a configured
// AI_CONFIG_ENCRYPTION_KEY, the repository returns ErrEncryptionKeyNotConfigured
// and the caller MUST fail-fast.
func (r *AIProviderConfigRepository) encryptKey(plaintext string) (string, error) {
        if len(r.encryptionKey) == 0 {
                // P1-9: NO base64 fallback. Storing API credentials as reversible
                // base64 strings would expose them to anyone with DB read access.
                // Production MUST set AI_CONFIG_ENCRYPTION_KEY (32 bytes min).
                return "", ErrEncryptionKeyNotConfigured
        }
        block, err := aes.NewCipher(r.encryptionKey)
        if err != nil {
                return "", fmt.Errorf("create cipher: %w", err)
        }
        gcm, err := cipher.NewGCM(block)
        if err != nil {
                return "", fmt.Errorf("create gcm: %w", err)
        }
        nonce := make([]byte, gcm.NonceSize())
        if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
                return "", fmt.Errorf("generate nonce: %w", err)
        }
        encrypted := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
        return base64.StdEncoding.EncodeToString(encrypted), nil
}

// decryptKey decrypts the API key.
// Per P1-9: when AI_CONFIG_ENCRYPTION_KEY is not configured, the
// repository refuses to decrypt — never silently fall back to
// "base64-decode and return as plaintext" which would mean a previously
// base64-stored credential could be silently readable.
func (r *AIProviderConfigRepository) decryptKey(encrypted string) (string, error) {
        data, err := base64.StdEncoding.DecodeString(encrypted)
        if err != nil {
                return "", fmt.Errorf("decode base64: %w", err)
        }
        if len(r.encryptionKey) == 0 {
                // P1-9: NO plaintext fallback. The repository refuses to
                // return a credential that wasn't encrypted.
                return "", ErrEncryptionKeyNotConfigured
        }
        block, err := aes.NewCipher(r.encryptionKey)
        if err != nil {
                return "", fmt.Errorf("create cipher: %w", err)
        }
        gcm, err := cipher.NewGCM(block)
        if err != nil {
                return "", fmt.Errorf("create gcm: %w", err)
        }
        nonceSize := gcm.NonceSize()
        if len(data) < nonceSize {
                return "", errors.New("encrypted data too short")
        }
        nonce, ciphertext := data[:nonceSize], data[nonceSize:]
        plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
        if err != nil {
                return "", fmt.Errorf("decrypt: %w", err)
        }
        return string(plaintext), nil
}

// ----------------------------------------------------------------------------
// Credential Repository
// ----------------------------------------------------------------------------

const aiCredSelectColumns = `id::text, provider, display_name, key_hint, status, validated_at, validation_error, created_by::text, created_at, updated_at, revoked_at`

func scanAICredential(scanner interface{ Scan(dest ...any) error }) (ports.AICredentialRecord, error) {
        var r ports.AICredentialRecord
        err := scanner.Scan(&r.ID, &r.Provider, &r.DisplayName, &r.KeyHint, &r.Status, &r.ValidatedAt, &r.ValidationError, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt, &r.RevokedAt)
        return r, err
}

func (r *AIProviderConfigRepository) StoreCredential(ctx context.Context, create ports.AICredentialCreate) (ports.AICredentialRecord, error) {
        if r == nil || r.adapter == nil {
                return ports.AICredentialRecord{}, ErrPoolClosed
        }
        encKey, err := r.encryptKey(create.EncryptedKey)
        if err != nil {
                return ports.AICredentialRecord{}, fmt.Errorf("encrypt key: %w", err)
        }
        executor, err := r.adapter.Executor(ctx)
        if err != nil {
                return ports.AICredentialRecord{}, err
        }
        var record ports.AICredentialRecord
        err = executor.QueryRow(ctx,
                `INSERT INTO ai_provider_credentials (id, provider, display_name, encrypted_key, key_hint, status, created_by, created_at, updated_at)
                 VALUES ($1::uuid, $2, $3, $4, $5, 'CONFIGURED', $6::uuid, $7, $7)
                 RETURNING `+aiCredSelectColumns,
                create.ID, create.Provider, create.DisplayName, encKey, create.KeyHint, create.CreatedBy, create.Now,
        ).Scan(&record.ID, &record.Provider, &record.DisplayName, &record.KeyHint, &record.Status, &record.ValidatedAt, &record.ValidationError, &record.CreatedBy, &record.CreatedAt, &record.UpdatedAt, &record.RevokedAt)
        if err != nil {
                return ports.AICredentialRecord{}, classifyRepositoryWriteError("ai_credential.store", err)
        }
        return record, nil
}

func (r *AIProviderConfigRepository) GetActiveCredential(ctx context.Context, provider string) (ports.AICredentialRecord, string, error) {
        if r == nil || r.adapter == nil {
                return ports.AICredentialRecord{}, "", ErrPoolClosed
        }
        executor, err := r.adapter.Executor(ctx)
        if err != nil {
                return ports.AICredentialRecord{}, "", err
        }
        var record ports.AICredentialRecord
        var encryptedKey string
        err = executor.QueryRow(ctx,
                `SELECT `+aiCredSelectColumns+`, encrypted_key FROM ai_provider_credentials WHERE provider = $1 AND status IN ('CONFIGURED', 'VALID') ORDER BY created_at DESC LIMIT 1`,
                provider,
        ).Scan(&record.ID, &record.Provider, &record.DisplayName, &record.KeyHint, &record.Status, &record.ValidatedAt, &record.ValidationError, &record.CreatedBy, &record.CreatedAt, &record.UpdatedAt, &record.RevokedAt, &encryptedKey)
        if err != nil {
                if errors.Is(err, pgx.ErrNoRows) {
                        return ports.AICredentialRecord{}, "", &RepositoryError{Operation: "ai_credential.get_active", Kind: RepositoryNotFound, Err: err}
                }
                return ports.AICredentialRecord{}, "", &RepositoryError{Operation: "ai_credential.get_active", Kind: RepositoryInvalid, Err: err}
        }
        decrypted, err := r.decryptKey(encryptedKey)
        if err != nil {
                return record, "", fmt.Errorf("decrypt key: %w", err)
        }
        return record, decrypted, nil
}

func (r *AIProviderConfigRepository) GetCredentialByID(ctx context.Context, id string) (ports.AICredentialRecord, error) {
        if r == nil || r.adapter == nil {
                return ports.AICredentialRecord{}, ErrPoolClosed
        }
        executor, err := r.adapter.Executor(ctx)
        if err != nil {
                return ports.AICredentialRecord{}, err
        }
        return scanAICredential(executor.QueryRow(ctx,
                `SELECT `+aiCredSelectColumns+` FROM ai_provider_credentials WHERE id = $1::uuid`, id))
}

// GetDecryptedKeyByID returns the decrypted API key for a specific credential.
// Per §10: the decrypted key is NEVER returned to the frontend — only used
// for outbound probes (TestConnection, model activation probe) and discarded.
func (r *AIProviderConfigRepository) GetDecryptedKeyByID(ctx context.Context, id string) (string, error) {
        if r == nil || r.adapter == nil {
                return "", ErrPoolClosed
        }
        executor, err := r.adapter.Executor(ctx)
        if err != nil {
                return "", err
        }
        var encryptedKey string
        err = executor.QueryRow(ctx,
                `SELECT encrypted_key FROM ai_provider_credentials WHERE id = $1::uuid`, id).Scan(&encryptedKey)
        if err != nil {
                if errors.Is(err, pgx.ErrNoRows) {
                        return "", &RepositoryError{Operation: "ai_credential.get_decrypted_key", Kind: RepositoryNotFound, Err: err}
                }
                return "", &RepositoryError{Operation: "ai_credential.get_decrypted_key", Kind: RepositoryInvalid, Err: err}
        }
        return r.decryptKey(encryptedKey)
}

func (r *AIProviderConfigRepository) UpdateCredentialStatus(ctx context.Context, id, status string, validationError *string, now time.Time) (ports.AICredentialRecord, error) {
        if r == nil || r.adapter == nil {
                return ports.AICredentialRecord{}, ErrPoolClosed
        }
        executor, err := r.adapter.Executor(ctx)
        if err != nil {
                return ports.AICredentialRecord{}, err
        }
        var validatedAt *time.Time
        if status == "VALID" || status == "INVALID" {
                validatedAt = &now
        }
        return scanAICredential(executor.QueryRow(ctx,
                `UPDATE ai_provider_credentials SET status = $2, validation_error = $3, validated_at = $4, updated_at = $5 WHERE id = $1::uuid RETURNING `+aiCredSelectColumns,
                id, status, validationError, validatedAt, now))
}

func (r *AIProviderConfigRepository) RevokeCredential(ctx context.Context, id string, now time.Time) error {
        if r == nil || r.adapter == nil {
                return ErrPoolClosed
        }
        executor, err := r.adapter.Executor(ctx)
        if err != nil {
                return err
        }
        _, err = executor.Exec(ctx,
                `UPDATE ai_provider_credentials SET status = 'REVOKED', revoked_at = $2, updated_at = $2 WHERE id = $1::uuid`,
                id, now)
        if err != nil {
                return classifyRepositoryWriteError("ai_credential.revoke", err)
        }
        return nil
}

func (r *AIProviderConfigRepository) ListCredentials(ctx context.Context, provider string) ([]ports.AICredentialRecord, error) {
        if r == nil || r.adapter == nil {
                return nil, ErrPoolClosed
        }
        executor, err := r.adapter.Executor(ctx)
        if err != nil {
                return nil, err
        }
        rows, err := executor.Query(ctx,
                `SELECT `+aiCredSelectColumns+` FROM ai_provider_credentials WHERE ($1 = '' OR provider = $1) ORDER BY created_at DESC`, provider)
        if err != nil {
                return nil, &RepositoryError{Operation: "ai_credential.list", Kind: RepositoryInvalid, Err: err}
        }
        defer rows.Close()
        items := []ports.AICredentialRecord{}
        for rows.Next() {
                r, err := scanAICredential(rows)
                if err != nil {
                        return nil, &RepositoryError{Operation: "ai_credential.list", Kind: RepositoryInvalid, Err: err}
                }
                items = append(items, r)
        }
        return items, nil
}

// ----------------------------------------------------------------------------
// Model Repository
// ----------------------------------------------------------------------------

func (r *AIProviderConfigRepository) UpsertDiscoveredModels(ctx context.Context, models []ports.AIProviderModel) error {
        if r == nil || r.adapter == nil {
                return ErrPoolClosed
        }
        executor, err := r.adapter.Executor(ctx)
        if err != nil {
                return err
        }
        for _, m := range models {
                methodsJSON, _ := json.Marshal(m.SupportedMethods)
                _, err := executor.Exec(ctx,
                        `INSERT INTO ai_provider_models (id, provider, model_name, display_name, description, input_token_limit, output_token_limit, supported_methods, thinking_supported, temperature_min, temperature_max, top_p_min, top_p_max, top_k_min, top_k_max, version, base_model, discovered_at, created_at)
                         VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $18)
                         ON CONFLICT (provider, model_name) DO UPDATE SET display_name = EXCLUDED.display_name, description = EXCLUDED.description, input_token_limit = EXCLUDED.input_token_limit, output_token_limit = EXCLUDED.output_token_limit, supported_methods = EXCLUDED.supported_methods, thinking_supported = EXCLUDED.thinking_supported, temperature_min = EXCLUDED.temperature_min, temperature_max = EXCLUDED.temperature_max, top_p_min = EXCLUDED.top_p_min, top_p_max = EXCLUDED.top_p_max, top_k_min = EXCLUDED.top_k_min, top_k_max = EXCLUDED.top_k_max, version = EXCLUDED.version, base_model = EXCLUDED.base_model, discovered_at = EXCLUDED.discovered_at`,
                        uuid.NewString(), m.Provider, m.ModelName, m.DisplayName, m.Description, m.InputTokenLimit, m.OutputTokenLimit, methodsJSON, m.ThinkingSupported, m.TemperatureMin, m.TemperatureMax, m.TopPMin, m.TopPMax, m.TopKMin, m.TopKMax, m.Version, m.BaseModel, m.DiscoveredAt,
                )
                if err != nil {
                        return classifyRepositoryWriteError("ai_model.upsert", err)
                }
        }
        return nil
}

func (r *AIProviderConfigRepository) ListModels(ctx context.Context, provider string) ([]ports.AIProviderModel, error) {
        if r == nil || r.adapter == nil {
                return nil, ErrPoolClosed
        }
        executor, err := r.adapter.Executor(ctx)
        if err != nil {
                return nil, err
        }
        rows, err := executor.Query(ctx,
                `SELECT id::text, provider, model_name, display_name, description, input_token_limit, output_token_limit, supported_methods, thinking_supported, temperature_min, temperature_max, top_p_min, top_p_max, top_k_min, top_k_max, version, base_model, discovered_at FROM ai_provider_models WHERE ($1 = '' OR provider = $1) ORDER BY model_name`, provider)
        if err != nil {
                return nil, &RepositoryError{Operation: "ai_model.list", Kind: RepositoryInvalid, Err: err}
        }
        defer rows.Close()
        items := []ports.AIProviderModel{}
        for rows.Next() {
                var m ports.AIProviderModel
                var methodsJSON []byte
                if err := rows.Scan(&m.ID, &m.Provider, &m.ModelName, &m.DisplayName, &m.Description, &m.InputTokenLimit, &m.OutputTokenLimit, &methodsJSON, &m.ThinkingSupported, &m.TemperatureMin, &m.TemperatureMax, &m.TopPMin, &m.TopPMax, &m.TopKMin, &m.TopKMax, &m.Version, &m.BaseModel, &m.DiscoveredAt); err != nil {
                        return nil, &RepositoryError{Operation: "ai_model.list", Kind: RepositoryInvalid, Err: err}
                }
                _ = json.Unmarshal(methodsJSON, &m.SupportedMethods)
                items = append(items, m)
        }
        return items, nil
}

func (r *AIProviderConfigRepository) GetModel(ctx context.Context, provider, modelName string) (ports.AIProviderModel, error) {
        if r == nil || r.adapter == nil {
                return ports.AIProviderModel{}, ErrPoolClosed
        }
        executor, err := r.adapter.Executor(ctx)
        if err != nil {
                return ports.AIProviderModel{}, err
        }
        var m ports.AIProviderModel
        var methodsJSON []byte
        err = executor.QueryRow(ctx,
                `SELECT id::text, provider, model_name, display_name, description, input_token_limit, output_token_limit, supported_methods, thinking_supported, temperature_min, temperature_max, top_p_min, top_p_max, top_k_min, top_k_max, version, base_model, discovered_at FROM ai_provider_models WHERE provider = $1 AND model_name = $2`, provider, modelName).
                Scan(&m.ID, &m.Provider, &m.ModelName, &m.DisplayName, &m.Description, &m.InputTokenLimit, &m.OutputTokenLimit, &methodsJSON, &m.ThinkingSupported, &m.TemperatureMin, &m.TemperatureMax, &m.TopPMin, &m.TopPMax, &m.TopKMin, &m.TopKMax, &m.Version, &m.BaseModel, &m.DiscoveredAt)
        if err != nil {
                if errors.Is(err, pgx.ErrNoRows) {
                        return ports.AIProviderModel{}, &RepositoryError{Operation: "ai_model.get", Kind: RepositoryNotFound, Err: err}
                }
                return ports.AIProviderModel{}, &RepositoryError{Operation: "ai_model.get", Kind: RepositoryInvalid, Err: err}
        }
        _ = json.Unmarshal(methodsJSON, &m.SupportedMethods)
        return m, nil
}

// ----------------------------------------------------------------------------
// Configuration Version Repository
// ----------------------------------------------------------------------------

const aiConfigSelectColumns = `id::text, version, provider, model, credential_id::text, mujeeb_max_input_chars, mujeeb_max_output_tokens, effective_max_output_tokens, pricing_version, status, created_by::text, created_at, activated_at, deactivated_at, reason`

func scanAIConfig(scanner interface{ Scan(dest ...any) error }) (ports.AIConfigurationVersion, error) {
        var v ports.AIConfigurationVersion
        err := scanner.Scan(&v.ID, &v.Version, &v.Provider, &v.Model, &v.CredentialID, &v.MujeebMaxInputChars, &v.MujeebMaxOutputTokens, &v.EffectiveMaxOutputTokens, &v.PricingVersion, &v.Status, &v.CreatedBy, &v.CreatedAt, &v.ActivatedAt, &v.DeactivatedAt, &v.Reason)
        return v, err
}

func (r *AIProviderConfigRepository) CreateVersion(ctx context.Context, create ports.AIConfigurationCreate) (ports.AIConfigurationVersion, error) {
        if r == nil || r.adapter == nil {
                return ports.AIConfigurationVersion{}, ErrPoolClosed
        }
        executor, err := r.adapter.Executor(ctx)
        if err != nil {
                return ports.AIConfigurationVersion{}, err
        }
        // Compute next version number.
        var maxVersion int
        _ = executor.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM ai_configuration_versions WHERE provider = $1`, create.Provider).Scan(&maxVersion)
        newVersion := maxVersion + 1
        // Per §6: effective = min(Mujeeb, Provider). The handler computes this
        // before calling CreateVersion and passes it via EffectiveMaxOutputTokens.
        // If the caller did NOT compute it (zero), fall back to the Mujeeb limit
        // — this preserves backward compatibility with callers that don't yet
        // enforce the provider clamp (e.g., tests, bootstrap).
        effMax := create.EffectiveMaxOutputTokens
        if effMax <= 0 {
                effMax = create.MujeebMaxOutputTokens
        }
        return scanAIConfig(executor.QueryRow(ctx,
                `INSERT INTO ai_configuration_versions (id, version, provider, model, credential_id, mujeeb_max_input_chars, mujeeb_max_output_tokens, effective_max_output_tokens, pricing_version, status, created_by, created_at)
                 VALUES ($1::uuid, $2, $3, $4, $5::uuid, $6, $7, $8, $9, 'DRAFT', $10::uuid, $11)
                 RETURNING `+aiConfigSelectColumns,
                create.ID, newVersion, create.Provider, create.Model, create.CredentialID, create.MujeebMaxInputChars, create.MujeebMaxOutputTokens, effMax, create.PricingVersion, create.CreatedBy, create.Now))
}

func (r *AIProviderConfigRepository) ActivateVersion(ctx context.Context, id string, now time.Time) (ports.AIConfigurationVersion, error) {
        if r == nil || r.adapter == nil {
                return ports.AIConfigurationVersion{}, ErrPoolClosed
        }
        executor, err := r.adapter.Executor(ctx)
        if err != nil {
                return ports.AIConfigurationVersion{}, err
        }
        // First, deactivate any currently active version for this provider.
        // The singleton unique index ensures only one ACTIVE per provider.
        var provider string
        _ = executor.QueryRow(ctx, `SELECT provider FROM ai_configuration_versions WHERE id = $1::uuid`, id).Scan(&provider)
        if provider != "" {
                _, _ = executor.Exec(ctx, `UPDATE ai_configuration_versions SET status = 'DEACTIVATED', deactivated_at = $2 WHERE provider = $1 AND status = 'ACTIVE'`, provider, now)
        }
        return scanAIConfig(executor.QueryRow(ctx,
                `UPDATE ai_configuration_versions SET status = 'ACTIVE', activated_at = $2 WHERE id = $1::uuid AND status = 'DRAFT' RETURNING `+aiConfigSelectColumns,
                id, now))
}

func (r *AIProviderConfigRepository) GetActiveVersion(ctx context.Context, provider string) (ports.AIConfigurationVersion, error) {
        if r == nil || r.adapter == nil {
                return ports.AIConfigurationVersion{}, ErrPoolClosed
        }
        executor, err := r.adapter.Executor(ctx)
        if err != nil {
                return ports.AIConfigurationVersion{}, err
        }
        return scanAIConfig(executor.QueryRow(ctx,
                `SELECT `+aiConfigSelectColumns+` FROM ai_configuration_versions WHERE provider = $1 AND status = 'ACTIVE' LIMIT 1`, provider))
}

func (r *AIProviderConfigRepository) GetVersionByID(ctx context.Context, id string) (ports.AIConfigurationVersion, error) {
        if r == nil || r.adapter == nil {
                return ports.AIConfigurationVersion{}, ErrPoolClosed
        }
        executor, err := r.adapter.Executor(ctx)
        if err != nil {
                return ports.AIConfigurationVersion{}, err
        }
        return scanAIConfig(executor.QueryRow(ctx,
                `SELECT `+aiConfigSelectColumns+` FROM ai_configuration_versions WHERE id = $1::uuid`, id))
}

func (r *AIProviderConfigRepository) ListVersions(ctx context.Context, provider string, limit int) ([]ports.AIConfigurationVersion, error) {
        if r == nil || r.adapter == nil {
                return nil, ErrPoolClosed
        }
        if limit <= 0 || limit > 100 {
                limit = 20
        }
        executor, err := r.adapter.Executor(ctx)
        if err != nil {
                return nil, err
        }
        rows, err := executor.Query(ctx,
                `SELECT `+aiConfigSelectColumns+` FROM ai_configuration_versions WHERE ($1 = '' OR provider = $1) ORDER BY version DESC LIMIT $2`, provider, limit)
        if err != nil {
                return nil, &RepositoryError{Operation: "ai_config.list", Kind: RepositoryInvalid, Err: err}
        }
        defer rows.Close()
        items := []ports.AIConfigurationVersion{}
        for rows.Next() {
                v, err := scanAIConfig(rows)
                if err != nil {
                        return nil, &RepositoryError{Operation: "ai_config.list", Kind: RepositoryInvalid, Err: err}
                }
                items = append(items, v)
        }
        return items, nil
}

// Compile-time assertions
var _ ports.AICredentialRepository = (*AIProviderConfigRepository)(nil)
var _ ports.AIModelRepository = (*AIProviderConfigRepository)(nil)
var _ ports.AIConfigurationRepository = (*AIProviderConfigRepository)(nil)

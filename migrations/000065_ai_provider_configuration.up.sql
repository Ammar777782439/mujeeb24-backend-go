-- AI Provider Configuration — dynamic runtime configuration
-- Per Platform Administration Contract §75-88 + AIUsageTokenTelemetry.md

CREATE TABLE ai_provider_credentials (
    id              UUID PRIMARY KEY,
    provider        TEXT NOT NULL,
    display_name    TEXT NOT NULL,
    encrypted_key   TEXT NOT NULL,
    key_hint        TEXT NOT NULL,  -- last 4 chars for UI display
    status          TEXT NOT NULL DEFAULT 'CONFIGURED',
    validated_at    TIMESTAMPTZ,
    validation_error TEXT,
    created_by      UUID NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,

    CONSTRAINT ai_creds_provider_chk CHECK (provider IN ('google_gemini', 'socialapi')),
    CONSTRAINT ai_creds_status_chk CHECK (status IN ('CONFIGURED', 'VALID', 'INVALID', 'REVOKED')),
    CONSTRAINT ai_creds_revoked_chk CHECK ((status = 'REVOKED') = (revoked_at IS NOT NULL))
);
CREATE INDEX idx_ai_creds_provider_active ON ai_provider_credentials (provider) WHERE status != 'REVOKED';

CREATE TABLE ai_provider_models (
    id                  UUID PRIMARY KEY,
    provider            TEXT NOT NULL,
    model_name          TEXT NOT NULL,
    display_name        TEXT,
    description         TEXT,
    input_token_limit   INT,
    output_token_limit  INT,
    supported_methods   JSONB NOT NULL DEFAULT '[]'::jsonb,
    thinking_supported  BOOLEAN NOT NULL DEFAULT FALSE,
    temperature_min     DOUBLE PRECISION,
    temperature_max     DOUBLE PRECISION,
    top_p_min           DOUBLE PRECISION,
    top_p_max           DOUBLE PRECISION,
    top_k_min           INT,
    top_k_max           INT,
    version             TEXT,
    base_model          TEXT,
    discovered_at       TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL,

    CONSTRAINT ai_models_provider_model_uq UNIQUE (provider, model_name)
);

CREATE TABLE ai_configuration_versions (
    id                      UUID PRIMARY KEY,
    version                 INT NOT NULL,
    provider                TEXT NOT NULL,
    model                   TEXT NOT NULL,
    credential_id           UUID NOT NULL REFERENCES ai_provider_credentials(id) ON DELETE RESTRICT,
    mujeeb_max_input_chars  INT NOT NULL,
    mujeeb_max_output_tokens INT NOT NULL,
    effective_max_output_tokens INT NOT NULL,
    pricing_version         TEXT,
    status                  TEXT NOT NULL DEFAULT 'DRAFT',
    created_by              UUID NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL,
    activated_at            TIMESTAMPTZ,
    deactivated_at          TIMESTAMPTZ,
    reason                  TEXT,

    CONSTRAINT ai_config_status_chk CHECK (status IN ('DRAFT', 'ACTIVE', 'DEACTIVATED')),
    CONSTRAINT ai_config_active_chk CHECK ((status = 'ACTIVE') = (activated_at IS NOT NULL AND deactivated_at IS NULL)),
    CONSTRAINT ai_config_provider_version_uq UNIQUE (provider, version)
);
CREATE UNIQUE INDEX idx_ai_config_active_singleton ON ai_configuration_versions (provider) WHERE status = 'ACTIVE';

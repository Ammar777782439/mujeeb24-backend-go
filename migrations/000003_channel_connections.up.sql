CREATE TABLE channel_connections (
    id                      UUID PRIMARY KEY,
    business_id             UUID NOT NULL,
    provider_ref            TEXT NOT NULL,
    channel                 TEXT NOT NULL,
    provider_account_ref    TEXT,
    provider_connection_ref TEXT NOT NULL,
    status                  TEXT NOT NULL,
    secret_reference        TEXT NOT NULL,
    last_health_check_at    TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL,
    updated_at              TIMESTAMPTZ NOT NULL,

    CONSTRAINT channel_connections_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT channel_connections_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT channel_connections_provider_ref_chk
        CHECK (length(btrim(provider_ref)) > 0),
    CONSTRAINT channel_connections_channel_chk
        CHECK (channel IN ('facebook', 'instagram', 'whatsapp')),
    CONSTRAINT channel_connections_connection_ref_chk
        CHECK (length(btrim(provider_connection_ref)) > 0),
    CONSTRAINT channel_connections_secret_ref_chk
        CHECK (length(btrim(secret_reference)) > 0),
    CONSTRAINT channel_connections_status_chk
        CHECK (status IN ('pending', 'active', 'disconnected', 'failed', 'reconnect_required', 'archived')),
    CONSTRAINT channel_connections_provider_connection_uq
        UNIQUE (provider_ref, provider_connection_ref)
);

CREATE INDEX idx_channel_connections_business_status
    ON channel_connections (business_id, status);

CREATE INDEX idx_channel_connections_business_channel
    ON channel_connections (business_id, channel);

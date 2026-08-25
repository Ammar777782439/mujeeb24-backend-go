CREATE TABLE channel_provisioning_sessions (
    id                      UUID PRIMARY KEY,
    business_id             UUID NOT NULL,
    idempotency_key         TEXT NOT NULL,
    provider_ref            TEXT NOT NULL,
    channel                 TEXT NOT NULL,
    display_name            TEXT NOT NULL,
    status                  TEXT NOT NULL,
    oauth_state             TEXT,
    authorization_url       TEXT,
    provider_account_ref    TEXT,
    provider_connection_ref TEXT,
    chatwoot_account_id     TEXT,
    chatwoot_inbox_id       TEXT,
    channel_connection_id   UUID,
    failure_code            TEXT,
    created_at              TIMESTAMPTZ NOT NULL,
    updated_at              TIMESTAMPTZ NOT NULL,

    CONSTRAINT channel_provisioning_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT channel_provisioning_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT channel_provisioning_idempotency_uq
        UNIQUE (business_id, idempotency_key),
    CONSTRAINT channel_provisioning_provider_chk
        CHECK (length(btrim(provider_ref)) > 0),
    CONSTRAINT channel_provisioning_channel_chk
        CHECK (channel IN ('facebook', 'instagram', 'whatsapp')),
    CONSTRAINT channel_provisioning_display_name_chk
        CHECK (length(btrim(display_name)) > 0),
    CONSTRAINT channel_provisioning_status_chk
        CHECK (status IN ('pending_authorization', 'provisioning', 'connected', 'failed', 'reconnect_required')),
    CONSTRAINT channel_provisioning_state_chk
        CHECK (oauth_state IS NULL OR length(btrim(oauth_state)) > 0),
    CONSTRAINT channel_provisioning_authorization_url_chk
        CHECK (authorization_url IS NULL OR authorization_url LIKE 'https://%'),
    CONSTRAINT channel_provisioning_failure_chk
        CHECK (failure_code IS NULL OR length(btrim(failure_code)) > 0),
    CONSTRAINT channel_provisioning_connection_fk
        FOREIGN KEY (business_id, channel_connection_id)
        REFERENCES channel_connections (business_id, id)
        ON DELETE RESTRICT
);

CREATE INDEX idx_channel_provisioning_business_status
    ON channel_provisioning_sessions (business_id, status, updated_at DESC, id DESC);

CREATE UNIQUE INDEX uq_channel_provisioning_active_channel
    ON channel_provisioning_sessions (business_id, channel)
    WHERE status IN ('pending_authorization', 'provisioning', 'connected');

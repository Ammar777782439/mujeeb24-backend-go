CREATE TABLE outbound_messages (
    id                       UUID PRIMARY KEY,
    business_id              UUID NOT NULL,
    conversation_id          UUID NOT NULL,
    conversation_reference_id UUID NOT NULL,
    connection_id            UUID NOT NULL,
    provider_ref             TEXT NOT NULL,
    channel                  TEXT NOT NULL,
    origin                   TEXT NOT NULL,
    direction                TEXT NOT NULL,
    transport                TEXT NOT NULL,
    content_reference        TEXT NOT NULL,
    provider_idempotency_key TEXT NOT NULL,
    status                   TEXT NOT NULL,
    provider_message_id      TEXT,
    chatwoot_message_id      TEXT,
    failure_code             TEXT,
    attempt_count            INTEGER NOT NULL DEFAULT 0,
    correlation_id           UUID,
    causation_id             UUID,
    created_at               TIMESTAMPTZ NOT NULL,
    updated_at               TIMESTAMPTZ NOT NULL,

    CONSTRAINT outbound_messages_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT outbound_messages_conversation_fk
        FOREIGN KEY (business_id, conversation_id)
        REFERENCES conversations (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT outbound_messages_reference_fk
        FOREIGN KEY (business_id, conversation_reference_id)
        REFERENCES conversation_references (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT outbound_messages_connection_fk
        FOREIGN KEY (business_id, connection_id)
        REFERENCES channel_connections (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT outbound_messages_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT outbound_messages_provider_ref_chk
        CHECK (length(btrim(provider_ref)) > 0),
    CONSTRAINT outbound_messages_channel_chk
        CHECK (channel IN ('facebook', 'instagram', 'whatsapp')),
    CONSTRAINT outbound_messages_origin_chk
        CHECK (origin IN ('human', 'ai', 'automation', 'system')),
    CONSTRAINT outbound_messages_direction_chk
        CHECK (direction = 'outbound'),
    CONSTRAINT outbound_messages_transport_chk
        CHECK (transport IN ('provider', 'chatwoot')),
    CONSTRAINT outbound_messages_content_ref_chk
        CHECK (length(btrim(content_reference)) > 0),
    CONSTRAINT outbound_messages_idempotency_key_chk
        CHECK (length(btrim(provider_idempotency_key)) > 0),
    CONSTRAINT outbound_messages_status_chk
        CHECK (status IN ('pending', 'sending', 'accepted', 'sent', 'delivered', 'read', 'failed', 'unknown')),
    CONSTRAINT outbound_messages_attempt_count_chk
        CHECK (attempt_count >= 0),
    CONSTRAINT outbound_messages_provider_idempotency_uq
        UNIQUE (provider_ref, connection_id, provider_idempotency_key)
);

CREATE INDEX idx_outbound_messages_business_status
    ON outbound_messages (business_id, status, updated_at DESC);

CREATE INDEX idx_outbound_messages_conversation
    ON outbound_messages (business_id, conversation_id, created_at DESC);

CREATE INDEX idx_outbound_messages_correlation
    ON outbound_messages (business_id, correlation_id)
    WHERE correlation_id IS NOT NULL;

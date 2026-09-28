CREATE TABLE conversation_references (
    id                    UUID PRIMARY KEY,
    business_id           UUID NOT NULL,
    conversation_id       UUID NOT NULL,
    system                TEXT NOT NULL,
    provider_ref          TEXT NOT NULL,
    resource_type         TEXT NOT NULL,
    resource_id           TEXT NOT NULL,
    connection_id         UUID,
    conversation_kind     TEXT,
    is_current            BOOLEAN NOT NULL,
    mapping_status        TEXT NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL,
    updated_at            TIMESTAMPTZ NOT NULL,

    CONSTRAINT conversation_references_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT conversation_references_conversation_fk
        FOREIGN KEY (business_id, conversation_id)
        REFERENCES conversations (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT conversation_references_connection_fk
        FOREIGN KEY (business_id, connection_id)
        REFERENCES channel_connections (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT conversation_references_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT conversation_references_system_chk
        CHECK (system IN ('provider', 'chatwoot')),
    CONSTRAINT conversation_references_provider_chk
        CHECK (length(btrim(provider_ref)) > 0),
    CONSTRAINT conversation_references_resource_type_chk
        CHECK (length(btrim(resource_type)) > 0),
    CONSTRAINT conversation_references_resource_id_chk
        CHECK (length(btrim(resource_id)) > 0),
    CONSTRAINT conversation_references_provider_connection_chk
        CHECK ((system = 'provider' AND connection_id IS NOT NULL) OR (system = 'chatwoot')),
    CONSTRAINT conversation_references_kind_chk
        CHECK (conversation_kind IS NULL OR conversation_kind IN (
            'dm', 'comment', 'story_reply', 'mention', 'review', 'other'
        )),
    CONSTRAINT conversation_references_mapping_status_chk
        CHECK (mapping_status IN ('pending', 'active', 'stale', 'failed'))
);

CREATE UNIQUE INDEX uq_conversation_references_external_resource
    ON conversation_references (
        business_id, system, provider_ref, resource_type, resource_id
    );

CREATE UNIQUE INDEX uq_conversation_references_current_system
    ON conversation_references (business_id, conversation_id, system)
    WHERE is_current;

CREATE INDEX idx_conversation_references_conversation
    ON conversation_references (business_id, conversation_id, updated_at DESC);

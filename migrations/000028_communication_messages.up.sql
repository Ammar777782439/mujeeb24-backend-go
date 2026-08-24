ALTER TABLE inbound_event_ledger
    ADD CONSTRAINT inbound_event_ledger_business_id_uq UNIQUE (business_id, id);

CREATE TABLE communication_messages (
    id                       UUID PRIMARY KEY,
    business_id              UUID NOT NULL,
    conversation_reference_id UUID NOT NULL,
    inbound_event_id         UUID,
    outbound_message_id      UUID,
    direction                TEXT NOT NULL,
    origin                   TEXT NOT NULL,
    transport                TEXT NOT NULL,
    provider_message_id      TEXT,
    chatwoot_message_id      TEXT,
    content_type             TEXT NOT NULL,
    text_content             TEXT,
    content_reference        TEXT NOT NULL,
    occurred_at              TIMESTAMPTZ NOT NULL,
    created_at               TIMESTAMPTZ NOT NULL,

    CONSTRAINT communication_messages_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT communication_messages_conversation_reference_fk
        FOREIGN KEY (business_id, conversation_reference_id)
        REFERENCES conversation_references (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT communication_messages_inbound_event_fk
        FOREIGN KEY (business_id, inbound_event_id)
        REFERENCES inbound_event_ledger (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT communication_messages_outbound_message_fk
        FOREIGN KEY (business_id, outbound_message_id)
        REFERENCES outbound_messages (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT communication_messages_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT communication_messages_direction_chk
        CHECK (direction IN ('inbound', 'outbound')),
    CONSTRAINT communication_messages_origin_chk
        CHECK (origin IN ('customer', 'ai', 'human', 'automation', 'system')),
    CONSTRAINT communication_messages_transport_chk
        CHECK (transport IN ('provider', 'chatwoot', 'mujeeb')),
    CONSTRAINT communication_messages_content_type_chk
        CHECK (content_type IN ('text', 'image', 'video', 'audio', 'file', 'mixed', 'unknown')),
    CONSTRAINT communication_messages_content_reference_chk
        CHECK (length(btrim(content_reference)) > 0),
    CONSTRAINT communication_messages_text_content_chk
        CHECK (content_type <> 'text' OR text_content IS NOT NULL),
    CONSTRAINT communication_messages_external_reference_chk
        CHECK (provider_message_id IS NULL OR length(btrim(provider_message_id)) > 0),
    CONSTRAINT communication_messages_chatwoot_reference_chk
        CHECK (chatwoot_message_id IS NULL OR length(btrim(chatwoot_message_id)) > 0)
);

CREATE UNIQUE INDEX uq_communication_messages_provider_message
    ON communication_messages (business_id, transport, provider_message_id)
    WHERE provider_message_id IS NOT NULL;

CREATE UNIQUE INDEX uq_communication_messages_chatwoot_message
    ON communication_messages (business_id, transport, chatwoot_message_id)
    WHERE chatwoot_message_id IS NOT NULL;

CREATE INDEX idx_communication_messages_timeline
    ON communication_messages (
        business_id,
        conversation_reference_id,
        occurred_at DESC,
        created_at DESC,
        id DESC
    );

CREATE INDEX idx_communication_messages_inbound_event
    ON communication_messages (business_id, inbound_event_id)
    WHERE inbound_event_id IS NOT NULL;

CREATE INDEX idx_communication_messages_outbound_message
    ON communication_messages (business_id, outbound_message_id)
    WHERE outbound_message_id IS NOT NULL;

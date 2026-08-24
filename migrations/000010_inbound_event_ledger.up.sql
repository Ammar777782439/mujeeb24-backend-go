CREATE TABLE inbound_event_ledger (
    id                      UUID PRIMARY KEY,
    provider_ref            TEXT NOT NULL,
    provider_connection_ref TEXT NOT NULL,
    provider_event_id       TEXT NOT NULL,
    dedupe_strategy         TEXT NOT NULL,
    business_id             UUID,
    connection_id           UUID,
    event_type              TEXT NOT NULL,
    interaction_kind        TEXT,
    provider_message_id     TEXT,
    provider_conversation_id TEXT,
    external_user_id        TEXT,
    content_reference       TEXT,
    external_created_at     TIMESTAMPTZ,
    received_at             TIMESTAMPTZ NOT NULL,
    raw_payload_reference   TEXT NOT NULL,
    payload_hash            TEXT NOT NULL,
    signature_verified      BOOLEAN NOT NULL,
    processing_state        TEXT NOT NULL,
    processing_owner        TEXT,
    lease_expires_at        TIMESTAMPTZ,
    attempt_count           INTEGER NOT NULL DEFAULT 0,
    last_error_code         TEXT,
    next_attempt_at         TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL,
    updated_at              TIMESTAMPTZ NOT NULL,

    CONSTRAINT inbound_event_provider_ref_chk
        CHECK (length(btrim(provider_ref)) > 0),
    CONSTRAINT inbound_event_connection_ref_chk
        CHECK (length(btrim(provider_connection_ref)) > 0),
    CONSTRAINT inbound_event_id_chk
        CHECK (length(btrim(provider_event_id)) > 0),
    CONSTRAINT inbound_event_dedupe_strategy_chk
        CHECK (dedupe_strategy IN ('provider_event_id', 'provider_specific_fallback', 'dedupe_uncertain')),
    CONSTRAINT inbound_event_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT inbound_event_connection_fk
        FOREIGN KEY (business_id, connection_id)
        REFERENCES channel_connections (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT inbound_event_resolution_pair_chk
        CHECK ((business_id IS NULL AND connection_id IS NULL) OR (business_id IS NOT NULL AND connection_id IS NOT NULL)),
    CONSTRAINT inbound_event_type_chk
        CHECK (event_type IN (
            'interaction_received', 'interaction_updated',
            'delivery_status_changed', 'conversation_updated',
            'account_status_changed'
        )),
    CONSTRAINT inbound_event_interaction_kind_chk
        CHECK (interaction_kind IS NULL OR interaction_kind IN (
            'dm', 'comment', 'story_reply', 'mention', 'review', 'postback', 'other'
        )),
    CONSTRAINT inbound_event_processing_state_chk
        CHECK (processing_state IN (
            'received', 'unresolved', 'processing', 'processed',
            'retryable_failed', 'dead_letter', 'rejected'
        )),
    CONSTRAINT inbound_event_attempt_count_chk
        CHECK (attempt_count >= 0),
    CONSTRAINT inbound_event_lease_chk
        CHECK (
            (processing_state = 'processing' AND processing_owner IS NOT NULL AND lease_expires_at IS NOT NULL)
            OR (processing_state <> 'processing' AND processing_owner IS NULL AND lease_expires_at IS NULL)
        ),
    CONSTRAINT inbound_event_dedupe_strategy_consistency_chk
        CHECK (
            dedupe_strategy <> 'dedupe_uncertain'
            OR processing_state IN ('unresolved', 'retryable_failed', 'dead_letter', 'rejected')
        ),
    CONSTRAINT inbound_event_event_uq
        UNIQUE (provider_ref, provider_connection_ref, provider_event_id)
);

CREATE INDEX idx_inbound_event_unresolved
    ON inbound_event_ledger (provider_ref, provider_connection_ref, processing_state, received_at)
    WHERE processing_state = 'unresolved';

CREATE INDEX idx_inbound_event_processing
    ON inbound_event_ledger (processing_state, next_attempt_at, received_at)
    WHERE processing_state IN ('received', 'retryable_failed');

CREATE INDEX idx_inbound_event_business_received
    ON inbound_event_ledger (business_id, received_at DESC)
    WHERE business_id IS NOT NULL;

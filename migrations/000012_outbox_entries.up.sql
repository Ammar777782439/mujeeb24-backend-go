CREATE TABLE outbox_entries (
    id                  UUID PRIMARY KEY,
    business_id         UUID NOT NULL,
    outbound_message_id UUID NOT NULL,
    command_type        TEXT NOT NULL,
    dedupe_key          TEXT NOT NULL,
    status              TEXT NOT NULL,
    attempt_count       INTEGER NOT NULL DEFAULT 0,
    available_at        TIMESTAMPTZ NOT NULL,
    lease_owner         TEXT,
    lease_expires_at    TIMESTAMPTZ,
    last_error_code     TEXT,
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL,

    CONSTRAINT outbox_entries_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT outbox_entries_outbound_fk
        FOREIGN KEY (business_id, outbound_message_id)
        REFERENCES outbound_messages (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT outbox_entries_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT outbox_entries_outbound_id_uq
        UNIQUE (business_id, outbound_message_id),
    CONSTRAINT outbox_entries_command_type_chk
        CHECK (length(btrim(command_type)) > 0),
    CONSTRAINT outbox_entries_dedupe_key_chk
        CHECK (length(btrim(dedupe_key)) > 0),
    CONSTRAINT outbox_entries_status_chk
        CHECK (status IN ('pending', 'processing', 'completed', 'retryable_failed', 'dead_letter')),
    CONSTRAINT outbox_entries_attempt_count_chk
        CHECK (attempt_count >= 0),
    CONSTRAINT outbox_entries_lease_chk
        CHECK (
            (status = 'processing' AND lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL)
            OR (status <> 'processing' AND lease_owner IS NULL AND lease_expires_at IS NULL)
        ),
    CONSTRAINT outbox_entries_dedupe_uq
        UNIQUE (business_id, command_type, dedupe_key)
);

CREATE INDEX idx_outbox_claimable
    ON outbox_entries (status, available_at, created_at)
    WHERE status IN ('pending', 'retryable_failed');

CREATE INDEX idx_outbox_business_status
    ON outbox_entries (business_id, status, updated_at DESC);

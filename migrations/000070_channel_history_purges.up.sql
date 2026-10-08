-- Explicit history purge is independent of channel disconnect and billing.
-- The watermark and dedupe key contain no customer message contents.
CREATE TABLE channel_history_purges (
    business_id UUID NOT NULL,
    connection_id UUID NOT NULL,
    cutoff TIMESTAMPTZ NOT NULL,
    idempotency_key TEXT NOT NULL,
    deleted_conversations BIGINT NOT NULL DEFAULT 0,
    deleted_messages BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, connection_id),
    CONSTRAINT channel_history_purges_connection_fk
        FOREIGN KEY (business_id, connection_id)
        REFERENCES channel_connections (business_id, id) ON DELETE RESTRICT,
    CONSTRAINT channel_history_purges_idempotency_chk
        CHECK (length(btrim(idempotency_key)) > 0)
);

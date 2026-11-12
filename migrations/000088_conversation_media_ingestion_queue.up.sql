-- Add a durable, lease-based work queue for inbound conversation media.
-- The source reference remains opaque and is cleared only after successful ingestion.
ALTER TABLE message_attachments
    ADD COLUMN ingestion_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN ingestion_next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN ingestion_lease_until TIMESTAMPTZ,
    ADD COLUMN ingestion_last_error_code TEXT;

ALTER TABLE message_attachments
    ADD CONSTRAINT message_attachments_ingestion_attempts_chk
        CHECK (ingestion_attempts >= 0),
    ADD CONSTRAINT message_attachments_ingestion_error_code_chk
        CHECK (
            ingestion_last_error_code IS NULL OR
            char_length(ingestion_last_error_code) BETWEEN 1 AND 100
        );

CREATE INDEX idx_message_attachments_ingestion_queue
    ON message_attachments (ingestion_next_attempt_at, created_at, id)
    WHERE status = 'pending_ingestion';

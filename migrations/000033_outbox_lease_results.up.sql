ALTER TABLE outbox_entries
    ADD COLUMN lease_token UUID,
    ADD COLUMN result_code TEXT,
    ADD COLUMN completed_at TIMESTAMPTZ,
    ADD CONSTRAINT outbox_entries_lease_token_chk
        CHECK ((status = 'processing' AND lease_token IS NOT NULL)
            OR (status <> 'processing' AND lease_token IS NULL)),
    ADD CONSTRAINT outbox_entries_result_code_chk
        CHECK (result_code IS NULL OR length(btrim(result_code)) > 0),
    ADD CONSTRAINT outbox_entries_completed_at_chk
        CHECK ((status = 'completed' AND completed_at IS NOT NULL)
            OR (status <> 'completed' AND completed_at IS NULL));

CREATE INDEX idx_outbox_business_available
    ON outbox_entries (business_id, status, available_at, created_at, id)
    WHERE status IN ('pending', 'retryable_failed');

CREATE INDEX idx_outbox_owner_lease
    ON outbox_entries (lease_owner, lease_expires_at, id)
    WHERE status = 'processing';

ALTER TABLE inbound_event_ledger
    ADD COLUMN processing_lease_token UUID,
    ADD COLUMN processing_result_code TEXT,
    ADD COLUMN processed_at TIMESTAMPTZ,
    ADD CONSTRAINT inbound_event_signature_chk
        CHECK (signature_verified OR processing_state IN ('unresolved', 'rejected')),
    ADD CONSTRAINT inbound_event_lease_token_chk
        CHECK ((processing_state = 'processing' AND processing_lease_token IS NOT NULL)
            OR (processing_state <> 'processing' AND processing_lease_token IS NULL)),
    ADD CONSTRAINT inbound_event_result_code_chk
        CHECK (processing_result_code IS NULL OR length(btrim(processing_result_code)) > 0),
    ADD CONSTRAINT inbound_event_processed_at_chk
        CHECK ((processing_state = 'processed' AND processed_at IS NOT NULL)
            OR (processing_state <> 'processed' AND processed_at IS NULL));

CREATE INDEX idx_inbound_event_claimable
    ON inbound_event_ledger (processing_state, next_attempt_at, received_at, id)
    WHERE processing_state IN ('received', 'retryable_failed');

CREATE INDEX idx_inbound_event_owner_lease
    ON inbound_event_ledger (processing_owner, lease_expires_at, id)
    WHERE processing_state = 'processing';

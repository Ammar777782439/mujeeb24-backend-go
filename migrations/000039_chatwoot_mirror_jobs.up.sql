CREATE TABLE chatwoot_mirror_jobs (
    id                       UUID PRIMARY KEY,
    business_id              UUID NOT NULL,
    communication_message_id UUID NOT NULL,
    status                   TEXT NOT NULL,
    attempt_count            INTEGER NOT NULL DEFAULT 0,
    lease_owner              TEXT,
    lease_token              TEXT,
    lease_expires_at         TIMESTAMPTZ,
    failure_code             TEXT,
    result_code              TEXT,
    chatwoot_contact_id      TEXT,
    chatwoot_conversation_id TEXT,
    chatwoot_message_id      TEXT,
    completed_at             TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL,
    updated_at               TIMESTAMPTZ NOT NULL,

    CONSTRAINT chatwoot_mirror_jobs_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT chatwoot_mirror_jobs_message_fk
        FOREIGN KEY (business_id, communication_message_id)
        REFERENCES communication_messages (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT chatwoot_mirror_jobs_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT chatwoot_mirror_jobs_message_uq
        UNIQUE (business_id, communication_message_id),
    CONSTRAINT chatwoot_mirror_jobs_status_chk
        CHECK (status IN ('pending', 'processing', 'completed', 'dead_letter')),
    CONSTRAINT chatwoot_mirror_jobs_attempt_count_chk
        CHECK (attempt_count >= 0),
    CONSTRAINT chatwoot_mirror_jobs_lease_chk
        CHECK (
            (status = 'processing' AND lease_owner IS NOT NULL AND lease_token IS NOT NULL AND lease_expires_at IS NOT NULL)
            OR
            (status <> 'processing' AND lease_owner IS NULL AND lease_token IS NULL AND lease_expires_at IS NULL)
        ),
    CONSTRAINT chatwoot_mirror_jobs_failure_chk
        CHECK (failure_code IS NULL OR length(btrim(failure_code)) > 0),
    CONSTRAINT chatwoot_mirror_jobs_result_chk
        CHECK (result_code IS NULL OR length(btrim(result_code)) > 0)
);

CREATE INDEX idx_chatwoot_mirror_jobs_claimable
    ON chatwoot_mirror_jobs (status, created_at ASC, id ASC)
    WHERE status = 'pending';

CREATE INDEX idx_chatwoot_mirror_jobs_business_status
    ON chatwoot_mirror_jobs (business_id, status, updated_at DESC, id DESC);

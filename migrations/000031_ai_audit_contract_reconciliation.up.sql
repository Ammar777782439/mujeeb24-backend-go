ALTER TABLE ai_decisions
    ADD COLUMN resource_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN human_review_reason TEXT,
    ADD COLUMN human_review_requested_at TIMESTAMPTZ,
    ADD COLUMN human_review_requested_by TEXT,
    ADD COLUMN decided_at TIMESTAMPTZ,
    ADD CONSTRAINT ai_decisions_resource_version_chk
        CHECK (resource_version > 0),
    ADD CONSTRAINT ai_decisions_human_review_reason_chk
        CHECK (human_review_reason IS NULL OR length(btrim(human_review_reason)) > 0),
    ADD CONSTRAINT ai_decisions_human_review_requested_by_chk
        CHECK (human_review_requested_by IS NULL OR length(btrim(human_review_requested_by)) > 0),
    ADD CONSTRAINT ai_decisions_human_review_pair_chk
        CHECK ((human_review_requested_at IS NULL AND human_review_requested_by IS NULL)
            OR (human_review_requested_at IS NOT NULL AND human_review_requested_by IS NOT NULL)),
    ADD CONSTRAINT ai_decisions_decided_at_chk
        CHECK (decided_at IS NULL OR decided_at >= created_at);

CREATE INDEX idx_ai_decisions_business_requires_human
    ON ai_decisions (business_id, requires_human, updated_at DESC, id DESC);

CREATE INDEX idx_ai_decisions_business_expires
    ON ai_decisions (business_id, expires_at, updated_at DESC, id DESC)
    WHERE expires_at IS NOT NULL;

ALTER TABLE audit_events
    ADD COLUMN decision_reference TEXT,
    ADD COLUMN result TEXT,
    ADD COLUMN reason_code TEXT,
    ADD COLUMN before_reference TEXT,
    ADD COLUMN after_reference TEXT,
    ADD COLUMN schema_version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN redaction_version INTEGER NOT NULL DEFAULT 1,
    ADD CONSTRAINT audit_events_decision_reference_chk
        CHECK (decision_reference IS NULL OR length(btrim(decision_reference)) > 0),
    ADD CONSTRAINT audit_events_result_chk
        CHECK (result IS NULL OR result IN ('accepted', 'rejected', 'failed', 'completed', 'skipped')),
    ADD CONSTRAINT audit_events_reason_code_chk
        CHECK (reason_code IS NULL OR length(btrim(reason_code)) > 0),
    ADD CONSTRAINT audit_events_before_reference_chk
        CHECK (before_reference IS NULL OR length(btrim(before_reference)) > 0),
    ADD CONSTRAINT audit_events_after_reference_chk
        CHECK (after_reference IS NULL OR length(btrim(after_reference)) > 0),
    ADD CONSTRAINT audit_events_schema_version_chk
        CHECK (schema_version > 0),
    ADD CONSTRAINT audit_events_redaction_version_chk
        CHECK (redaction_version > 0);

CREATE INDEX idx_audit_events_business_action
    ON audit_events (business_id, action, occurred_at DESC, id DESC);

CREATE INDEX idx_audit_events_business_actor
    ON audit_events (business_id, actor_type, occurred_at DESC, id DESC);

CREATE OR REPLACE FUNCTION reject_audit_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only';
END;
$$;

CREATE TRIGGER audit_events_append_only_trigger
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW
    EXECUTE FUNCTION reject_audit_event_mutation();

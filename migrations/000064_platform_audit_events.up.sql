-- Platform Administration Contract V1 — Platform Audit Events
-- Per docs/architecture/PlatformAdministrationContractV1.md §45-49
--
-- Platform Audit is SEPARATE from Merchant Audit (audit_events table).
-- Platform Audit records all Platform Command executions + sensitive reads.

CREATE TABLE platform_audit_events (
    id              UUID PRIMARY KEY,
    actor_platform_admin_id UUID,
    action          TEXT NOT NULL,
    target_type     TEXT NOT NULL,
    target_id       TEXT,
    business_id     UUID,
    result          TEXT NOT NULL DEFAULT 'SUCCESS',
    failure_code    TEXT,
    correlation_id  TEXT,
    occurred_at     TIMESTAMPTZ NOT NULL,
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL,

    CONSTRAINT platform_audit_action_not_blank_chk CHECK (length(btrim(action)) > 0),
    CONSTRAINT platform_audit_target_type_not_blank_chk CHECK (length(btrim(target_type)) > 0),
    CONSTRAINT platform_audit_result_chk CHECK (result IN ('SUCCESS', 'FAILURE', 'FORBIDDEN', 'NOT_FOUND', 'CONFLICT')),
    CONSTRAINT platform_audit_metadata_object_chk CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX idx_platform_audit_action ON platform_audit_events (action);
CREATE INDEX idx_platform_audit_actor ON platform_audit_events (actor_platform_admin_id);
CREATE INDEX idx_platform_audit_target ON platform_audit_events (target_type, target_id);
CREATE INDEX idx_platform_audit_business ON platform_audit_events (business_id);
CREATE INDEX idx_platform_audit_occurred_at ON platform_audit_events (occurred_at);
CREATE INDEX idx_platform_audit_result ON platform_audit_events (result);

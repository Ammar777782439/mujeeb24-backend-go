CREATE TABLE audit_events (
    id              UUID PRIMARY KEY,
    business_id     UUID NOT NULL,
    actor_type      TEXT NOT NULL,
    actor_reference TEXT,
    action          TEXT NOT NULL,
    resource_type   TEXT NOT NULL,
    resource_id     TEXT,
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    correlation_id  UUID,
    causation_id    UUID,
    occurred_at     TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,

    CONSTRAINT audit_events_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT audit_events_actor_type_chk
        CHECK (actor_type IN ('customer', 'human_agent', 'ai', 'automation', 'system', 'provider')),
    CONSTRAINT audit_events_action_chk
        CHECK (length(btrim(action)) > 0),
    CONSTRAINT audit_events_resource_type_chk
        CHECK (length(btrim(resource_type)) > 0),
    CONSTRAINT audit_events_metadata_object_chk
        CHECK (jsonb_typeof(metadata) = 'object')
);

CREATE INDEX idx_audit_events_business_occurred
    ON audit_events (business_id, occurred_at DESC, id DESC);

CREATE INDEX idx_audit_events_business_resource
    ON audit_events (business_id, resource_type, resource_id, occurred_at DESC)
    WHERE resource_id IS NOT NULL;

CREATE INDEX idx_audit_events_correlation
    ON audit_events (business_id, correlation_id)
    WHERE correlation_id IS NOT NULL;

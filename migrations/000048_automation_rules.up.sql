CREATE TABLE automation_rules (
    id               UUID PRIMARY KEY,
    business_id      UUID NOT NULL,
    name             TEXT NOT NULL,
    status           TEXT NOT NULL,
    trigger_kind     TEXT NOT NULL,
    conditions       JSONB NOT NULL DEFAULT '{}'::jsonb,
    action_kind      TEXT NOT NULL,
    action_payload   JSONB NOT NULL DEFAULT '{}'::jsonb,
    position         INTEGER NOT NULL DEFAULT 100,
    resource_version BIGINT NOT NULL DEFAULT 1,
    created_at       TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL,

    CONSTRAINT automation_rules_business_fk
        FOREIGN KEY (business_id) REFERENCES businesses(id) ON DELETE RESTRICT,
    CONSTRAINT automation_rules_name_not_blank_chk
        CHECK (length(btrim(name)) > 0),
    CONSTRAINT automation_rules_status_chk
        CHECK (status IN ('active', 'disabled')),
    CONSTRAINT automation_rules_trigger_kind_chk
        CHECK (trigger_kind IN ('inbound_message')),
    CONSTRAINT automation_rules_conditions_object_chk
        CHECK (jsonb_typeof(conditions) = 'object'),
    CONSTRAINT automation_rules_action_kind_chk
        CHECK (action_kind IN ('add_label', 'set_priority', 'assign_human')),
    CONSTRAINT automation_rules_action_payload_object_chk
        CHECK (jsonb_typeof(action_payload) = 'object'),
    CONSTRAINT automation_rules_position_positive_chk
        CHECK (position > 0),
    CONSTRAINT automation_rules_resource_version_chk
        CHECK (resource_version > 0)
);

CREATE INDEX idx_automation_rules_active_inbound
    ON automation_rules (business_id, trigger_kind, position, id)
    WHERE status = 'active';

CREATE TABLE automation_executions (
    id               UUID PRIMARY KEY,
    business_id      UUID NOT NULL,
    rule_id          UUID NOT NULL,
    inbound_event_id UUID NOT NULL,
    result           TEXT NOT NULL,
    reason_code      TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL,

    CONSTRAINT automation_executions_business_fk
        FOREIGN KEY (business_id) REFERENCES businesses(id) ON DELETE RESTRICT,
    CONSTRAINT automation_executions_rule_fk
        FOREIGN KEY (rule_id) REFERENCES automation_rules(id) ON DELETE RESTRICT,
    CONSTRAINT automation_executions_inbound_event_fk
        FOREIGN KEY (inbound_event_id) REFERENCES inbound_event_ledger(id) ON DELETE RESTRICT,
    CONSTRAINT automation_executions_result_chk
        CHECK (result IN ('processing', 'executed', 'skipped', 'failed')),
    CONSTRAINT automation_executions_reason_not_blank_chk
        CHECK (length(btrim(reason_code)) > 0),
    CONSTRAINT automation_executions_rule_event_uq
        UNIQUE (business_id, rule_id, inbound_event_id)
);

CREATE INDEX idx_automation_executions_business_created
    ON automation_executions (business_id, created_at DESC, id DESC);

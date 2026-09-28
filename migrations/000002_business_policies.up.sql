CREATE TABLE business_policies (
    business_id                     UUID PRIMARY KEY,
    ai_mode                         TEXT NOT NULL,
    default_human_review            BOOLEAN NOT NULL,
    allow_auto_reply                BOOLEAN NOT NULL,
    allow_auto_lead_creation        BOOLEAN NOT NULL,
    allow_auto_transaction_draft    BOOLEAN NOT NULL,
    allow_auto_confirmation         BOOLEAN NOT NULL,
    business_hours_reference        TEXT,
    escalation_policy_reference     TEXT,
    created_at                      TIMESTAMPTZ NOT NULL,
    updated_at                      TIMESTAMPTZ NOT NULL,

    CONSTRAINT business_policies_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT business_policies_ai_mode_chk
        CHECK (ai_mode IN ('disabled', 'assist', 'approval', 'restricted_auto'))
);

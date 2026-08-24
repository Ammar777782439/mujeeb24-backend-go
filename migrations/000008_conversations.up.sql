CREATE TABLE conversations (
    id                  UUID PRIMARY KEY,
    business_id         UUID NOT NULL,
    customer_id         UUID NOT NULL,
    state               TEXT NOT NULL,
    ownership           TEXT NOT NULL,
    ai_mode_override    TEXT,
    priority            TEXT NOT NULL,
    assignment_reference TEXT,
    last_activity_at    TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL,

    CONSTRAINT conversations_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT conversations_customer_fk
        FOREIGN KEY (business_id, customer_id)
        REFERENCES customers (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT conversations_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT conversations_state_chk
        CHECK (state IN (
            'open', 'ai_handling', 'waiting_customer',
            'waiting_human', 'human_handling', 'closed'
        )),
    CONSTRAINT conversations_ownership_chk
        CHECK (ownership IN ('none', 'ai', 'human')),
    CONSTRAINT conversations_ai_mode_chk
        CHECK (ai_mode_override IS NULL OR ai_mode_override IN ('allowed', 'draft_only', 'disabled')),
    CONSTRAINT conversations_priority_chk
        CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    CONSTRAINT conversations_assignment_ref_chk
        CHECK (assignment_reference IS NULL OR length(btrim(assignment_reference)) > 0)
);

CREATE INDEX idx_conversations_business_activity
    ON conversations (business_id, last_activity_at DESC);

CREATE INDEX idx_conversations_business_state
    ON conversations (business_id, state, updated_at DESC);

CREATE INDEX idx_conversations_business_customer
    ON conversations (business_id, customer_id, updated_at DESC);

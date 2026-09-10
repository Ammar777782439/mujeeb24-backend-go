-- Conversation state for generic reference resolution. Business+conversation scoped.
CREATE TABLE conversation_state (
    business_id     UUID NOT NULL,
    conversation_id UUID NOT NULL,
    focus           JSONB,
    previous        JSONB NOT NULL DEFAULT '[]'::jsonb,
    comparison      JSONB,
    preferences     JSONB NOT NULL DEFAULT '[]'::jsonb,
    constraints     JSONB NOT NULL DEFAULT '[]'::jsonb,
    pending         JSONB NOT NULL DEFAULT '[]'::jsonb,
    version         BIGINT NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (business_id, conversation_id),
    CONSTRAINT conversation_state_business_fk
        FOREIGN KEY (business_id) REFERENCES businesses(id) ON DELETE RESTRICT,
    CONSTRAINT conversation_state_conversation_fk
        FOREIGN KEY (business_id, conversation_id) REFERENCES conversations(business_id, id) ON DELETE RESTRICT,
    CONSTRAINT conversation_state_focus_object_chk
        CHECK (focus IS NULL OR jsonb_typeof(focus) = 'object'),
    CONSTRAINT conversation_state_previous_array_chk
        CHECK (jsonb_typeof(previous) = 'array'),
    CONSTRAINT conversation_state_comparison_object_chk
        CHECK (comparison IS NULL OR jsonb_typeof(comparison) = 'object'),
    CONSTRAINT conversation_state_preferences_array_chk
        CHECK (jsonb_typeof(preferences) = 'array'),
    CONSTRAINT conversation_state_constraints_array_chk
        CHECK (jsonb_typeof(constraints) = 'array'),
    CONSTRAINT conversation_state_pending_array_chk
        CHECK (jsonb_typeof(pending) = 'array'),
    CONSTRAINT conversation_state_version_chk
        CHECK (version > 0)
);

CREATE INDEX idx_conversation_state_business_updated
    ON conversation_state (business_id, updated_at DESC);

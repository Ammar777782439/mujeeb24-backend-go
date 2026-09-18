-- Merchant AI Sessions and Messages for merchant-to-assistant multi-turn interactions.

CREATE TABLE merchant_ai_sessions (
    id           UUID NOT NULL,
    business_id  UUID NOT NULL,
    principal_id UUID NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (business_id, id),
    CONSTRAINT merchant_ai_sessions_business_fk
        FOREIGN KEY (business_id) REFERENCES businesses(id) ON DELETE RESTRICT,
    CONSTRAINT merchant_ai_sessions_principal_fk
        FOREIGN KEY (principal_id) REFERENCES principals(id) ON DELETE RESTRICT
);

CREATE INDEX idx_merchant_ai_sessions_principal_updated
    ON merchant_ai_sessions (business_id, principal_id, updated_at DESC);

CREATE TABLE merchant_ai_messages (
    id          UUID NOT NULL,
    business_id UUID NOT NULL,
    session_id  UUID NOT NULL,
    sender_type VARCHAR(32) NOT NULL,
    text        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (business_id, id),
    CONSTRAINT merchant_ai_messages_session_fk
        FOREIGN KEY (business_id, session_id) REFERENCES merchant_ai_sessions(business_id, id) ON DELETE CASCADE,
    CONSTRAINT merchant_ai_messages_sender_type_chk
        CHECK (sender_type IN ('merchant', 'assistant'))
);

CREATE INDEX idx_merchant_ai_messages_session_created
    ON merchant_ai_messages (business_id, session_id, created_at ASC);

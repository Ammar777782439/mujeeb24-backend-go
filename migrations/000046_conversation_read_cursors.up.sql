CREATE TABLE conversation_read_cursors (
    business_id          UUID NOT NULL,
    conversation_id      UUID NOT NULL,
    principal_id         UUID NOT NULL,
    last_read_message_id UUID,
    read_at              TIMESTAMPTZ NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL,
    updated_at           TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (business_id, conversation_id, principal_id),
    CONSTRAINT conversation_read_cursors_business_fk
        FOREIGN KEY (business_id) REFERENCES businesses(id) ON DELETE RESTRICT,
    CONSTRAINT conversation_read_cursors_conversation_fk
        FOREIGN KEY (business_id, conversation_id) REFERENCES conversations(business_id, id) ON DELETE RESTRICT,
    CONSTRAINT conversation_read_cursors_membership_fk
        FOREIGN KEY (business_id, principal_id) REFERENCES business_memberships(business_id, principal_id) ON DELETE RESTRICT
);

CREATE INDEX idx_conversation_read_cursors_principal
    ON conversation_read_cursors (business_id, principal_id, updated_at DESC);

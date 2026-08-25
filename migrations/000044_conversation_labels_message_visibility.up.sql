ALTER TABLE communication_messages
    ADD COLUMN visibility TEXT NOT NULL DEFAULT 'public',
    ADD CONSTRAINT communication_messages_visibility_chk CHECK (visibility IN ('public', 'private'));

CREATE INDEX idx_communication_messages_visibility
    ON communication_messages (business_id, conversation_reference_id, visibility, occurred_at DESC, id DESC);

CREATE TABLE conversation_labels (
    business_id     UUID NOT NULL,
    conversation_id UUID NOT NULL,
    label           TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (business_id, conversation_id, label),
    CONSTRAINT conversation_labels_business_fk FOREIGN KEY (business_id) REFERENCES businesses(id) ON DELETE RESTRICT,
    CONSTRAINT conversation_labels_conversation_fk FOREIGN KEY (business_id, conversation_id) REFERENCES conversations(business_id, id) ON DELETE RESTRICT,
    CONSTRAINT conversation_labels_not_blank_chk CHECK (length(btrim(label)) > 0),
    CONSTRAINT conversation_labels_normalized_chk CHECK (label = lower(btrim(label)))
);

CREATE INDEX idx_conversation_labels_business_label
    ON conversation_labels (business_id, label, conversation_id);

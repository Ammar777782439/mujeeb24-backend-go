-- Sender identity is recorded only from the authenticated merchant actor.
-- Historical messages remain unknown; assignment is never used to invent a sender.
ALTER TABLE communication_messages
    ADD COLUMN author_principal_id UUID,
    ADD CONSTRAINT communication_messages_author_membership_fk
        FOREIGN KEY (business_id, author_principal_id)
        REFERENCES business_memberships (business_id, principal_id)
        ON DELETE RESTRICT;

CREATE INDEX idx_communication_messages_author
    ON communication_messages (business_id, author_principal_id)
    WHERE author_principal_id IS NOT NULL;

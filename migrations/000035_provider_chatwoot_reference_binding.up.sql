ALTER TABLE conversation_references
    ADD COLUMN chatwoot_account_id TEXT,
    ADD COLUMN chatwoot_inbox_id TEXT,
    ADD COLUMN chatwoot_conversation_id TEXT,
    ADD CONSTRAINT conversation_references_chatwoot_binding_chk CHECK (
        system <> 'provider'
        OR (
            (chatwoot_account_id IS NULL AND chatwoot_inbox_id IS NULL AND chatwoot_conversation_id IS NULL)
            OR (
                length(btrim(chatwoot_account_id)) > 0
                AND length(btrim(chatwoot_inbox_id)) > 0
                AND length(btrim(chatwoot_conversation_id)) > 0
            )
        )
    );

CREATE UNIQUE INDEX uq_provider_conversation_references_chatwoot_binding
    ON conversation_references (
        business_id,
        chatwoot_account_id,
        chatwoot_inbox_id,
        chatwoot_conversation_id
    )
    WHERE system = 'provider'
      AND is_current
      AND chatwoot_account_id IS NOT NULL
      AND chatwoot_inbox_id IS NOT NULL
      AND chatwoot_conversation_id IS NOT NULL;

CREATE INDEX idx_provider_conversation_references_chatwoot_lookup
    ON conversation_references (
        business_id,
        chatwoot_account_id,
        chatwoot_inbox_id,
        chatwoot_conversation_id,
        updated_at DESC
    )
    WHERE system = 'provider'
      AND is_current;

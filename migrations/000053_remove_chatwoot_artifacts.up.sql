-- 000053_remove_chatwoot_artifacts.up.sql
-- Complete removal of Chatwoot tables, columns, indexes, and constraints.
-- Mujeeb/PostgreSQL is the single source of truth; SocialAPI is transport only.

-- 1. Drop Chatwoot-only tables in reverse dependency order
DROP TABLE IF EXISTS chatwoot_contact_links CASCADE;
DROP TABLE IF EXISTS chatwoot_workspace_bindings CASCADE;
DROP TABLE IF EXISTS chatwoot_mirror_jobs CASCADE;

-- 2. Clean channel_provisioning_sessions: remove chatwoot columns
ALTER TABLE channel_provisioning_sessions
    DROP COLUMN IF EXISTS chatwoot_account_id,
    DROP COLUMN IF EXISTS chatwoot_inbox_id;

-- 3. Clean communication_messages: remove chatwoot message index, constraints, and column
DROP INDEX IF EXISTS uq_communication_messages_chatwoot_message;

ALTER TABLE communication_messages
    DROP CONSTRAINT IF EXISTS communication_messages_chatwoot_reference_chk,
    DROP CONSTRAINT IF EXISTS communication_messages_transport_chk,
    ADD CONSTRAINT communication_messages_transport_chk
        CHECK (transport IN ('provider', 'mujeeb')),
    DROP COLUMN IF EXISTS chatwoot_message_id;

-- 4. Clean conversation_references: remove chatwoot indexes, constraints, and columns
DROP INDEX IF EXISTS uq_provider_conversation_references_chatwoot_binding;
DROP INDEX IF EXISTS idx_provider_conversation_references_chatwoot_lookup;

ALTER TABLE conversation_references
    DROP CONSTRAINT IF EXISTS conversation_references_chatwoot_binding_chk,
    DROP CONSTRAINT IF EXISTS conversation_references_system_chk,
    ADD CONSTRAINT conversation_references_system_chk
        CHECK (system IN ('provider')),
    DROP CONSTRAINT IF EXISTS conversation_references_provider_connection_chk,
    ADD CONSTRAINT conversation_references_provider_connection_chk
        CHECK (system = 'provider' AND connection_id IS NOT NULL),
    DROP COLUMN IF EXISTS chatwoot_account_id,
    DROP COLUMN IF EXISTS chatwoot_inbox_id,
    DROP COLUMN IF EXISTS chatwoot_conversation_id;

-- 5. Clean inbound_event_ledger: remove chatwoot exception from resolution pair check
ALTER TABLE inbound_event_ledger
    DROP CONSTRAINT IF EXISTS inbound_event_resolution_pair_chk,
    ADD CONSTRAINT inbound_event_resolution_pair_chk
        CHECK (
            (business_id IS NULL AND connection_id IS NULL)
            OR (business_id IS NOT NULL AND connection_id IS NOT NULL)
        );

-- 6. Clean outbound_messages: remove chatwoot transport value and chatwoot message id column
ALTER TABLE outbound_messages
    DROP CONSTRAINT IF EXISTS outbound_messages_transport_chk,
    ADD CONSTRAINT outbound_messages_transport_chk
        CHECK (transport IN ('provider')),
    DROP COLUMN IF EXISTS chatwoot_message_id;

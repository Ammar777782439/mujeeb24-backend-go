DROP INDEX IF EXISTS idx_message_attachments_ingestion_queue;

ALTER TABLE message_attachments
    DROP CONSTRAINT IF EXISTS message_attachments_ingestion_error_code_chk,
    DROP CONSTRAINT IF EXISTS message_attachments_ingestion_attempts_chk,
    DROP COLUMN IF EXISTS ingestion_last_error_code,
    DROP COLUMN IF EXISTS ingestion_lease_until,
    DROP COLUMN IF EXISTS ingestion_next_attempt_at,
    DROP COLUMN IF EXISTS ingestion_attempts;

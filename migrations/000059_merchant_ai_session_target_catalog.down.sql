-- ADR-041 rollback.
ALTER TABLE merchant_ai_sessions
    DROP CONSTRAINT IF EXISTS merchant_ai_sessions_catalog_fk;
DROP INDEX IF EXISTS idx_merchant_ai_sessions_catalog;
ALTER TABLE merchant_ai_sessions
    DROP COLUMN IF EXISTS target_catalog_id,
    DROP COLUMN IF EXISTS target_catalog_set_at;

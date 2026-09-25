-- ADR-041: Catalog Selection deterministic layer 2 — sticky session catalog.
--
-- Per ADR-041, when the merchant picks a catalog in the dashboard (or
-- confirms an auto-select), that catalog_id is stored in the session
-- so subsequent turns don't re-ask. This avoids forcing the merchant
-- to re-pick the catalog on every turn.
--
-- NULL = no sticky catalog set yet (will go through layers 1, 3, 4).
-- ON DELETE SET NULL = if the catalog is deleted, the session forgets
-- the sticky and falls back to asking the merchant.

ALTER TABLE merchant_ai_sessions
    ADD COLUMN IF NOT EXISTS target_catalog_id UUID,
    ADD COLUMN IF NOT EXISTS target_catalog_set_at TIMESTAMPTZ;

ALTER TABLE merchant_ai_sessions
    ADD CONSTRAINT merchant_ai_sessions_catalog_fk
        FOREIGN KEY (business_id, target_catalog_id)
        REFERENCES catalogs(business_id, id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_merchant_ai_sessions_catalog
    ON merchant_ai_sessions (business_id, target_catalog_id);

DROP INDEX IF EXISTS idx_offers_ai_bulk;
DROP INDEX IF EXISTS idx_variants_ai_bulk;
DROP INDEX IF EXISTS idx_catalog_items_ai_full_scan;
DROP INDEX IF EXISTS idx_offers_ai_search;
DROP INDEX IF EXISTS idx_variants_ai_search;
DROP INDEX IF EXISTS idx_catalog_items_ai_search;

ALTER TABLE offers DROP COLUMN IF EXISTS ai_search_document;
ALTER TABLE variants DROP COLUMN IF EXISTS ai_search_document;
ALTER TABLE catalog_items DROP COLUMN IF EXISTS ai_search_document;

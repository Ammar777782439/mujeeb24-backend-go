-- Universal Catalog AI v3
-- Generic search documents for cross-vertical AI retrieval.
-- No vertical-specific vocabulary is encoded here.

ALTER TABLE catalog_items
    ADD COLUMN ai_search_document tsvector
    GENERATED ALWAYS AS (
        to_tsvector(
            'simple'::regconfig,
            coalesce(item_type, '') || ' ' ||
            coalesce(name, '') || ' ' ||
            coalesce(short_description, '') || ' ' ||
            coalesce(long_description, '') || ' ' ||
            coalesce(pricing_mode, '') || ' ' ||
            coalesce(availability_mode, '') || ' ' ||
            coalesce(fulfillment_mode, '') || ' ' ||
            coalesce(attributes::text, '')
        )
    ) STORED;

ALTER TABLE variants
    ADD COLUMN ai_search_document tsvector
    GENERATED ALWAYS AS (
        to_tsvector(
            'simple'::regconfig,
            coalesce(name, '') || ' ' || coalesce(attributes::text, '')
        )
    ) STORED;

ALTER TABLE offers
    ADD COLUMN ai_search_document tsvector
    GENERATED ALWAYS AS (
        to_tsvector(
            'simple'::regconfig,
            coalesce(name, '') || ' ' ||
            coalesce(pricing_mode, '') || ' ' ||
            coalesce(amount::text, '') || ' ' ||
            coalesce(currency, '') || ' ' ||
            coalesce(pricing_unit, '') || ' ' ||
            coalesce(price_source, '') || ' ' ||
            coalesce(price_verification_status, '') || ' ' ||
            coalesce(availability_mode, '') || ' ' ||
            coalesce(availability_status, '') || ' ' ||
            coalesce(availability_source, '') || ' ' ||
            coalesce(fulfillment_mode, '') || ' ' ||
            coalesce(status, '')
        )
    ) STORED;

CREATE INDEX idx_catalog_items_ai_search
    ON catalog_items USING GIN (ai_search_document);

CREATE INDEX idx_variants_ai_search
    ON variants USING GIN (ai_search_document);

CREATE INDEX idx_offers_ai_search
    ON offers USING GIN (ai_search_document);

CREATE INDEX idx_catalog_items_ai_full_scan
    ON catalog_items (business_id, status, id)
    WHERE status = 'active';

CREATE INDEX idx_variants_ai_bulk
    ON variants (business_id, catalog_item_id, status, id)
    WHERE status = 'active';

CREATE INDEX idx_offers_ai_bulk
    ON offers (business_id, catalog_item_id, status, id)
    WHERE status = 'active';

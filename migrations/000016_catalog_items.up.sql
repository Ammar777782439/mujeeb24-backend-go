CREATE TABLE catalog_items (
    id                    UUID PRIMARY KEY,
    business_id           UUID NOT NULL,
    catalog_id            UUID NOT NULL,
    attribute_schema_id   UUID,
    attribute_schema_version INTEGER,
    item_type             TEXT NOT NULL,
    name                  TEXT NOT NULL,
    short_description     TEXT,
    long_description      TEXT,
    status                TEXT NOT NULL,
    pricing_mode          TEXT NOT NULL,
    availability_mode     TEXT NOT NULL,
    fulfillment_mode      TEXT NOT NULL,
    requires_confirmation BOOLEAN NOT NULL,
    attributes            JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at            TIMESTAMPTZ NOT NULL,
    updated_at            TIMESTAMPTZ NOT NULL,

    CONSTRAINT catalog_items_catalog_fk
        FOREIGN KEY (business_id, catalog_id)
        REFERENCES catalogs (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT catalog_items_attribute_schema_fk
        FOREIGN KEY (business_id, attribute_schema_id)
        REFERENCES attribute_schemas (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT catalog_items_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT catalog_items_schema_version_pair_chk
        CHECK ((attribute_schema_id IS NULL AND attribute_schema_version IS NULL)
            OR (attribute_schema_id IS NOT NULL AND attribute_schema_version IS NOT NULL AND attribute_schema_version > 0)),
    CONSTRAINT catalog_items_type_chk
        CHECK (length(btrim(item_type)) > 0),
    CONSTRAINT catalog_items_name_chk
        CHECK (length(btrim(name)) > 0),
    CONSTRAINT catalog_items_status_chk
        CHECK (status IN ('draft', 'active', 'inactive', 'archived')),
    CONSTRAINT catalog_items_pricing_mode_chk
        CHECK (pricing_mode IN (
            'fixed', 'starting_from', 'per_unit', 'per_person',
            'per_day', 'quote_required', 'dynamic'
        )),
    CONSTRAINT catalog_items_availability_mode_chk
        CHECK (availability_mode IN (
            'stock', 'schedule', 'supplier_check', 'always_available', 'unknown'
        )),
    CONSTRAINT catalog_items_fulfillment_mode_chk
        CHECK (fulfillment_mode IN (
            'delivery', 'pickup', 'digital', 'appointment', 'travel', 'manual'
        )),
    CONSTRAINT catalog_items_attributes_object_chk
        CHECK (jsonb_typeof(attributes) = 'object')
);

CREATE INDEX idx_catalog_items_business_status
    ON catalog_items (business_id, catalog_id, status, updated_at DESC);

CREATE INDEX idx_catalog_items_business_name
    ON catalog_items (business_id, catalog_id, name);

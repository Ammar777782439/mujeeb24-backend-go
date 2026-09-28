CREATE TABLE variants (
    id              UUID PRIMARY KEY,
    business_id     UUID NOT NULL,
    catalog_item_id UUID NOT NULL,
    name            TEXT NOT NULL,
    attributes      JSONB NOT NULL,
    status          TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,

    CONSTRAINT variants_catalog_item_fk
        FOREIGN KEY (business_id, catalog_item_id)
        REFERENCES catalog_items (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT variants_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT variants_business_item_id_uq
        UNIQUE (business_id, catalog_item_id, id),
    CONSTRAINT variants_name_chk
        CHECK (length(btrim(name)) > 0),
    CONSTRAINT variants_attributes_object_chk
        CHECK (jsonb_typeof(attributes) = 'object'),
    CONSTRAINT variants_status_chk
        CHECK (status IN ('active', 'inactive', 'archived'))
);

CREATE INDEX idx_variants_business_item_status
    ON variants (business_id, catalog_item_id, status, updated_at DESC);

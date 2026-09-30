ALTER TABLE catalogs
    ADD COLUMN resource_version BIGINT NOT NULL DEFAULT 1,
    ADD CONSTRAINT catalogs_resource_version_chk CHECK (resource_version > 0);

ALTER TABLE catalog_items
    ADD COLUMN resource_version BIGINT NOT NULL DEFAULT 1,
    ADD CONSTRAINT catalog_items_resource_version_chk CHECK (resource_version > 0);

ALTER TABLE offers
    ADD COLUMN resource_version BIGINT NOT NULL DEFAULT 1,
    ADD CONSTRAINT offers_resource_version_chk CHECK (resource_version > 0);

ALTER TABLE variants
    ADD COLUMN resource_version BIGINT NOT NULL DEFAULT 1,
    ADD CONSTRAINT variants_resource_version_chk CHECK (resource_version > 0);

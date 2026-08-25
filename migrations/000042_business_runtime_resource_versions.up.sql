ALTER TABLE businesses
    ADD COLUMN resource_version BIGINT NOT NULL DEFAULT 1,
    ADD CONSTRAINT businesses_resource_version_positive_chk CHECK (resource_version > 0);

ALTER TABLE business_policies
    ADD COLUMN resource_version BIGINT NOT NULL DEFAULT 1,
    ADD CONSTRAINT business_policies_resource_version_positive_chk CHECK (resource_version > 0);

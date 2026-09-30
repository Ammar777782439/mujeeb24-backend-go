ALTER TABLE customers
    ADD COLUMN resource_version BIGINT NOT NULL DEFAULT 1,
    ADD CONSTRAINT customers_resource_version_positive_chk CHECK (resource_version > 0);

ALTER TABLE conversations
    ADD COLUMN resource_version BIGINT NOT NULL DEFAULT 1,
    ADD CONSTRAINT conversations_resource_version_positive_chk CHECK (resource_version > 0);

ALTER TABLE channel_connections
    ADD COLUMN resource_version BIGINT NOT NULL DEFAULT 1,
    ADD CONSTRAINT channel_connections_resource_version_positive_chk CHECK (resource_version > 0);

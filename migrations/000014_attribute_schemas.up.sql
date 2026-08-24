CREATE TABLE attribute_schemas (
    id          UUID PRIMARY KEY,
    business_id UUID NOT NULL,
    name        TEXT NOT NULL,
    version     INTEGER NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL,

    CONSTRAINT attribute_schemas_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT attribute_schemas_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT attribute_schemas_name_chk
        CHECK (length(btrim(name)) > 0),
    CONSTRAINT attribute_schemas_version_chk
        CHECK (version > 0),
    CONSTRAINT attribute_schemas_version_uq
        UNIQUE (business_id, name, version)
);

CREATE INDEX idx_attribute_schemas_business
    ON attribute_schemas (business_id, name, version DESC);

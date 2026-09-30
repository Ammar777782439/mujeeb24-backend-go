CREATE TABLE catalogs (
    id          UUID PRIMARY KEY,
    business_id UUID NOT NULL,
    name        TEXT NOT NULL,
    description TEXT,
    status      TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL,

    CONSTRAINT catalogs_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT catalogs_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT catalogs_name_chk
        CHECK (length(btrim(name)) > 0),
    CONSTRAINT catalogs_status_chk
        CHECK (status IN ('draft', 'active', 'archived'))
);

CREATE INDEX idx_catalogs_business_status
    ON catalogs (business_id, status, updated_at DESC);

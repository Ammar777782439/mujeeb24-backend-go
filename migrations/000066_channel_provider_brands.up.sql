CREATE TABLE channel_provider_brands (
    id                 UUID PRIMARY KEY,
    business_id        UUID NOT NULL,
    provider_ref       TEXT NOT NULL,
    provider_brand_ref TEXT NOT NULL,
    display_name       TEXT NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL,

    CONSTRAINT channel_provider_brands_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,

    CONSTRAINT channel_provider_brands_provider_chk
        CHECK (length(btrim(provider_ref)) > 0),

    CONSTRAINT channel_provider_brands_ref_chk
        CHECK (length(btrim(provider_brand_ref)) > 0),

    CONSTRAINT channel_provider_brands_name_chk
        CHECK (length(btrim(display_name)) > 0),

    CONSTRAINT channel_provider_brands_business_provider_uq
        UNIQUE (business_id, provider_ref),

    CONSTRAINT channel_provider_brands_provider_brand_uq
        UNIQUE (provider_ref, provider_brand_ref)
);

CREATE INDEX idx_channel_provider_brands_business
    ON channel_provider_brands (business_id, provider_ref);

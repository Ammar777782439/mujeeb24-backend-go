CREATE TABLE customers (
    id                      UUID PRIMARY KEY,
    business_id             UUID NOT NULL,
    profile                 JSONB NOT NULL DEFAULT '{}'::jsonb,
    contact_points         JSONB NOT NULL DEFAULT '[]'::jsonb,
    locale_preference       TEXT,
    status                  TEXT NOT NULL,
    merged_into_customer_id UUID,
    created_at              TIMESTAMPTZ NOT NULL,
    updated_at              TIMESTAMPTZ NOT NULL,

    CONSTRAINT customers_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT customers_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT customers_status_chk
        CHECK (status IN ('active', 'merged', 'archived')),
    CONSTRAINT customers_merged_target_chk
        CHECK (status <> 'merged' OR merged_into_customer_id IS NOT NULL),
    CONSTRAINT customers_profile_object_chk
        CHECK (jsonb_typeof(profile) = 'object'),
    CONSTRAINT customers_contact_points_array_chk
        CHECK (jsonb_typeof(contact_points) = 'array'),
    CONSTRAINT customers_merge_same_business_fk
        FOREIGN KEY (business_id, merged_into_customer_id)
        REFERENCES customers (business_id, id)
        ON DELETE RESTRICT
);

CREATE INDEX idx_customers_business_status
    ON customers (business_id, status, updated_at DESC);

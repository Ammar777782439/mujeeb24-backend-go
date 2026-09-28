CREATE TABLE offers (
    id                           UUID PRIMARY KEY,
    business_id                  UUID NOT NULL,
    catalog_item_id              UUID NOT NULL,
    variant_id                   UUID,
    name                         TEXT NOT NULL,
    pricing_mode                 TEXT NOT NULL,
    amount                       NUMERIC(20, 4),
    currency                     CHAR(3),
    pricing_unit                 TEXT,
    price_source                 TEXT,
    price_verification_status    TEXT NOT NULL,
    price_checked_at             TIMESTAMPTZ,
    availability_mode            TEXT NOT NULL,
    availability_status          TEXT NOT NULL,
    availability_source          TEXT,
    availability_checked_at      TIMESTAMPTZ,
    availability_valid_until     TIMESTAMPTZ,
    availability_evidence_ref    TEXT,
    fulfillment_mode             TEXT NOT NULL,
    validity_from                TIMESTAMPTZ,
    validity_until               TIMESTAMPTZ,
    status                       TEXT NOT NULL,
    created_at                   TIMESTAMPTZ NOT NULL,
    updated_at                   TIMESTAMPTZ NOT NULL,

    CONSTRAINT offers_catalog_item_fk
        FOREIGN KEY (business_id, catalog_item_id)
        REFERENCES catalog_items (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT offers_variant_same_item_fk
        FOREIGN KEY (business_id, catalog_item_id, variant_id)
        REFERENCES variants (business_id, catalog_item_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT offers_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT offers_business_item_id_uq
        UNIQUE (business_id, catalog_item_id, id),
    CONSTRAINT offers_name_chk
        CHECK (length(btrim(name)) > 0),
    CONSTRAINT offers_pricing_mode_chk
        CHECK (pricing_mode IN (
            'fixed', 'starting_from', 'per_unit', 'per_person',
            'per_day', 'quote_required', 'dynamic'
        )),
    CONSTRAINT offers_amount_chk
        CHECK (amount IS NULL OR amount >= 0),
    CONSTRAINT offers_currency_chk
        CHECK (currency IS NULL OR currency ~ '^[A-Z]{3}$'),
    CONSTRAINT offers_pricing_consistency_chk
        CHECK (
            (pricing_mode IN ('fixed', 'starting_from') AND amount IS NOT NULL AND currency IS NOT NULL)
            OR (pricing_mode IN ('per_unit', 'per_person', 'per_day') AND amount IS NOT NULL AND currency IS NOT NULL AND pricing_unit IS NOT NULL)
            OR (pricing_mode = 'quote_required')
            OR (pricing_mode = 'dynamic' AND (amount IS NULL OR currency IS NOT NULL))
        ),
    CONSTRAINT offers_price_verification_chk
        CHECK (price_verification_status IN ('unverified', 'verified', 'stale', 'rejected')),
    CONSTRAINT offers_availability_mode_chk
        CHECK (availability_mode IN ('stock', 'schedule', 'supplier_check', 'always_available', 'unknown')),
    CONSTRAINT offers_availability_status_chk
        CHECK (availability_status IN ('available', 'unavailable', 'unknown', 'requires_check', 'stale')),
    CONSTRAINT offers_fulfillment_mode_chk
        CHECK (fulfillment_mode IN ('delivery', 'pickup', 'digital', 'appointment', 'travel', 'manual')),
    CONSTRAINT offers_status_chk
        CHECK (status IN ('draft', 'active', 'inactive', 'expired', 'archived')),
    CONSTRAINT offers_validity_chk
        CHECK (validity_until IS NULL OR validity_from IS NULL OR validity_until >= validity_from),
    CONSTRAINT offers_availability_validity_chk
        CHECK (
            availability_valid_until IS NULL
            OR availability_checked_at IS NULL
            OR availability_valid_until >= availability_checked_at
        )
);

CREATE INDEX idx_offers_business_item_status
    ON offers (business_id, catalog_item_id, status, updated_at DESC);

CREATE INDEX idx_offers_business_validity
    ON offers (business_id, validity_from, validity_until);

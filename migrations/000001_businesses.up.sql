CREATE TABLE businesses (
    id              UUID PRIMARY KEY,
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL,
    status          TEXT NOT NULL,
    vertical_type   TEXT NOT NULL,
    timezone        TEXT NOT NULL,
    default_currency CHAR(3) NOT NULL,
    locale          TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,

    CONSTRAINT businesses_name_not_blank_chk
        CHECK (length(btrim(name)) > 0),
    CONSTRAINT businesses_slug_not_blank_chk
        CHECK (length(btrim(slug)) > 0),
    CONSTRAINT businesses_status_chk
        CHECK (status IN ('pending_setup', 'active', 'suspended', 'archived')),
    CONSTRAINT businesses_vertical_type_chk
        CHECK (vertical_type IN (
            'retail', 'travel', 'services', 'restaurant', 'clinic',
            'hospitality', 'education', 'real_estate', 'other'
        )),
    CONSTRAINT businesses_currency_chk
        CHECK (default_currency ~ '^[A-Z]{3}$'),
    CONSTRAINT businesses_locale_not_blank_chk
        CHECK (length(btrim(locale)) > 0),
    CONSTRAINT businesses_slug_uq UNIQUE (slug)
);

CREATE INDEX idx_businesses_status
    ON businesses (status);

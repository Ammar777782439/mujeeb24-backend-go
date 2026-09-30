CREATE TABLE external_identities (
    id                           UUID PRIMARY KEY,
    business_id                  UUID NOT NULL,
    connection_id                UUID NOT NULL,
    customer_id                 UUID,
    provider_ref                 TEXT NOT NULL,
    channel                     TEXT NOT NULL,
    external_account_ref         TEXT,
    external_user_id             TEXT NOT NULL,
    profile_snapshot_reference   TEXT,
    link_status                  TEXT NOT NULL,
    first_seen_at                TIMESTAMPTZ NOT NULL,
    last_seen_at                 TIMESTAMPTZ NOT NULL,
    created_at                   TIMESTAMPTZ NOT NULL,
    updated_at                   TIMESTAMPTZ NOT NULL,

    CONSTRAINT external_identities_connection_fk
        FOREIGN KEY (business_id, connection_id)
        REFERENCES channel_connections (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT external_identities_customer_fk
        FOREIGN KEY (business_id, customer_id)
        REFERENCES customers (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT external_identities_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT external_identities_connection_user_uq
        UNIQUE (connection_id, external_user_id),
    CONSTRAINT external_identities_provider_ref_chk
        CHECK (length(btrim(provider_ref)) > 0),
    CONSTRAINT external_identities_channel_chk
        CHECK (channel IN ('facebook', 'instagram', 'whatsapp')),
    CONSTRAINT external_identities_user_ref_chk
        CHECK (length(btrim(external_user_id)) > 0),
    CONSTRAINT external_identities_link_status_chk
        CHECK (link_status IN ('unresolved', 'linked', 'revoked')),
    CONSTRAINT external_identities_link_consistency_chk
        CHECK (
            (link_status = 'linked' AND customer_id IS NOT NULL)
            OR (link_status IN ('unresolved', 'revoked'))
        )
);

CREATE INDEX idx_external_identities_business_customer
    ON external_identities (business_id, customer_id);

CREATE INDEX idx_external_identities_business_status
    ON external_identities (business_id, link_status, last_seen_at DESC);

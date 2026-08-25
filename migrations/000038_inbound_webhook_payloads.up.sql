CREATE TABLE inbound_webhook_payloads (
    id            UUID PRIMARY KEY,
    provider_ref  TEXT NOT NULL,
    delivery_id   TEXT NOT NULL,
    payload       BYTEA NOT NULL,
    payload_hash  TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,

    CONSTRAINT inbound_webhook_payloads_provider_chk
        CHECK (length(btrim(provider_ref)) > 0),
    CONSTRAINT inbound_webhook_payloads_delivery_chk
        CHECK (length(btrim(delivery_id)) > 0),
    CONSTRAINT inbound_webhook_payloads_payload_chk
        CHECK (octet_length(payload) > 0),
    CONSTRAINT inbound_webhook_payloads_hash_chk
        CHECK (length(btrim(payload_hash)) = 64),
    CONSTRAINT inbound_webhook_payloads_provider_delivery_uq
        UNIQUE (provider_ref, delivery_id)
);

CREATE INDEX idx_inbound_webhook_payloads_created
    ON inbound_webhook_payloads (created_at DESC);

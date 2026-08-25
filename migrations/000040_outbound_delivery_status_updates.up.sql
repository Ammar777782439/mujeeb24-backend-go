CREATE TABLE outbound_delivery_status_updates (
    id                  UUID PRIMARY KEY,
    business_id         UUID NOT NULL,
    inbound_event_id    UUID NOT NULL,
    outbound_message_id UUID,
    provider_ref        TEXT NOT NULL,
    provider_message_id TEXT NOT NULL,
    delivery_status     TEXT NOT NULL,
    occurred_at         TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL,

    CONSTRAINT outbound_delivery_updates_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT outbound_delivery_updates_event_fk
        FOREIGN KEY (business_id, inbound_event_id)
        REFERENCES inbound_event_ledger (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT outbound_delivery_updates_message_fk
        FOREIGN KEY (business_id, outbound_message_id)
        REFERENCES outbound_messages (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT outbound_delivery_updates_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT outbound_delivery_updates_event_uq
        UNIQUE (business_id, inbound_event_id),
    CONSTRAINT outbound_delivery_updates_provider_chk
        CHECK (length(btrim(provider_ref)) > 0),
    CONSTRAINT outbound_delivery_updates_message_chk
        CHECK (length(btrim(provider_message_id)) > 0),
    CONSTRAINT outbound_delivery_updates_status_chk
        CHECK (delivery_status IN ('accepted', 'sent', 'delivered', 'read', 'failed'))
);

CREATE INDEX idx_outbound_delivery_updates_message
    ON outbound_delivery_status_updates (business_id, outbound_message_id, occurred_at DESC)
    WHERE outbound_message_id IS NOT NULL;

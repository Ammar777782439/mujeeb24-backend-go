CREATE TABLE channel_connection_state_events (
    id                    UUID PRIMARY KEY,
    business_id           UUID NOT NULL,
    connection_id         UUID NOT NULL,
    action                TEXT NOT NULL,
    from_status           TEXT NOT NULL,
    to_status             TEXT NOT NULL,
    reason                TEXT NOT NULL,
    actor_reference       TEXT NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL,
    CONSTRAINT channel_connection_state_events_business_fk FOREIGN KEY (business_id) REFERENCES businesses(id) ON DELETE RESTRICT,
    CONSTRAINT channel_connection_state_events_connection_fk FOREIGN KEY (business_id, connection_id) REFERENCES channel_connections(business_id, id) ON DELETE RESTRICT,
    CONSTRAINT channel_connection_state_events_action_chk CHECK (action IN ('reconnect_requested', 'disconnect_requested')),
    CONSTRAINT channel_connection_state_events_reason_chk CHECK (length(btrim(reason)) > 0),
    CONSTRAINT channel_connection_state_events_actor_chk CHECK (length(btrim(actor_reference)) > 0)
);

CREATE INDEX idx_channel_connection_state_events_connection
    ON channel_connection_state_events (business_id, connection_id, created_at DESC);

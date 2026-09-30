CREATE TABLE channel_connection_capabilities (
    connection_id   UUID NOT NULL,
    capability      TEXT NOT NULL,
    enabled         BOOLEAN NOT NULL,
    checked_at      TIMESTAMPTZ NOT NULL,
    evidence_source TEXT,

    CONSTRAINT channel_connection_capabilities_pk
        PRIMARY KEY (connection_id, capability),
    CONSTRAINT channel_connection_capabilities_connection_fk
        FOREIGN KEY (connection_id)
        REFERENCES channel_connections (id)
        ON DELETE RESTRICT,
    CONSTRAINT channel_connection_capabilities_capability_chk
        CHECK (capability IN (
            'receive_messages', 'send_messages', 'receive_comments',
            'reply_comments', 'private_reply', 'media_inbound',
            'media_outbound', 'interactive_messages', 'templates',
            'delivery_status', 'read_status'
        )),
    CONSTRAINT channel_connection_capabilities_evidence_chk
        CHECK (evidence_source IS NULL OR length(btrim(evidence_source)) > 0)
);

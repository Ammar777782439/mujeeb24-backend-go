ALTER TABLE inbound_event_ledger
    DROP CONSTRAINT inbound_event_resolution_pair_chk,
    ADD CONSTRAINT inbound_event_resolution_pair_chk CHECK (
        (business_id IS NULL AND connection_id IS NULL)
        OR (business_id IS NOT NULL AND (connection_id IS NOT NULL OR provider_ref = 'chatwoot'))
    );

CREATE TABLE chatwoot_workspace_bindings (
    id                  UUID PRIMARY KEY,
    business_id         UUID NOT NULL,
    route_key           TEXT NOT NULL,
    account_id          TEXT NOT NULL,
    inbox_id            TEXT NOT NULL,
    channel             TEXT NOT NULL,
    active              BOOLEAN NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL,

    CONSTRAINT chatwoot_workspace_bindings_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT chatwoot_workspace_bindings_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT chatwoot_workspace_bindings_route_uq
        UNIQUE (route_key),
    CONSTRAINT chatwoot_workspace_bindings_account_inbox_uq
        UNIQUE (account_id, inbox_id),
    CONSTRAINT chatwoot_workspace_bindings_route_chk
        CHECK (length(btrim(route_key)) > 0),
    CONSTRAINT chatwoot_workspace_bindings_account_chk
        CHECK (length(btrim(account_id)) > 0),
    CONSTRAINT chatwoot_workspace_bindings_inbox_chk
        CHECK (length(btrim(inbox_id)) > 0),
    CONSTRAINT chatwoot_workspace_bindings_channel_chk
        CHECK (channel IN ('facebook', 'instagram', 'whatsapp', 'other'))
);

CREATE INDEX idx_chatwoot_workspace_bindings_business_active
    ON chatwoot_workspace_bindings (business_id, active);

CREATE TABLE chatwoot_contact_links (
    id                  UUID PRIMARY KEY,
    binding_id          UUID NOT NULL,
    business_id         UUID NOT NULL,
    customer_id         UUID NOT NULL,
    external_user_id    TEXT NOT NULL,
    chatwoot_contact_id TEXT,
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL,

    CONSTRAINT chatwoot_contact_links_binding_fk
        FOREIGN KEY (binding_id)
        REFERENCES chatwoot_workspace_bindings (id)
        ON DELETE RESTRICT,
    CONSTRAINT chatwoot_contact_links_binding_business_fk
        FOREIGN KEY (business_id, binding_id)
        REFERENCES chatwoot_workspace_bindings (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT chatwoot_contact_links_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT chatwoot_contact_links_customer_fk
        FOREIGN KEY (business_id, customer_id)
        REFERENCES customers (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT chatwoot_contact_links_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT chatwoot_contact_links_binding_user_uq
        UNIQUE (binding_id, external_user_id),
    CONSTRAINT chatwoot_contact_links_chatwoot_contact_uq
        UNIQUE (binding_id, chatwoot_contact_id),
    CONSTRAINT chatwoot_contact_links_user_chk
        CHECK (length(btrim(external_user_id)) > 0),
    CONSTRAINT chatwoot_contact_links_contact_chk
        CHECK (chatwoot_contact_id IS NULL OR length(btrim(chatwoot_contact_id)) > 0)
);

CREATE INDEX idx_chatwoot_contact_links_business_customer
    ON chatwoot_contact_links (business_id, customer_id);

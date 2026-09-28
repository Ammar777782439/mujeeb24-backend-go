CREATE TABLE lead_attributions (
    id                            UUID PRIMARY KEY,
    business_id                   UUID NOT NULL,
    lead_id                       UUID NOT NULL,
    source_conversation_id        UUID,
    source_channel                TEXT,
    source_interaction_reference  TEXT,
    catalog_item_id              UUID,
    offer_id                     UUID,
    campaign_reference           TEXT,
    captured_at                  TIMESTAMPTZ NOT NULL,

    CONSTRAINT lead_attributions_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT lead_attributions_lead_fk
        FOREIGN KEY (business_id, lead_id)
        REFERENCES leads (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT lead_attributions_conversation_fk
        FOREIGN KEY (business_id, source_conversation_id)
        REFERENCES conversations (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT lead_attributions_catalog_item_fk
        FOREIGN KEY (business_id, catalog_item_id)
        REFERENCES catalog_items (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT lead_attributions_offer_fk
        FOREIGN KEY (business_id, offer_id)
        REFERENCES offers (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT lead_attributions_source_channel_chk
        CHECK (source_channel IS NULL OR source_channel IN ('facebook', 'instagram', 'whatsapp'))
);

CREATE INDEX idx_lead_attributions_business_lead
    ON lead_attributions (business_id, lead_id, captured_at DESC);

CREATE INDEX idx_lead_attributions_business_conversation
    ON lead_attributions (business_id, source_conversation_id, captured_at DESC)
    WHERE source_conversation_id IS NOT NULL;

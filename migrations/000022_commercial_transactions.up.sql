CREATE TABLE commercial_transactions (
    id                              UUID PRIMARY KEY,
    business_id                     UUID NOT NULL,
    customer_id                     UUID NOT NULL,
    lead_id                         UUID,
    transaction_type                TEXT NOT NULL,
    state                           TEXT NOT NULL,
    source_conversation_reference_id UUID,
    currency                        CHAR(3),
    total_amount                    NUMERIC(20, 4),
    schema_version                  INTEGER NOT NULL,
    created_at                      TIMESTAMPTZ NOT NULL,
    updated_at                      TIMESTAMPTZ NOT NULL,

    CONSTRAINT commercial_transactions_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT commercial_transactions_customer_fk
        FOREIGN KEY (business_id, customer_id)
        REFERENCES customers (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT commercial_transactions_lead_fk
        FOREIGN KEY (business_id, lead_id)
        REFERENCES leads (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT commercial_transactions_conversation_reference_fk
        FOREIGN KEY (business_id, source_conversation_reference_id)
        REFERENCES conversation_references (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT commercial_transactions_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT commercial_transactions_type_chk
        CHECK (transaction_type IN (
            'order', 'booking', 'appointment', 'service_request',
            'reservation', 'quote', 'subscription'
        )),
    CONSTRAINT commercial_transactions_state_chk
        CHECK (state IN (
            'draft', 'needs_information', 'awaiting_confirmation',
            'confirmed', 'in_fulfillment', 'completed', 'cancelled',
            'expired', 'rejected'
        )),
    CONSTRAINT commercial_transactions_amount_chk
        CHECK (total_amount IS NULL OR total_amount >= 0),
    CONSTRAINT commercial_transactions_currency_chk
        CHECK (currency IS NULL OR currency ~ '^[A-Z]{3}$'),
    CONSTRAINT commercial_transactions_amount_currency_chk
        CHECK (total_amount IS NULL OR currency IS NOT NULL),
    CONSTRAINT commercial_transactions_schema_version_chk
        CHECK (schema_version > 0)
);

CREATE INDEX idx_commercial_transactions_business_state
    ON commercial_transactions (business_id, state, updated_at DESC);

CREATE INDEX idx_commercial_transactions_business_customer
    ON commercial_transactions (business_id, customer_id, created_at DESC);

CREATE INDEX idx_commercial_transactions_business_lead
    ON commercial_transactions (business_id, lead_id, created_at DESC)
    WHERE lead_id IS NOT NULL;

CREATE TABLE transaction_confirmations (
    id                 UUID PRIMARY KEY,
    business_id        UUID NOT NULL,
    transaction_id     UUID NOT NULL,
    status             TEXT NOT NULL,
    confirmed_by       TEXT,
    confirmed_at       TIMESTAMPTZ,
    evidence_reference TEXT,
    policy_version     TEXT,
    created_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL,

    CONSTRAINT transaction_confirmations_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT transaction_confirmations_transaction_fk
        FOREIGN KEY (business_id, transaction_id)
        REFERENCES commercial_transactions (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT transaction_confirmations_transaction_uq
        UNIQUE (business_id, transaction_id),
    CONSTRAINT transaction_confirmations_status_chk
        CHECK (status IN ('not_required', 'pending', 'confirmed', 'rejected')),
    CONSTRAINT transaction_confirmations_confirmed_by_chk
        CHECK (confirmed_by IS NULL OR confirmed_by IN ('customer', 'human_agent', 'system_policy')),
    CONSTRAINT transaction_confirmations_lifecycle_chk
        CHECK (
            (status IN ('not_required', 'pending') AND confirmed_by IS NULL AND confirmed_at IS NULL)
            OR (status IN ('confirmed', 'rejected') AND confirmed_by IS NOT NULL AND confirmed_at IS NOT NULL)
        )
);

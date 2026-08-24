CREATE TABLE transaction_reviews (
    id                 UUID PRIMARY KEY,
    business_id        UUID NOT NULL,
    transaction_id     UUID NOT NULL,
    required           BOOLEAN NOT NULL,
    status             TEXT NOT NULL,
    reason_codes       JSONB NOT NULL DEFAULT '[]'::jsonb,
    reviewer_reference TEXT,
    decided_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL,

    CONSTRAINT transaction_reviews_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT transaction_reviews_transaction_fk
        FOREIGN KEY (business_id, transaction_id)
        REFERENCES commercial_transactions (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT transaction_reviews_transaction_uq
        UNIQUE (business_id, transaction_id),
    CONSTRAINT transaction_reviews_status_chk
        CHECK (status IN ('pending', 'approved', 'rejected', 'bypassed')),
    CONSTRAINT transaction_reviews_reason_codes_array_chk
        CHECK (jsonb_typeof(reason_codes) = 'array'),
    CONSTRAINT transaction_reviews_reviewer_chk
        CHECK (reviewer_reference IS NULL OR length(btrim(reviewer_reference)) > 0),
    CONSTRAINT transaction_reviews_lifecycle_chk
        CHECK (
            (required = false AND status = 'bypassed' AND reviewer_reference IS NULL AND decided_at IS NULL)
            OR (required = true AND status = 'pending' AND reviewer_reference IS NULL AND decided_at IS NULL)
            OR (required = true AND status IN ('approved', 'rejected') AND reviewer_reference IS NOT NULL AND decided_at IS NOT NULL)
        )
);

CREATE INDEX idx_transaction_reviews_business_status
    ON transaction_reviews (business_id, status, updated_at DESC);

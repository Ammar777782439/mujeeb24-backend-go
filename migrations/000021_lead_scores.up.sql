CREATE TABLE lead_scores (
    id              UUID PRIMARY KEY,
    business_id     UUID NOT NULL,
    lead_id         UUID NOT NULL,
    value           NUMERIC(5, 2) NOT NULL,
    band            TEXT NOT NULL,
    factors         JSONB NOT NULL,
    rule_version    TEXT,
    model_reference TEXT,
    calculated_at   TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,

    CONSTRAINT lead_scores_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT lead_scores_lead_fk
        FOREIGN KEY (business_id, lead_id)
        REFERENCES leads (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT lead_scores_value_chk
        CHECK (value >= 0 AND value <= 100),
    CONSTRAINT lead_scores_band_chk
        CHECK (band IN ('unknown', 'low', 'medium', 'high')),
    CONSTRAINT lead_scores_factors_object_chk
        CHECK (jsonb_typeof(factors) = 'object')
);

CREATE INDEX idx_lead_scores_business_history
    ON lead_scores (business_id, lead_id, calculated_at DESC, id DESC);

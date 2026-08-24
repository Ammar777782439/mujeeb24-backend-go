CREATE TABLE leads (
    id                    UUID PRIMARY KEY,
    business_id           UUID NOT NULL,
    customer_id           UUID NOT NULL,
    status                TEXT NOT NULL,
    current_score_value   NUMERIC(5, 2),
    current_score_band    TEXT,
    score_rule_version    TEXT,
    score_model_reference TEXT,
    qualification_context JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by            TEXT NOT NULL,
    qualified_by          TEXT,
    lost_reason           TEXT,
    created_at            TIMESTAMPTZ NOT NULL,
    updated_at            TIMESTAMPTZ NOT NULL,

    CONSTRAINT leads_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT leads_customer_fk
        FOREIGN KEY (business_id, customer_id)
        REFERENCES customers (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT leads_business_id_uq
        UNIQUE (business_id, id),
    CONSTRAINT leads_status_chk
        CHECK (status IN ('new', 'interested', 'qualified', 'won', 'lost')),
    CONSTRAINT leads_score_value_chk
        CHECK (current_score_value IS NULL OR (current_score_value >= 0 AND current_score_value <= 100)),
    CONSTRAINT leads_score_band_chk
        CHECK (current_score_band IS NULL OR current_score_band IN ('unknown', 'low', 'medium', 'high')), 
    CONSTRAINT leads_context_object_chk
        CHECK (jsonb_typeof(qualification_context) = 'object'),
    CONSTRAINT leads_created_by_chk
        CHECK (created_by IN ('ai', 'human', 'automation', 'system')),
    CONSTRAINT leads_qualified_by_chk
        CHECK (qualified_by IS NULL OR qualified_by IN ('ai', 'human', 'automation', 'system', 'none')),
    CONSTRAINT leads_lost_reason_chk
        CHECK (status <> 'lost' OR (lost_reason IS NOT NULL AND length(btrim(lost_reason)) > 0))
);

CREATE INDEX idx_leads_business_status
    ON leads (business_id, status, updated_at DESC);

CREATE INDEX idx_leads_business_customer
    ON leads (business_id, customer_id, updated_at DESC);

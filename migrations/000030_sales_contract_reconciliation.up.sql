ALTER TABLE leads
    ADD COLUMN source_conversation_reference_id UUID,
    ADD COLUMN source_channel TEXT,
    ADD COLUMN intent_reference TEXT,
    ADD COLUMN assigned_ownership_reference TEXT,
    ADD COLUMN next_action_at TIMESTAMPTZ,
    ADD COLUMN qualification_reason TEXT,
    ADD COLUMN qualification_evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN qualification_state TEXT NOT NULL DEFAULT 'new',
    ADD COLUMN resource_version BIGINT NOT NULL DEFAULT 1,
    ADD CONSTRAINT leads_source_conversation_reference_fk
        FOREIGN KEY (business_id, source_conversation_reference_id)
        REFERENCES conversation_references (business_id, id)
        ON DELETE RESTRICT,
    ADD CONSTRAINT leads_source_channel_chk
        CHECK (source_channel IS NULL OR source_channel IN ('facebook', 'instagram', 'whatsapp')),
    ADD CONSTRAINT leads_intent_reference_chk
        CHECK (intent_reference IS NULL OR length(btrim(intent_reference)) > 0),
    ADD CONSTRAINT leads_assignment_reference_chk
        CHECK (assigned_ownership_reference IS NULL OR length(btrim(assigned_ownership_reference)) > 0),
    ADD CONSTRAINT leads_qualification_reason_chk
        CHECK (qualification_reason IS NULL OR length(btrim(qualification_reason)) > 0),
    ADD CONSTRAINT leads_qualification_evidence_array_chk
        CHECK (jsonb_typeof(qualification_evidence) = 'array'),
    ADD CONSTRAINT leads_qualification_state_chk
        CHECK (qualification_state IN ('new', 'qualified', 'working', 'converted', 'lost', 'disqualified')),
    ADD CONSTRAINT leads_resource_version_chk
        CHECK (resource_version > 0);

CREATE INDEX idx_leads_business_qualification_state
    ON leads (business_id, qualification_state, updated_at DESC, id DESC);

CREATE INDEX idx_leads_business_source_reference
    ON leads (business_id, source_conversation_reference_id, created_at DESC, id DESC)
    WHERE source_conversation_reference_id IS NOT NULL;

ALTER TABLE commercial_transactions
    ADD COLUMN requires_human_review BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN cancellation_reason TEXT,
    ADD COLUMN resource_version BIGINT NOT NULL DEFAULT 1,
    ADD CONSTRAINT commercial_transactions_cancellation_reason_chk
        CHECK (cancellation_reason IS NULL OR length(btrim(cancellation_reason)) > 0),
    ADD CONSTRAINT commercial_transactions_resource_version_chk
        CHECK (resource_version > 0);

CREATE INDEX idx_commercial_transactions_business_type
    ON commercial_transactions (business_id, transaction_type, updated_at DESC, id DESC);

ALTER TABLE transaction_reviews
    ADD COLUMN decision_reason TEXT,
    ADD CONSTRAINT transaction_reviews_decision_reason_chk
        CHECK (decision_reason IS NULL OR length(btrim(decision_reason)) > 0);

ALTER TABLE order_lines
    ADD COLUMN pricing_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN availability_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN fulfillment_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD CONSTRAINT order_lines_pricing_snapshot_object_chk
        CHECK (jsonb_typeof(pricing_snapshot) = 'object'),
    ADD CONSTRAINT order_lines_availability_snapshot_object_chk
        CHECK (jsonb_typeof(availability_snapshot) = 'object'),
    ADD CONSTRAINT order_lines_fulfillment_snapshot_object_chk
        CHECK (jsonb_typeof(fulfillment_snapshot) = 'object');

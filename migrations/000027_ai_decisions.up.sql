CREATE TABLE ai_decisions (
    id                       UUID PRIMARY KEY,
    business_id              UUID NOT NULL,
    conversation_id          UUID,
    source_message_reference TEXT,
    intent_base              TEXT NOT NULL,
    domain_context           TEXT,
    entities                 JSONB NOT NULL DEFAULT '{}'::jsonb,
    evidence_references      JSONB NOT NULL DEFAULT '[]'::jsonb,
    requested_action         TEXT NOT NULL,
    confidence_value         NUMERIC(5, 4),
    confidence_band          TEXT NOT NULL,
    requires_human           BOOLEAN NOT NULL,
    missing_information      JSONB NOT NULL DEFAULT '[]'::jsonb,
    reason_codes             JSONB NOT NULL DEFAULT '[]'::jsonb,
    policy_reference         TEXT,
    policy_version           TEXT NOT NULL,
    knowledge_version        TEXT,
    model_reference          TEXT,
    schema_version           INTEGER NOT NULL,
    lifecycle                TEXT NOT NULL,
    policy_decision          TEXT,
    outcome                  TEXT,
    execution_reference      TEXT,
    correlation_id           UUID,
    causation_id             UUID,
    expires_at               TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL,
    updated_at               TIMESTAMPTZ NOT NULL,

    CONSTRAINT ai_decisions_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id)
        ON DELETE RESTRICT,
    CONSTRAINT ai_decisions_conversation_fk
        FOREIGN KEY (business_id, conversation_id)
        REFERENCES conversations (business_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT ai_decisions_conversation_ref_chk
        CHECK (source_message_reference IS NULL OR length(btrim(source_message_reference)) > 0),
    CONSTRAINT ai_decisions_intent_chk
        CHECK (length(btrim(intent_base)) > 0),
    CONSTRAINT ai_decisions_entities_object_chk
        CHECK (jsonb_typeof(entities) = 'object'),
    CONSTRAINT ai_decisions_evidence_array_chk
        CHECK (jsonb_typeof(evidence_references) = 'array'),
    CONSTRAINT ai_decisions_missing_array_chk
        CHECK (jsonb_typeof(missing_information) = 'array'),
    CONSTRAINT ai_decisions_reason_codes_array_chk
        CHECK (jsonb_typeof(reason_codes) = 'array'),
    CONSTRAINT ai_decisions_action_chk
        CHECK (requested_action IN (
            'answer', 'ask_clarification', 'create_lead', 'update_lead',
            'create_transaction_draft', 'request_availability_check',
            'request_price_check', 'request_human', 'no_action'
        )),
    CONSTRAINT ai_decisions_confidence_value_chk
        CHECK (confidence_value IS NULL OR (confidence_value >= 0 AND confidence_value <= 1)),
    CONSTRAINT ai_decisions_confidence_band_chk
        CHECK (confidence_band IN ('unknown', 'low', 'medium', 'high')),
    CONSTRAINT ai_decisions_policy_decision_chk
        CHECK (policy_decision IS NULL OR policy_decision IN ('allowed', 'requires_approval', 'denied')),
    CONSTRAINT ai_decisions_lifecycle_chk
        CHECK (lifecycle IN ('proposed', 'validated', 'policy_evaluated', 'expired', 'rejected')),
    CONSTRAINT ai_decisions_schema_version_chk
        CHECK (schema_version > 0),
    CONSTRAINT ai_decisions_expiry_chk
        CHECK (expires_at IS NULL OR expires_at >= created_at)
);

CREATE INDEX idx_ai_decisions_business_created
    ON ai_decisions (business_id, created_at DESC, id DESC);

CREATE INDEX idx_ai_decisions_business_conversation
    ON ai_decisions (business_id, conversation_id, created_at DESC)
    WHERE conversation_id IS NOT NULL;

CREATE INDEX idx_ai_decisions_business_lifecycle
    ON ai_decisions (business_id, lifecycle, updated_at DESC);

CREATE INDEX idx_ai_decisions_correlation
    ON ai_decisions (business_id, correlation_id)
    WHERE correlation_id IS NOT NULL;

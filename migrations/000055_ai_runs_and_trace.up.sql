-- ADR-034 — AI Run + Attempt + Tool Call + Gemini Interaction + Catalog Batch + Usage Telemetry
-- Implements contracts ⑧ Observability + Audit + AI Trace and ⑨ AI Runtime Lifecycle.
-- Operational tables, separated from the business ai_decisions row.

CREATE TABLE ai_runs (
    id                  UUID PRIMARY KEY,
    business_id         UUID NOT NULL,
    conversation_id     UUID,
    message_id          TEXT,
    source_event_id     UUID,
    idempotency_key     TEXT NOT NULL,
    agent_role          TEXT NOT NULL,
    status              TEXT NOT NULL,
    failure_stage      TEXT,
    failure_category   TEXT,
    failure_reason     TEXT,
    gemini_interaction_id      TEXT,
    previous_interaction_id    TEXT,
    started_at         TIMESTAMPTZ NOT NULL,
    context_built_at   TIMESTAMPTZ,
    running_started_at TIMESTAMPTZ,
    waiting_tool_at    TIMESTAMPTZ,
    validating_at      TIMESTAMPTZ,
    authorized_at      TIMESTAMPTZ,
    executing_at       TIMESTAMPTZ,
    completed_at       TIMESTAMPTZ,
    failed_at          TIMESTAMPTZ,
    cancelled_at      TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL,

    CONSTRAINT ai_runs_business_fk
        FOREIGN KEY (business_id) REFERENCES businesses(id) ON DELETE RESTRICT,
    CONSTRAINT ai_runs_status_chk
        CHECK (status IN (
            'received', 'context_built', 'running', 'waiting_tool',
            'validating', 'authorized', 'executing',
            'completed', 'failed', 'cancelled'
        )),
    CONSTRAINT ai_runs_agent_role_chk
        CHECK (agent_role IN ('customer_sales', 'merchant_catalog_authoring')),
    CONSTRAINT ai_runs_failure_stage_chk
        CHECK (failure_stage IS NULL OR failure_stage IN (
            'context_build', 'gemini_request', 'tool_call',
            'validation', 'policy', 'authorization', 'execution'
        )),
    CONSTRAINT ai_runs_failure_category_chk
        CHECK (failure_category IS NULL OR failure_category IN (
            'provider_temporary', 'provider_permanent', 'network',
            'timeout', 'rate_limit', 'invalid_ai_output',
            'invalid_tool_arguments', 'tenant_violation',
            'invalid_reference', 'policy_denial', 'authorization_denial',
            'unsupported_action', 'execution_failure', 'infrastructure'
        ))
);

CREATE UNIQUE INDEX uq_ai_runs_idempotency
    ON ai_runs (business_id, idempotency_key);

CREATE INDEX idx_ai_runs_business_status_updated
    ON ai_runs (business_id, status, updated_at DESC);

CREATE INDEX idx_ai_runs_business_conversation
    ON ai_runs (business_id, conversation_id, started_at DESC)
    WHERE conversation_id IS NOT NULL;

CREATE INDEX idx_ai_runs_business_message
    ON ai_runs (business_id, message_id)
    WHERE message_id IS NOT NULL;

CREATE INDEX idx_ai_runs_business_agent_status
    ON ai_runs (business_id, agent_role, status, updated_at DESC);

CREATE INDEX idx_ai_runs_gemini_interaction
    ON ai_runs (gemini_interaction_id)
    WHERE gemini_interaction_id IS NOT NULL;

-- 9.27 — AI Run Attempts (one logical Run may have multiple operational Attempts)
CREATE TABLE ai_run_attempts (
    id              UUID PRIMARY KEY,
    ai_run_id       UUID NOT NULL,
    attempt_number  INTEGER NOT NULL,
    provider        TEXT NOT NULL,
    model           TEXT,
    status          TEXT NOT NULL,
    failure_stage      TEXT,
    failure_category   TEXT,
    failure_reason    TEXT,
    request_payload_hash TEXT,
    started_at      TIMESTAMPTZ NOT NULL,
    finished_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL,

    CONSTRAINT ai_run_attempts_run_fk
        FOREIGN KEY (ai_run_id) REFERENCES ai_runs(id) ON DELETE CASCADE,
    CONSTRAINT ai_run_attempts_status_chk
        CHECK (status IN ('running', 'succeeded', 'failed', 'cancelled', 'timed_out')),
    CONSTRAINT ai_run_attempts_failure_stage_chk
        CHECK (failure_stage IS NULL OR failure_stage IN (
            'context_build', 'gemini_request', 'tool_call',
            'validation', 'policy', 'authorization', 'execution'
        )),
    CONSTRAINT ai_run_attempts_failure_category_chk
        CHECK (failure_category IS NULL OR failure_category IN (
            'provider_temporary', 'provider_permanent', 'network',
            'timeout', 'rate_limit', 'invalid_ai_output',
            'invalid_tool_arguments', 'tenant_violation',
            'invalid_reference', 'policy_denial', 'authorization_denial',
            'unsupported_action', 'execution_failure', 'infrastructure'
        ))
);

CREATE UNIQUE INDEX uq_ai_run_attempts_run_number
    ON ai_run_attempts (ai_run_id, attempt_number);

CREATE INDEX idx_ai_run_attempts_run_started
    ON ai_run_attempts (ai_run_id, started_at ASC);

-- 9.27 — Tool Calls within an Attempt (Function Calling trace per contract ⑧ §7)
CREATE TABLE ai_tool_calls (
    id              UUID PRIMARY KEY,
    ai_run_id      UUID NOT NULL,
    attempt_id     UUID NOT NULL,
    tool_name       TEXT NOT NULL,
    tool_call_id   TEXT,
    status          TEXT NOT NULL,
    request_params  JSONB NOT NULL DEFAULT '{}'::jsonb,
    result_payload  JSONB,
    failure_reason  TEXT,
    started_at      TIMESTAMPTZ NOT NULL,
    finished_at     TIMESTAMPTZ,
    latency_ms      INTEGER,
    created_at      TIMESTAMPTZ NOT NULL,

    CONSTRAINT ai_tool_calls_run_fk
        FOREIGN KEY (ai_run_id) REFERENCES ai_runs(id) ON DELETE CASCADE,
    CONSTRAINT ai_tool_calls_attempt_fk
        FOREIGN KEY (attempt_id) REFERENCES ai_run_attempts(id) ON DELETE CASCADE,
    CONSTRAINT ai_tool_calls_status_chk
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'timed_out', 'cancelled'))
);

CREATE INDEX idx_ai_tool_calls_run_started
    ON ai_tool_calls (ai_run_id, started_at ASC);

CREATE INDEX idx_ai_tool_calls_attempt_started
    ON ai_tool_calls (attempt_id, started_at ASC);

CREATE INDEX idx_ai_tool_calls_status
    ON ai_tool_calls (status, started_at DESC);

-- 8.6 — Gemini Interaction trace: previous_interaction_id continuity per contract ③
CREATE TABLE ai_gemini_interactions (
    id                       UUID PRIMARY KEY,
    ai_run_id                UUID NOT NULL,
    attempt_id               UUID NOT NULL,
    gemini_interaction_id    TEXT NOT NULL,
    previous_interaction_id  TEXT,
    model                    TEXT NOT NULL,
    system_instruction_hash  TEXT,
    tools_hash               TEXT,
    generation_config_hash   TEXT,
    started_at               TIMESTAMPTZ NOT NULL,
    finished_at              TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL,

    CONSTRAINT ai_gemini_interactions_run_fk
        FOREIGN KEY (ai_run_id) REFERENCES ai_runs(id) ON DELETE CASCADE,
    CONSTRAINT ai_gemini_interactions_attempt_fk
        FOREIGN KEY (attempt_id) REFERENCES ai_run_attempts(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX uq_ai_gemini_interactions_id
    ON ai_gemini_interactions (gemini_interaction_id);

CREATE INDEX idx_ai_gemini_interactions_run_started
    ON ai_gemini_interactions (ai_run_id, started_at ASC);

CREATE INDEX idx_ai_gemini_interactions_previous
    ON ai_gemini_interactions (previous_interaction_id)
    WHERE previous_interaction_id IS NOT NULL;

-- ② Catalog Batch Tracking — Coverage enforcement per contract ② §3
CREATE TABLE ai_catalog_batches (
    id              UUID PRIMARY KEY,
    ai_run_id       UUID NOT NULL,
    batch_number    INTEGER NOT NULL,
    status          TEXT NOT NULL,
    items_count     INTEGER NOT NULL,
    schemas_count   INTEGER NOT NULL,
    input_tokens    INTEGER,
    candidate_count INTEGER,
    failure_reason  TEXT,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,

    CONSTRAINT ai_catalog_batches_run_fk
        FOREIGN KEY (ai_run_id) REFERENCES ai_runs(id) ON DELETE CASCADE,
    CONSTRAINT ai_catalog_batches_status_chk
        CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    CONSTRAINT ai_catalog_batches_items_count_chk
        CHECK (items_count >= 0),
    CONSTRAINT ai_catalog_batches_batch_number_chk
        CHECK (batch_number > 0)
);

CREATE UNIQUE INDEX uq_ai_catalog_batches_run_number
    ON ai_catalog_batches (ai_run_id, batch_number);

CREATE INDEX idx_ai_catalog_batches_run_status
    ON ai_catalog_batches (ai_run_id, status, batch_number ASC);

-- 8.8 — AI Usage / Cost Telemetry per contract ⑧ §8
CREATE TABLE ai_usage_telemetry (
    id                       UUID PRIMARY KEY,
    ai_run_id                UUID NOT NULL,
    attempt_id               UUID,
    provider                 TEXT NOT NULL,
    model                    TEXT NOT NULL,
    input_tokens             INTEGER NOT NULL DEFAULT 0,
    cached_tokens            INTEGER NOT NULL DEFAULT 0,
    output_tokens            INTEGER NOT NULL DEFAULT 0,
    tool_calls_count         INTEGER NOT NULL DEFAULT 0,
    catalog_projection_tokens INTEGER NOT NULL DEFAULT 0,
    estimated_cost_micros   BIGINT NOT NULL DEFAULT 0,
    currency                 TEXT NOT NULL DEFAULT 'USD',
    measured_at             TIMESTAMPTZ NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL,

    CONSTRAINT ai_usage_telemetry_run_fk
        FOREIGN KEY (ai_run_id) REFERENCES ai_runs(id) ON DELETE CASCADE,
    CONSTRAINT ai_usage_telemetry_attempt_fk
        FOREIGN KEY (attempt_id) REFERENCES ai_run_attempts(id) ON DELETE SET NULL,
    CONSTRAINT ai_usage_telemetry_tokens_chk
        CHECK (input_tokens >= 0 AND cached_tokens >= 0 AND output_tokens >= 0),
    CONSTRAINT ai_usage_telemetry_cost_chk
        CHECK (estimated_cost_micros >= 0)
);

CREATE INDEX idx_ai_usage_telemetry_run
    ON ai_usage_telemetry (ai_run_id, measured_at DESC);

CREATE INDEX idx_ai_usage_telemetry_business_provider_model
    ON ai_usage_telemetry (provider, model, measured_at DESC);

-- 8.9 — Per-stage latency for observability per contract ⑧ §9
CREATE TABLE ai_stage_latencies (
    id           UUID PRIMARY KEY,
    ai_run_id    UUID NOT NULL,
    stage        TEXT NOT NULL,
    latency_ms   INTEGER NOT NULL,
    measured_at  TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,

    CONSTRAINT ai_stage_latencies_run_fk
        FOREIGN KEY (ai_run_id) REFERENCES ai_runs(id) ON DELETE CASCADE,
    CONSTRAINT ai_stage_latencies_stage_chk
        CHECK (stage IN (
            'context_build', 'gemini_request', 'tool_call',
            'validation', 'policy', 'authorization', 'execution',
            'batch_evaluation', 'final_evaluation'
        )),
    CONSTRAINT ai_stage_latencies_latency_chk
        CHECK (latency_ms >= 0)
);

CREATE INDEX idx_ai_stage_latencies_run_stage
    ON ai_stage_latencies (ai_run_id, stage, measured_at DESC);

CREATE INDEX idx_ai_stage_latencies_stage_latency
    ON ai_stage_latencies (stage, measured_at DESC);

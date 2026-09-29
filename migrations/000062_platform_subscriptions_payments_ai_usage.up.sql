-- Platform Administration Contract V1 — Subscriptions + Payments + Usage
-- Per docs/architecture/PlatformAdministrationContractV1.md §20-36
-- Per docs/architecture/AIUsageTokenTelemetry.md §6-§36

CREATE TABLE subscriptions (
    id              UUID PRIMARY KEY,
    business_id    UUID NOT NULL REFERENCES businesses(id) ON DELETE RESTRICT,
    plan_id         UUID NOT NULL REFERENCES plans(id) ON DELETE RESTRICT,
    period_start    TIMESTAMPTZ NOT NULL,
    period_end      TIMESTAMPTZ NOT NULL,
    status          TEXT NOT NULL DEFAULT 'PENDING',
    ai_reply_limit       INT NOT NULL,
    ai_catalog_limit     INT NOT NULL,
    channel_limit        INT NOT NULL,
    internal_ai_cost_budget_yer INT NOT NULL,
    cost_budget_override_yer INT,
    cost_budget_override_reason TEXT,
    cost_budget_override_by UUID,
    cost_budget_override_at TIMESTAMPTZ,
    cancelled_at    TIMESTAMPTZ,
    cancelled_reason TEXT,
    cancelled_by    UUID,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,

    CONSTRAINT subscriptions_period_chk CHECK (period_end > period_start),
    CONSTRAINT subscriptions_status_chk CHECK (status IN ('PENDING', 'ACTIVE', 'EXPIRED', 'CANCELLED')),
    CONSTRAINT subscriptions_cancelled_chk CHECK ((status = 'CANCELLED') = (cancelled_at IS NOT NULL)),
    CONSTRAINT subscriptions_ai_reply_limit_positive_chk CHECK (ai_reply_limit > 0),
    CONSTRAINT subscriptions_ai_catalog_limit_nonneg_chk CHECK (ai_catalog_limit >= 0),
    CONSTRAINT subscriptions_channel_limit_positive_chk CHECK (channel_limit > 0),
    CONSTRAINT subscriptions_internal_ai_cost_budget_positive_chk CHECK (internal_ai_cost_budget_yer > 0),
    CONSTRAINT subscriptions_cost_budget_override_chk CHECK (
        (cost_budget_override_yer IS NULL) = (cost_budget_override_reason IS NULL)
    )
);

CREATE INDEX idx_subscriptions_business ON subscriptions (business_id);
CREATE INDEX idx_subscriptions_status ON subscriptions (status);
CREATE INDEX idx_subscriptions_period ON subscriptions (period_end);

CREATE TABLE subscription_payments (
    id              UUID PRIMARY KEY,
    subscription_id UUID NOT NULL REFERENCES subscriptions(id) ON DELETE RESTRICT,
    business_id     UUID NOT NULL REFERENCES businesses(id) ON DELETE RESTRICT,
    amount_yer      INT NOT NULL,
    method          TEXT NOT NULL,
    reference       TEXT NOT NULL,
    paid_at         TIMESTAMPTZ NOT NULL,
    recorded_by     UUID NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL,

    CONSTRAINT subscription_payments_amount_positive_chk CHECK (amount_yer > 0),
    CONSTRAINT subscription_payments_method_chk CHECK (method IN ('CASH', 'BANK_TRANSFER', 'MOBILE_MONEY', 'OTHER')),
    CONSTRAINT subscription_payments_reference_not_blank_chk CHECK (length(btrim(reference)) > 0),
    CONSTRAINT subscription_payments_subscription_uq UNIQUE (subscription_id, paid_at, reference)
);

CREATE INDEX idx_subscription_payments_subscription ON subscription_payments (subscription_id);
CREATE INDEX idx_subscription_payments_business ON subscription_payments (business_id);

-- Subscription AI Usage aggregate (snapshot refreshed by worker; contract §8)
CREATE TABLE subscription_ai_usage (
    subscription_id    UUID PRIMARY KEY REFERENCES subscriptions(id) ON DELETE CASCADE,
    business_id        UUID NOT NULL REFERENCES businesses(id) ON DELETE RESTRICT,
    ai_reply_limit     INT NOT NULL,
    ai_replies_used    INT NOT NULL DEFAULT 0,
    input_tokens       BIGINT NOT NULL DEFAULT 0,
    cached_input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens      BIGINT NOT NULL DEFAULT 0,
    model_requests     INT NOT NULL DEFAULT 0,
    tool_calls         INT NOT NULL DEFAULT 0,
    provider_cost_yer  INT NOT NULL DEFAULT 0,
    last_recorded_at   TIMESTAMPTZ,
    updated_at         TIMESTAMPTZ NOT NULL,

    CONSTRAINT subscription_ai_usage_replies_nonneg_chk CHECK (ai_replies_used >= 0),
    CONSTRAINT subscription_ai_usage_tokens_nonneg_chk CHECK (input_tokens >= 0 AND cached_input_tokens >= 0 AND output_tokens >= 0),
    CONSTRAINT subscription_ai_usage_counts_nonneg_chk CHECK (model_requests >= 0 AND tool_calls >= 0),
    CONSTRAINT subscription_ai_usage_cost_nonneg_chk CHECK (provider_cost_yer >= 0)
);

CREATE INDEX idx_subscription_ai_usage_business ON subscription_ai_usage (business_id);

-- AI Usage Records — per-execution telemetry (contract §6)
CREATE TABLE ai_usage_records (
    id                  UUID PRIMARY KEY,
    business_id        UUID NOT NULL REFERENCES businesses(id) ON DELETE RESTRICT,
    subscription_id    UUID NOT NULL REFERENCES subscriptions(id) ON DELETE RESTRICT,
    provider           TEXT NOT NULL,
    model              TEXT NOT NULL,
    input_tokens       BIGINT NOT NULL,
    cached_input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens      BIGINT NOT NULL,
    model_requests     INT NOT NULL DEFAULT 1,
    tool_calls         INT NOT NULL DEFAULT 0,
    final_ai_replies   INT NOT NULL DEFAULT 0,
    provider_cost_yer  INT NOT NULL DEFAULT 0,
    pricing_version    TEXT NOT NULL,
    status             TEXT NOT NULL,
    failure_code       TEXT,
    correlation_id     TEXT,
    started_at         TIMESTAMPTZ NOT NULL,
    completed_at       TIMESTAMPTZ NOT NULL,

    CONSTRAINT ai_usage_records_tokens_nonneg_chk CHECK (input_tokens >= 0 AND cached_input_tokens >= 0 AND output_tokens >= 0),
    CONSTRAINT ai_usage_records_counts_nonneg_chk CHECK (model_requests >= 0 AND tool_calls >= 0 AND final_ai_replies >= 0),
    CONSTRAINT ai_usage_records_cost_nonneg_chk CHECK (provider_cost_yer >= 0),
    CONSTRAINT ai_usage_records_completed_after_started_chk CHECK (completed_at >= started_at)
);

CREATE INDEX idx_ai_usage_records_business ON ai_usage_records (business_id);
CREATE INDEX idx_ai_usage_records_subscription ON ai_usage_records (subscription_id);
CREATE INDEX idx_ai_usage_records_started_at ON ai_usage_records (started_at);

-- AI Provider Pricing Versions — immutable historical pricing (contract §30)
CREATE TABLE ai_provider_pricing_versions (
    id                  UUID PRIMARY KEY,
    provider           TEXT NOT NULL,
    model              TEXT NOT NULL,
    pricing_version    TEXT NOT NULL,
    input_per_million_yer   INT NOT NULL,
    cached_input_per_million_yer INT NOT NULL DEFAULT 0,
    output_per_million_yer  INT NOT NULL,
    effective_from     TIMESTAMPTZ NOT NULL,
    effective_to       TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL,

    CONSTRAINT ai_provider_pricing_prices_positive_chk CHECK (input_per_million_yer > 0 AND output_per_million_yer > 0),
    CONSTRAINT ai_provider_pricing_cached_chk CHECK (cached_input_per_million_yer >= 0),
    CONSTRAINT ai_provider_pricing_version_not_blank_chk CHECK (length(btrim(pricing_version)) > 0),
    CONSTRAINT ai_provider_pricing_effective_chk CHECK (effective_to IS NULL OR effective_to > effective_from),
    CONSTRAINT ai_provider_pricing_provider_model_version_uq UNIQUE (provider, model, pricing_version)
);

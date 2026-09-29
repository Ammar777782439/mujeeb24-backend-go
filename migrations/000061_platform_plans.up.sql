-- Platform Administration Contract V1 — Plan definitions
-- Per docs/architecture/PlatformAdministrationContractV1.md §15-19

CREATE TABLE plans (
    id              UUID PRIMARY KEY,
    code            TEXT NOT NULL,
    version         INT NOT NULL DEFAULT 1,
    display_name    TEXT NOT NULL,
    price_yer       INT NOT NULL,
    billing_interval TEXT NOT NULL DEFAULT 'MONTH',
    ai_reply_limit       INT NOT NULL,
    ai_catalog_limit    INT NOT NULL,
    channel_limit       INT NOT NULL,
    internal_ai_cost_budget_yer INT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'DRAFT',
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,
    retired_at      TIMESTAMPTZ,

    CONSTRAINT plans_code_not_blank_chk CHECK (length(btrim(code)) > 0),
    CONSTRAINT plans_display_name_not_blank_chk CHECK (length(btrim(display_name)) > 0),
    CONSTRAINT plans_price_positive_chk CHECK (price_yer > 0),
    CONSTRAINT plans_billing_interval_chk CHECK (billing_interval = 'MONTH'),
    CONSTRAINT plans_ai_reply_limit_positive_chk CHECK (ai_reply_limit > 0),
    CONSTRAINT plans_ai_catalog_limit_nonneg_chk CHECK (ai_catalog_limit >= 0),
    CONSTRAINT plans_channel_limit_positive_chk CHECK (channel_limit > 0),
    CONSTRAINT plans_internal_ai_cost_budget_positive_chk CHECK (internal_ai_cost_budget_yer > 0),
    CONSTRAINT plans_status_chk CHECK (status IN ('DRAFT', 'ACTIVE', 'RETIRED')),
    CONSTRAINT plans_retired_chk CHECK ((status = 'RETIRED') = (retired_at IS NOT NULL)),
    CONSTRAINT plans_code_version_uq UNIQUE (code, version)
);

CREATE INDEX idx_plans_status ON plans (status);
CREATE INDEX idx_plans_code ON plans (code);

-- Seed the 3 baseline plans per Contract §16
INSERT INTO plans (id, code, version, display_name, price_yer, billing_interval, ai_reply_limit, ai_catalog_limit, channel_limit, internal_ai_cost_budget_yer, status, created_at, updated_at)
VALUES
    ('00000000-0000-0000-0000-00000000a001', 'basic',    1, 'Basic',    5000,  'MONTH', 500,  200,  1, 1000, 'ACTIVE', now(), now()),
    ('00000000-0000-0000-0000-00000000a002', 'growth',   1, 'Growth',   10000, 'MONTH', 1500, 750,  2, 3500, 'ACTIVE', now(), now()),
    ('00000000-0000-0000-0000-00000000a003', 'business', 1, 'Business', 20000, 'MONTH', 4000, 2500, 5, 9000, 'ACTIVE', now(), now())
ON CONFLICT (code, version) DO NOTHING;

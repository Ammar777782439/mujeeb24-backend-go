-- Pricing V2 and subscription integrity.
-- Forward-only: historical plan versions and subscriptions remain immutable.

ALTER TABLE plans
    ADD COLUMN included_channel_limit INT NOT NULL DEFAULT 1,
    ADD COLUMN channel_ceiling INT NOT NULL DEFAULT 1,
    ADD COLUMN seat_limit INT NOT NULL DEFAULT 1,
    ADD CONSTRAINT plans_included_channel_limit_positive_chk CHECK (included_channel_limit > 0),
    ADD CONSTRAINT plans_channel_ceiling_chk CHECK (channel_ceiling >= included_channel_limit),
    ADD CONSTRAINT plans_seat_limit_positive_chk CHECK (seat_limit > 0);

-- The legacy channel_limit represented the only channel limit. Preserve it as
-- both the included allowance and ceiling for historical plan versions.
UPDATE plans
SET included_channel_limit = channel_limit,
    channel_ceiling = channel_limit,
    seat_limit = 1;

ALTER TABLE subscriptions
    ADD COLUMN price_yer_snapshot INT,
    ADD COLUMN amount_due_yer INT,
    ADD COLUMN included_channel_limit INT NOT NULL DEFAULT 1,
    ADD COLUMN channel_ceiling INT NOT NULL DEFAULT 1,
    ADD COLUMN seat_limit INT NOT NULL DEFAULT 1,
    ADD COLUMN activated_at TIMESTAMPTZ;

UPDATE subscriptions s
SET price_yer_snapshot = p.price_yer,
    amount_due_yer = p.price_yer,
    included_channel_limit = s.channel_limit,
    channel_ceiling = s.channel_limit,
    seat_limit = 1,
    activated_at = CASE WHEN s.status IN ('ACTIVE', 'EXPIRED') THEN s.updated_at ELSE NULL END
FROM plans p
WHERE p.id = s.plan_id;

ALTER TABLE subscriptions
    ALTER COLUMN price_yer_snapshot SET NOT NULL,
    ALTER COLUMN amount_due_yer SET NOT NULL,
    ADD CONSTRAINT subscriptions_price_snapshot_positive_chk CHECK (price_yer_snapshot > 0),
    ADD CONSTRAINT subscriptions_amount_due_positive_chk CHECK (amount_due_yer > 0),
    ADD CONSTRAINT subscriptions_included_channel_limit_positive_chk CHECK (included_channel_limit > 0),
    ADD CONSTRAINT subscriptions_channel_ceiling_chk CHECK (channel_ceiling >= included_channel_limit),
    ADD CONSTRAINT subscriptions_seat_limit_positive_chk CHECK (seat_limit > 0),
    ADD CONSTRAINT subscriptions_business_id_id_uq UNIQUE (business_id, id),
    ADD CONSTRAINT subscriptions_activation_state_chk CHECK (
        (status = 'PENDING' AND activated_at IS NULL)
        OR status = 'CANCELLED'
        OR (status IN ('ACTIVE', 'EXPIRED') AND activated_at IS NOT NULL)
    );

ALTER TABLE subscription_payments
    ADD COLUMN idempotency_key TEXT,
    ADD CONSTRAINT subscription_payments_idempotency_not_blank_chk
        CHECK (idempotency_key IS NULL OR length(btrim(idempotency_key)) > 0),
    ADD CONSTRAINT subscription_payments_business_subscription_fk
        FOREIGN KEY (business_id, subscription_id)
        REFERENCES subscriptions (business_id, id)
        ON DELETE RESTRICT;

CREATE UNIQUE INDEX subscription_payments_actor_idempotency_uq
    ON subscription_payments (recorded_by, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE UNIQUE INDEX subscription_payments_one_payment_per_subscription_uq
    ON subscription_payments (subscription_id);

ALTER TABLE subscription_ai_usage
    ADD CONSTRAINT subscription_ai_usage_business_subscription_fk
        FOREIGN KEY (business_id, subscription_id)
        REFERENCES subscriptions (business_id, id)
        ON DELETE CASCADE;

ALTER TABLE ai_usage_records
    ADD CONSTRAINT ai_usage_records_business_subscription_fk
        FOREIGN KEY (business_id, subscription_id)
        REFERENCES subscriptions (business_id, id)
        ON DELETE RESTRICT;

-- Fail migration instead of silently deciding which financially-active row wins.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM subscriptions
        WHERE status = 'ACTIVE'
        GROUP BY business_id
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'pricing v2 migration blocked: a business has multiple ACTIVE subscriptions; reconcile explicitly';
    END IF;
    IF EXISTS (
        SELECT 1 FROM subscriptions
        WHERE status = 'PENDING'
        GROUP BY business_id
        HAVING count(*) > 1
    ) THEN
        RAISE EXCEPTION 'pricing v2 migration blocked: a business has multiple PENDING subscriptions; reconcile explicitly';
    END IF;
END $$;

CREATE UNIQUE INDEX subscriptions_one_active_per_business_uq
    ON subscriptions (business_id)
    WHERE status = 'ACTIVE';

CREATE UNIQUE INDEX subscriptions_one_pending_per_business_uq
    ON subscriptions (business_id)
    WHERE status = 'PENDING';

-- Retire only the three legacy public versions. Existing subscriptions keep
-- their snapshots and references. The new public versions implement Pricing V2.
UPDATE plans
SET status = 'RETIRED', retired_at = now(), updated_at = now()
WHERE id IN (
    '00000000-0000-0000-0000-00000000a001',
    '00000000-0000-0000-0000-00000000a002',
    '00000000-0000-0000-0000-00000000a003'
) AND status = 'ACTIVE';

INSERT INTO plans (
    id, code, version, display_name, price_yer, billing_interval,
    ai_reply_limit, ai_catalog_limit, channel_limit,
    included_channel_limit, channel_ceiling, seat_limit,
    internal_ai_cost_budget_yer, status, created_at, updated_at
) VALUES
    ('00000000-0000-0000-0000-00000000a101', 'starter',  1, 'Starter',  4000,  'MONTH', 50,   20,   1, 1, 1,  1,  500, 'ACTIVE', now(), now()),
    ('00000000-0000-0000-0000-00000000a102', 'basic',    2, 'Basic',    6500,  'MONTH', 200,  60,   1, 1, 1,  2, 1000, 'ACTIVE', now(), now()),
    ('00000000-0000-0000-0000-00000000a103', 'growth',   2, 'Growth',  10000,  'MONTH', 500,  200,  1, 1, 2,  3, 2000, 'ACTIVE', now(), now()),
    ('00000000-0000-0000-0000-00000000a104', 'pro',      1, 'Pro',     16000,  'MONTH', 1200, 600,  1, 1, 3,  5, 4000, 'ACTIVE', now(), now()),
    ('00000000-0000-0000-0000-00000000a105', 'business', 2, 'Business',25000,  'MONTH', 3000, 1500, 1, 1, 5, 10, 7500, 'ACTIVE', now(), now());

CREATE UNIQUE INDEX plans_one_active_version_per_code_uq
    ON plans (code)
    WHERE status = 'ACTIVE';

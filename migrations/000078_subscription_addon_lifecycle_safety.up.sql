-- Durable command identity and a complete lifecycle for period-bound add-ons.
-- This migration is deliberately forward-only: 000077 may already be applied.
ALTER TABLE subscription_addons
    ADD COLUMN creation_idempotency_key TEXT,
    ADD COLUMN cancelled_by UUID;

-- Backfill any cancellation recorded before this lifecycle metadata existed.
UPDATE subscription_addons
SET cancelled_at = COALESCE(cancelled_at, updated_at, created_at),
    cancelled_reason = COALESCE(NULLIF(btrim(cancelled_reason), ''), 'legacy cancellation')
WHERE status = 'CANCELLED';

ALTER TABLE subscription_addons
    ADD CONSTRAINT subscription_addons_cancelled_state_chk CHECK (
        (status <> 'CANCELLED' AND cancelled_at IS NULL)
        OR (status = 'CANCELLED' AND cancelled_at IS NOT NULL)
    );

CREATE UNIQUE INDEX subscription_addons_creation_idempotency_idx
    ON subscription_addons (subscription_id, created_by, creation_idempotency_key)
    WHERE creation_idempotency_key IS NOT NULL;

-- A Channel Pass is saleable only after a platform administrator has verified
-- and reserved the external provider capacity for this exact subscription.
CREATE TABLE subscription_channel_pass_capacity_approvals (
    subscription_id UUID PRIMARY KEY REFERENCES subscriptions(id) ON DELETE RESTRICT,
    business_id UUID NOT NULL,
    max_channel_passes INT NOT NULL CHECK (max_channel_passes > 0),
    reason TEXT NOT NULL CHECK (length(btrim(reason)) > 0),
    approved_by UUID NOT NULL,
    approved_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT subscription_channel_pass_capacity_approvals_business_subscription_fk
        FOREIGN KEY (business_id, subscription_id)
        REFERENCES subscriptions (business_id, id) ON DELETE RESTRICT
);

-- Do not leave paid capacity live after the period is already terminal.
UPDATE subscription_addons a
SET status = 'CANCELLED',
    cancelled_at = now(),
    cancelled_by = a.created_by,
    cancelled_reason = 'subscription is no longer active for its period',
    updated_at = now()
FROM subscriptions s
WHERE a.subscription_id = s.id
  AND a.status IN ('PENDING', 'ACTIVE')
  AND (s.status <> 'ACTIVE' OR s.period_end <= now());

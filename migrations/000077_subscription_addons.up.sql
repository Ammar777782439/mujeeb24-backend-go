-- Paid, period-bound commercial add-ons.  They never alter plan or
-- subscription snapshots, and therefore preserve historical billing facts.
CREATE TABLE subscription_addons (
    id UUID PRIMARY KEY,
    subscription_id UUID NOT NULL REFERENCES subscriptions(id) ON DELETE RESTRICT,
    business_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('AI_REPLIES_100','AI_REPLIES_500','CATALOG_PRODUCTS_100','EXTRA_SEAT','CHANNEL_PASS')),
    price_yer_snapshot INT NOT NULL CHECK (price_yer_snapshot > 0),
    ai_reply_delta INT NOT NULL DEFAULT 0 CHECK (ai_reply_delta >= 0),
    catalog_delta INT NOT NULL DEFAULT 0 CHECK (catalog_delta >= 0),
    seat_delta INT NOT NULL DEFAULT 0 CHECK (seat_delta >= 0),
    channel_delta INT NOT NULL DEFAULT 0 CHECK (channel_delta >= 0),
    status TEXT NOT NULL CHECK (status IN ('PENDING','ACTIVE','CANCELLED')),
    activated_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    cancelled_reason TEXT,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT subscription_addons_business_subscription_fk FOREIGN KEY (business_id, subscription_id)
        REFERENCES subscriptions (business_id, id) ON DELETE RESTRICT,
    CONSTRAINT subscription_addons_activation_state_chk CHECK (
        (status = 'PENDING' AND activated_at IS NULL)
        OR (status = 'ACTIVE' AND activated_at IS NOT NULL)
        OR status = 'CANCELLED'
    ),
    CONSTRAINT subscription_addons_kind_delta_chk CHECK (
        (kind = 'AI_REPLIES_100' AND ai_reply_delta = 100 AND catalog_delta = 0 AND seat_delta = 0 AND channel_delta = 0)
        OR (kind = 'AI_REPLIES_500' AND ai_reply_delta = 500 AND catalog_delta = 0 AND seat_delta = 0 AND channel_delta = 0)
        OR (kind = 'CATALOG_PRODUCTS_100' AND ai_reply_delta = 0 AND catalog_delta = 100 AND seat_delta = 0 AND channel_delta = 0)
        OR (kind = 'EXTRA_SEAT' AND ai_reply_delta = 0 AND catalog_delta = 0 AND seat_delta = 1 AND channel_delta = 0)
        OR (kind = 'CHANNEL_PASS' AND ai_reply_delta = 0 AND catalog_delta = 0 AND seat_delta = 0 AND channel_delta = 1)
    )
);

CREATE INDEX subscription_addons_active_subscription_idx
    ON subscription_addons (subscription_id) WHERE status = 'ACTIVE';

CREATE TABLE subscription_addon_payments (
    id UUID PRIMARY KEY,
    addon_id UUID NOT NULL UNIQUE REFERENCES subscription_addons(id) ON DELETE RESTRICT,
    business_id UUID NOT NULL,
    amount_yer INT NOT NULL CHECK (amount_yer > 0),
    method TEXT NOT NULL CHECK (method IN ('CASH','BANK_TRANSFER','MOBILE_MONEY','OTHER')),
    reference TEXT NOT NULL CHECK (length(btrim(reference)) > 0),
    paid_at TIMESTAMPTZ NOT NULL,
    recorded_by UUID NOT NULL,
    idempotency_key TEXT NOT NULL CHECK (length(btrim(idempotency_key)) > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (recorded_by, idempotency_key)
);

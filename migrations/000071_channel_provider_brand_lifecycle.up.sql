-- A provider Brand is shared by all channels of ONE SaaS merchant.
-- Explicit last-channel disconnection can reserve remote deletion without
-- keeping a PostgreSQL transaction open during the SocialAPI DELETE request.
ALTER TABLE channel_provider_brands
    ADD COLUMN lifecycle_state TEXT NOT NULL DEFAULT 'active'
    CONSTRAINT channel_provider_brands_lifecycle_chk CHECK (lifecycle_state IN ('active','deleting'));

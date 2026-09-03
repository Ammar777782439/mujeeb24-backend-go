-- Forward-only hardening for tenant isolation (P0.2).
-- Prevent duplicate provider account mapping across tenants; ensure oauth_state is globally unique.
-- Historical Chatwoot columns remain untouched (see 000037).
CREATE UNIQUE INDEX IF NOT EXISTS uq_channel_connections_provider_account
    ON channel_connections (provider_ref, provider_account_ref)
    WHERE provider_account_ref IS NOT NULL AND length(btrim(provider_account_ref)) > 0;

CREATE UNIQUE INDEX IF NOT EXISTS uq_channel_provisioning_oauth_state
    ON channel_provisioning_sessions (oauth_state)
    WHERE oauth_state IS NOT NULL AND length(btrim(oauth_state)) > 0;

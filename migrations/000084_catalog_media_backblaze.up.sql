-- Support both Cloudflare R2 and Backblaze B2 without rewriting historical assets.
ALTER TABLE media_assets DROP CONSTRAINT media_assets_provider_chk;
ALTER TABLE media_assets ADD CONSTRAINT media_assets_provider_chk
    CHECK (storage_provider IN ('r2', 'backblaze'));

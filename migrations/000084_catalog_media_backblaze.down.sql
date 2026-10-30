-- Refuse a destructive downgrade while Backblaze assets remain.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM media_assets WHERE storage_provider = 'backblaze') THEN
        RAISE EXCEPTION 'Backblaze media assets must be migrated before reverting provider support';
    END IF;
END
$$;
ALTER TABLE media_assets DROP CONSTRAINT media_assets_provider_chk;
ALTER TABLE media_assets ADD CONSTRAINT media_assets_provider_chk
    CHECK (storage_provider = 'r2');

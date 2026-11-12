-- Refuse rollback while non-catalog media or conversation attachments exist.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM message_attachments) THEN
        RAISE EXCEPTION 'message attachments must be migrated or removed before reverting 000087';
    END IF;
    IF EXISTS (
        SELECT 1 FROM media_assets
        WHERE purpose <> 'catalog' OR status = 'pending_ingestion' OR media_kind <> 'image'
    ) THEN
        RAISE EXCEPTION 'non-catalog media assets must be migrated before reverting 000087';
    END IF;
END
$$;

DROP TABLE message_attachments;
DROP INDEX idx_media_assets_ingestion;

ALTER TABLE media_assets
    DROP CONSTRAINT media_assets_kind_chk,
    DROP COLUMN media_kind,
    DROP CONSTRAINT media_assets_purpose_chk,
    ADD CONSTRAINT media_assets_purpose_chk CHECK (purpose = 'catalog'),
    DROP CONSTRAINT media_assets_status_chk,
    ADD CONSTRAINT media_assets_status_chk
        CHECK (status IN ('pending_upload', 'ready', 'rejected', 'deleted'));

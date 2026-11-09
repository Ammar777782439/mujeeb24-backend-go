-- Generalize media ownership for Communication without changing Catalog relationships.
-- Existing catalog assets are backfilled as images. Catalog galleries remain
-- attached through catalog_media_links; conversation files use message_attachments.
ALTER TABLE media_assets
    ADD COLUMN media_kind TEXT NOT NULL DEFAULT 'image';

ALTER TABLE media_assets
    DROP CONSTRAINT media_assets_purpose_chk,
    ADD CONSTRAINT media_assets_purpose_chk
        CHECK (purpose IN ('catalog', 'conversation_attachment', 'business_brand')),
    DROP CONSTRAINT media_assets_status_chk,
    ADD CONSTRAINT media_assets_status_chk
        CHECK (status IN ('pending_upload', 'pending_ingestion', 'ready', 'rejected', 'deleted')),
    ADD CONSTRAINT media_assets_kind_chk
        CHECK (media_kind IN ('image', 'video', 'audio', 'document', 'file'));

CREATE INDEX idx_media_assets_ingestion
    ON media_assets (business_id, status, created_at)
    WHERE purpose = 'conversation_attachment' AND status = 'pending_ingestion';

CREATE TABLE message_attachments (
    id                              UUID PRIMARY KEY,
    business_id                     UUID NOT NULL,
    communication_message_id        UUID NOT NULL,
    media_asset_id                  UUID NOT NULL,
    kind                            TEXT NOT NULL,
    caption                         TEXT NOT NULL DEFAULT '',
    sort_order                      INTEGER NOT NULL DEFAULT 0,
    status                          TEXT NOT NULL DEFAULT 'pending_ingestion',
    source_provider_media_reference TEXT,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT message_attachments_business_fk
        FOREIGN KEY (business_id)
        REFERENCES businesses (id) ON DELETE RESTRICT,
    CONSTRAINT message_attachments_message_fk
        FOREIGN KEY (business_id, communication_message_id)
        REFERENCES communication_messages (business_id, id) ON DELETE RESTRICT,
    CONSTRAINT message_attachments_asset_fk
        FOREIGN KEY (business_id, media_asset_id)
        REFERENCES media_assets (business_id, id) ON DELETE RESTRICT,
    CONSTRAINT message_attachments_business_id_uq UNIQUE (business_id, id),
    CONSTRAINT message_attachments_message_asset_uq
        UNIQUE (business_id, communication_message_id, media_asset_id),
    CONSTRAINT message_attachments_kind_chk
        CHECK (kind IN ('image', 'video', 'audio', 'document', 'file')),
    CONSTRAINT message_attachments_caption_chk
        CHECK (char_length(caption) <= 2000),
    CONSTRAINT message_attachments_sort_order_chk
        CHECK (sort_order >= 0),
    CONSTRAINT message_attachments_status_chk
        CHECK (status IN ('pending_ingestion', 'ready', 'failed', 'removed')),
    CONSTRAINT message_attachments_provider_reference_chk
        CHECK (
            source_provider_media_reference IS NULL OR
            (
                length(btrim(source_provider_media_reference)) BETWEEN 1 AND 2048
                AND lower(btrim(source_provider_media_reference)) NOT LIKE 'http://%'
                AND lower(btrim(source_provider_media_reference)) NOT LIKE 'https://%'
            )
        ),
    CONSTRAINT message_attachments_pending_reference_chk
        CHECK (status <> 'pending_ingestion' OR source_provider_media_reference IS NOT NULL)
);

CREATE INDEX idx_message_attachments_timeline
    ON message_attachments (business_id, communication_message_id, sort_order, id)
    WHERE status <> 'removed';

CREATE INDEX idx_message_attachments_asset
    ON message_attachments (business_id, media_asset_id);

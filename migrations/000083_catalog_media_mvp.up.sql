-- Media MVP: catalog images only. No visual search, queues, or smart links.
-- Object bytes live in private object storage; PostgreSQL holds immutable references.
CREATE TABLE media_assets (
    id UUID PRIMARY KEY,
    business_id UUID NOT NULL,
    purpose TEXT NOT NULL DEFAULT 'catalog',
    storage_provider TEXT NOT NULL,
    storage_key TEXT NOT NULL,
    mime_type TEXT,
    size_bytes BIGINT,
    sha256 BYTEA,
    width INTEGER,
    height INTEGER,
    status TEXT NOT NULL DEFAULT 'pending_upload',
    created_by TEXT,
    upload_expires_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT media_assets_business_fk FOREIGN KEY (business_id)
        REFERENCES businesses (id) ON DELETE RESTRICT,
    CONSTRAINT media_assets_business_id_uq UNIQUE (business_id, id),
    CONSTRAINT media_assets_storage_key_uq UNIQUE (storage_provider, storage_key),
    CONSTRAINT media_assets_purpose_chk CHECK (purpose = 'catalog'),
    CONSTRAINT media_assets_provider_chk CHECK (storage_provider = 'r2'),
    CONSTRAINT media_assets_storage_key_chk CHECK (length(btrim(storage_key)) > 0),
    CONSTRAINT media_assets_mime_chk CHECK (mime_type IS NULL OR mime_type IN ('image/jpeg', 'image/png', 'image/webp')),
    CONSTRAINT media_assets_size_chk CHECK (size_bytes IS NULL OR (size_bytes > 0 AND size_bytes <= 10485760)),
    CONSTRAINT media_assets_sha256_chk CHECK (sha256 IS NULL OR octet_length(sha256) = 32),
    CONSTRAINT media_assets_dimensions_chk CHECK (
        (width IS NULL AND height IS NULL) OR
        (width > 0 AND height > 0 AND width <= 12000 AND height <= 12000)
    ),
    CONSTRAINT media_assets_status_chk CHECK (status IN ('pending_upload', 'ready', 'rejected', 'deleted')),
    CONSTRAINT media_assets_ready_complete_chk CHECK (
        status <> 'ready' OR
        (mime_type IS NOT NULL AND size_bytes IS NOT NULL AND sha256 IS NOT NULL
         AND width IS NOT NULL AND height IS NOT NULL AND deleted_at IS NULL)
    ),
    CONSTRAINT media_assets_deleted_marked_chk CHECK (status <> 'deleted' OR deleted_at IS NOT NULL)
);

CREATE INDEX idx_media_assets_business_status_created
    ON media_assets (business_id, status, created_at DESC);
CREATE INDEX idx_media_assets_staged_expiry
    ON media_assets (upload_expires_at)
    WHERE status = 'pending_upload' AND upload_expires_at IS NOT NULL;

-- A link belongs to exactly one product gallery (variant_id NULL) or one variant gallery.
-- The composite FK below rejects a variant from another product/tenant.
CREATE TABLE catalog_media_links (
    id UUID PRIMARY KEY,
    business_id UUID NOT NULL,
    catalog_item_id UUID NOT NULL,
    variant_id UUID,
    media_id UUID NOT NULL,
    is_primary BOOLEAN NOT NULL DEFAULT false,
    sort_order INTEGER NOT NULL DEFAULT 0,
    alt_text TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    approved_by TEXT,
    approved_at TIMESTAMPTZ,
    resource_version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT catalog_media_links_business_item_fk
        FOREIGN KEY (business_id, catalog_item_id)
        REFERENCES catalog_items (business_id, id) ON DELETE RESTRICT,
    CONSTRAINT catalog_media_links_business_item_variant_fk
        FOREIGN KEY (business_id, catalog_item_id, variant_id)
        REFERENCES variants (business_id, catalog_item_id, id) ON DELETE RESTRICT,
    CONSTRAINT catalog_media_links_business_asset_fk
        FOREIGN KEY (business_id, media_id)
        REFERENCES media_assets (business_id, id) ON DELETE RESTRICT,
    CONSTRAINT catalog_media_links_business_id_uq UNIQUE (business_id, id),
    CONSTRAINT catalog_media_links_status_chk CHECK (status IN ('active', 'removed')),
    CONSTRAINT catalog_media_links_active_approval_chk CHECK (status <> 'active' OR approved_at IS NOT NULL),
    CONSTRAINT catalog_media_links_primary_active_chk CHECK (NOT is_primary OR status = 'active'),
    CONSTRAINT catalog_media_links_order_chk CHECK (sort_order >= 0),
    CONSTRAINT catalog_media_links_version_chk CHECK (resource_version > 0),
    CONSTRAINT catalog_media_links_alt_chk CHECK (char_length(alt_text) <= 500)
);

-- PostgreSQL NULL semantics: product galleries require their own unique indexes.
CREATE UNIQUE INDEX uq_catalog_media_product_asset
    ON catalog_media_links (business_id, catalog_item_id, media_id)
    WHERE status = 'active' AND variant_id IS NULL;
CREATE UNIQUE INDEX uq_catalog_media_variant_asset
    ON catalog_media_links (business_id, catalog_item_id, variant_id, media_id)
    WHERE status = 'active' AND variant_id IS NOT NULL;
CREATE UNIQUE INDEX uq_catalog_media_product_primary
    ON catalog_media_links (business_id, catalog_item_id)
    WHERE status = 'active' AND is_primary AND variant_id IS NULL;
CREATE UNIQUE INDEX uq_catalog_media_variant_primary
    ON catalog_media_links (business_id, catalog_item_id, variant_id)
    WHERE status = 'active' AND is_primary AND variant_id IS NOT NULL;
CREATE INDEX idx_catalog_media_gallery
    ON catalog_media_links (business_id, catalog_item_id, variant_id, status, sort_order, id);
CREATE INDEX idx_catalog_media_asset
    ON catalog_media_links (business_id, media_id) WHERE status = 'active';

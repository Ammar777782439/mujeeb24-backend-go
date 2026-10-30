package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/jackc/pgx/v5"
)

// CatalogMediaRepository is deliberately separate from the commercial CatalogRepository.
// All SQL paths include the authenticated business scope; ID-only lookups are forbidden.
type CatalogMediaRepository struct{ adapter *Adapter }

func NewCatalogMediaRepository(adapter *Adapter) *CatalogMediaRepository {
	return &CatalogMediaRepository{adapter: adapter}
}

func (r *CatalogMediaRepository) executor(ctx context.Context) (SQLExecutor, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	return r.adapter.Executor(ctx)
}

func (r *CatalogMediaRepository) CreatePendingAsset(ctx context.Context, draft ports.PendingMediaAsset) (ports.MediaAsset, error) {
	db, err := r.executor(ctx)
	if err != nil {
		return ports.MediaAsset{}, err
	}
	if draft.BusinessID == "" || draft.ID == "" || draft.StorageKey == "" || draft.UploadExpiresAt.IsZero() {
		return ports.MediaAsset{}, errors.New("pending media requires business, asset, storage key and expiry")
	}
	var result ports.MediaAsset
	err = db.QueryRow(ctx, `INSERT INTO media_assets
        (id, business_id, purpose, storage_provider, storage_key, status, created_by, upload_expires_at)
        VALUES ($1::uuid, $2::uuid, 'catalog', 'r2', $3, 'pending_upload', $4, $5)
        RETURNING id::text, business_id::text, storage_key, status, created_at, updated_at`,
		draft.ID, draft.BusinessID, draft.StorageKey, draft.CreatedBy, draft.UploadExpiresAt,
	).Scan(&result.ID, &result.BusinessID, &result.StorageKey, &result.Status, &result.CreatedAt, &result.UpdatedAt)
	if err != nil {
		return ports.MediaAsset{}, fmt.Errorf("media asset insert: %w", err)
	}
	result.Purpose, result.StorageProvider = "catalog", "r2"
	result.UploadExpiresAt = &draft.UploadExpiresAt
	return result, nil
}

func (r *CatalogMediaRepository) GetAsset(ctx context.Context, businessID, mediaID string) (ports.MediaAsset, error) {
	db, err := r.executor(ctx)
	if err != nil {
		return ports.MediaAsset{}, err
	}
	var a ports.MediaAsset
	err = db.QueryRow(ctx, `SELECT id::text, business_id::text, purpose, storage_provider,
        storage_key, COALESCE(mime_type,''), COALESCE(size_bytes,0), sha256,
        COALESCE(width,0), COALESCE(height,0), status, upload_expires_at, deleted_at, created_at, updated_at
        FROM media_assets WHERE business_id=$1::uuid AND id=$2::uuid`, businessID, mediaID).Scan(
		&a.ID, &a.BusinessID, &a.Purpose, &a.StorageProvider, &a.StorageKey, &a.MimeType,
		&a.SizeBytes, &a.SHA256, &a.Width, &a.Height, &a.Status,
		&a.UploadExpiresAt, &a.DeletedAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return ports.MediaAsset{}, fmt.Errorf("media asset get: %w", err)
	}
	return a, nil
}

func (r *CatalogMediaRepository) MarkAssetReady(ctx context.Context, businessID, mediaID string, info ports.VerifiedImage) (ports.MediaAsset, error) {
	if (info.MimeType != "image/jpeg" && info.MimeType != "image/png" && info.MimeType != "image/webp") ||
		info.SizeBytes < 1 || info.SizeBytes > 10*1024*1024 || len(info.SHA256) != 32 ||
		info.Width < 1 || info.Height < 1 || info.Width > 12000 || info.Height > 12000 {
		return ports.MediaAsset{}, errors.New("invalid verified image metadata")
	}
	db, err := r.executor(ctx)
	if err != nil {
		return ports.MediaAsset{}, err
	}
	var a ports.MediaAsset
	err = db.QueryRow(ctx, `UPDATE media_assets SET status='ready', mime_type=$3,
        size_bytes=$4, sha256=$5, width=$6, height=$7, updated_at=now()
        WHERE business_id=$1::uuid AND id=$2::uuid AND status='pending_upload'
          AND upload_expires_at > now()
        RETURNING id::text, business_id::text, storage_key, status, created_at, updated_at`,
		businessID, mediaID, info.MimeType, info.SizeBytes, info.SHA256, info.Width, info.Height).
		Scan(&a.ID, &a.BusinessID, &a.StorageKey, &a.Status, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return ports.MediaAsset{}, fmt.Errorf("media completion rejected: %w", err)
	}
	a.Purpose, a.StorageProvider, a.MimeType = "catalog", "r2", info.MimeType
	a.SizeBytes, a.SHA256, a.Width, a.Height = info.SizeBytes, info.SHA256, info.Width, info.Height
	return a, nil
}

// LockGallery locks the parent item row. All gallery writers for an item,
// including variant galleries, serialize behind this row-level lock.
// Must be called inside a transaction owned by the application service.
func (r *CatalogMediaRepository) LockGallery(ctx context.Context, businessID, itemID string, variantID *string) error {
	db, err := r.executor(ctx)
	if err != nil {
		return err
	}
	var id string
	err = db.QueryRow(ctx, `SELECT i.id::text FROM catalog_items i
        WHERE i.business_id=$1::uuid AND i.id=$2::uuid AND i.status <> 'archived'
        FOR UPDATE`, businessID, itemID).Scan(&id)
	if err != nil {
		return fmt.Errorf("media gallery item lock: %w", err)
	}
	if variantID != nil {
		var valid bool
		err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM variants
            WHERE business_id=$1::uuid AND catalog_item_id=$2::uuid
              AND id=$3::uuid AND status='active')`, businessID, itemID, *variantID).Scan(&valid)
		if err != nil || !valid {
			return errors.New("media gallery variant not active or not in product")
		}
	}
	return nil
}

func (r *CatalogMediaRepository) ListGallery(ctx context.Context, businessID, itemID string, variantID *string) ([]ports.CatalogMediaLink, error) {
	db, err := r.executor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `SELECT id::text, business_id::text, catalog_item_id::text,
        variant_id::text, media_id::text, is_primary, sort_order, alt_text, status,
        approved_at, resource_version, created_at, updated_at
        FROM catalog_media_links WHERE business_id=$1::uuid AND catalog_item_id=$2::uuid
        AND variant_id IS NOT DISTINCT FROM $3::uuid AND status='active'
        ORDER BY is_primary DESC, sort_order, id`, businessID, itemID, variantID)
	if err != nil {
		return nil, fmt.Errorf("list catalog media: %w", err)
	}
	defer rows.Close()
	result := []ports.CatalogMediaLink{}
	for rows.Next() {
		var m ports.CatalogMediaLink
		if err := rows.Scan(&m.ID, &m.BusinessID, &m.CatalogItemID, &m.VariantID, &m.MediaID,
			&m.IsPrimary, &m.SortOrder, &m.AltText, &m.Status, &m.ApprovedAt,
			&m.ResourceVersion, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (r *CatalogMediaRepository) Attach(ctx context.Context, draft ports.CatalogMediaAttach, primary bool) (ports.CatalogMediaLink, error) {
	db, err := r.executor(ctx)
	if err != nil {
		return ports.CatalogMediaLink{}, err
	}
	if strings.TrimSpace(draft.ID) == "" || draft.ApprovedAt.IsZero() || draft.SortOrder < 0 || len([]rune(draft.AltText)) > 500 {
		return ports.CatalogMediaLink{}, errors.New("invalid catalog media attach request")
	}
	// SQL predicate is the authoritative gate: staged/removed assets cannot be linked.
	var ready bool
	err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_assets
       WHERE business_id=$1::uuid AND id=$2::uuid AND purpose='catalog'
         AND status='ready' AND deleted_at IS NULL)`, draft.BusinessID, draft.MediaID).Scan(&ready)
	if err != nil || !ready {
		return ports.CatalogMediaLink{}, errors.New("asset not ready or outside business")
	}
	if primary {
		_, err = db.Exec(ctx, `UPDATE catalog_media_links SET is_primary=false, resource_version=resource_version+1,
            updated_at=now() WHERE business_id=$1::uuid AND catalog_item_id=$2::uuid
            AND variant_id IS NOT DISTINCT FROM $3::uuid AND status='active' AND is_primary`,
			draft.BusinessID, draft.CatalogItemID, draft.VariantID)
		if err != nil {
			return ports.CatalogMediaLink{}, fmt.Errorf("unset gallery primary: %w", err)
		}
	}
	var m ports.CatalogMediaLink
	err = db.QueryRow(ctx, `INSERT INTO catalog_media_links
        (id,business_id,catalog_item_id,variant_id,media_id,is_primary,sort_order,alt_text,status,approved_by,approved_at)
        VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,$7,$8,'active',$9,$10)
        RETURNING id::text,business_id::text,catalog_item_id::text,variant_id::text,
           media_id::text,is_primary,sort_order,alt_text,status,approved_at,resource_version,created_at,updated_at`,
		draft.ID, draft.BusinessID, draft.CatalogItemID, draft.VariantID, draft.MediaID, primary,
		draft.SortOrder, draft.AltText, draft.ApprovedBy, draft.ApprovedAt).Scan(
		&m.ID, &m.BusinessID, &m.CatalogItemID, &m.VariantID, &m.MediaID,
		&m.IsPrimary, &m.SortOrder, &m.AltText, &m.Status, &m.ApprovedAt,
		&m.ResourceVersion, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return ports.CatalogMediaLink{}, fmt.Errorf("attach media: %w", err)
	}
	return m, nil
}

func (r *CatalogMediaRepository) SetPrimary(ctx context.Context, businessID, itemID, linkID string, variantID *string, _ string) error {
	db, err := r.executor(ctx)
	if err != nil {
		return err
	}
	var valid bool
	err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_media_links
        WHERE business_id=$1::uuid AND catalog_item_id=$2::uuid AND id=$3::uuid
          AND variant_id IS NOT DISTINCT FROM $4::uuid AND status='active')`,
		businessID, itemID, linkID, variantID).Scan(&valid)
	if err != nil || !valid {
		return errors.New("media link not in active gallery")
	}
	_, err = db.Exec(ctx, `UPDATE catalog_media_links
        SET is_primary=false,resource_version=resource_version+1,updated_at=now()
        WHERE business_id=$1::uuid AND catalog_item_id=$2::uuid
          AND variant_id IS NOT DISTINCT FROM $3::uuid AND is_primary AND status='active'`,
		businessID, itemID, variantID)
	if err != nil {
		return err
	}
	tag, err := db.Exec(ctx, `UPDATE catalog_media_links
        SET is_primary=true,resource_version=resource_version+1,updated_at=now()
        WHERE business_id=$1::uuid AND catalog_item_id=$2::uuid AND id=$3::uuid
          AND variant_id IS NOT DISTINCT FROM $4::uuid AND status='active'`,
		businessID, itemID, linkID, variantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("media primary update missed")
	}
	return nil
}

func (r *CatalogMediaRepository) Remove(ctx context.Context, businessID, itemID, linkID string, variantID *string, _ string) error {
	db, err := r.executor(ctx)
	if err != nil {
		return err
	}
	var primary bool
	err = db.QueryRow(ctx, `SELECT is_primary FROM catalog_media_links
       WHERE business_id=$1::uuid AND catalog_item_id=$2::uuid AND id=$3::uuid
         AND variant_id IS NOT DISTINCT FROM $4::uuid AND status='active'`,
		businessID, itemID, linkID, variantID).Scan(&primary)
	if err != nil {
		return fmt.Errorf("remove media link not found: %w", err)
	}
	_, err = db.Exec(ctx, `UPDATE catalog_media_links
        SET status='removed',is_primary=false,resource_version=resource_version+1,updated_at=now()
        WHERE business_id=$1::uuid AND catalog_item_id=$2::uuid AND id=$3::uuid`,
		businessID, itemID, linkID)
	if err != nil {
		return err
	}
	if !primary {
		return nil
	}
	// Re-elect a primary in the same transaction. Zero images is valid.
	var replacement string
	err = db.QueryRow(ctx, `SELECT id::text FROM catalog_media_links
        WHERE business_id=$1::uuid AND catalog_item_id=$2::uuid
          AND variant_id IS NOT DISTINCT FROM $3::uuid AND status='active'
        ORDER BY sort_order,id LIMIT 1`, businessID, itemID, variantID).Scan(&replacement)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `UPDATE catalog_media_links SET is_primary=true,
        resource_version=resource_version+1,updated_at=now()
        WHERE business_id=$1::uuid AND id=$2::uuid`, businessID, replacement)
	return err
}

// PrimaryForReply returns only the approved image for the exact requested gallery.
// NEVER fall back from a variant image to a potentially different colour.
func (r *CatalogMediaRepository) PrimaryForReply(ctx context.Context, businessID, itemID string, variantID *string) (*ports.CatalogMediaLink, error) {
	db, err := r.executor(ctx)
	if err != nil {
		return nil, err
	}
	var m ports.CatalogMediaLink
	err = db.QueryRow(ctx, `SELECT l.id::text,l.business_id::text,l.catalog_item_id::text,
        l.variant_id::text,l.media_id::text,l.is_primary,l.sort_order,l.alt_text,
        l.status,l.approved_at,l.resource_version,l.created_at,l.updated_at
        FROM catalog_media_links l
        JOIN media_assets a ON a.business_id=l.business_id AND a.id=l.media_id
        JOIN catalog_items i ON i.business_id=l.business_id AND i.id=l.catalog_item_id
        JOIN catalogs c ON c.business_id=i.business_id AND c.id=i.catalog_id
        LEFT JOIN variants v ON v.business_id=l.business_id
            AND v.catalog_item_id=l.catalog_item_id AND v.id=l.variant_id
        WHERE l.business_id=$1::uuid AND l.catalog_item_id=$2::uuid
          AND l.variant_id IS NOT DISTINCT FROM $3::uuid
          AND l.status='active' AND l.approved_at IS NOT NULL AND l.is_primary
          AND a.status='ready' AND a.deleted_at IS NULL
          AND i.status='active' AND c.status='active'
          AND (l.variant_id IS NULL OR v.status='active')
        LIMIT 1`, businessID, itemID, variantID).Scan(
		&m.ID, &m.BusinessID, &m.CatalogItemID, &m.VariantID, &m.MediaID,
		&m.IsPrimary, &m.SortOrder, &m.AltText, &m.Status, &m.ApprovedAt,
		&m.ResourceVersion, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve catalog primary media: %w", err)
	}
	return &m, nil
}

// FindApprovedMediaLink rechecks an exact approved gallery link at AutoReply creation,
// including current product, variant and catalog status. Deleted/reassigned
// photos are never sent by the delayed outbox worker.
func (r *CatalogMediaRepository) FindApprovedMediaLink(ctx context.Context, businessID, linkID string) (*ports.CatalogMediaLink, error) {
	db, err := r.executor(ctx)
	if err != nil {
		return nil, err
	}
	var m ports.CatalogMediaLink
	err = db.QueryRow(ctx, `SELECT l.id::text,l.business_id::text,l.catalog_item_id::text,
    l.variant_id::text,l.media_id::text,l.is_primary,l.sort_order,l.alt_text,
    l.status,l.approved_at,l.resource_version,l.created_at,l.updated_at
    FROM catalog_media_links l
    JOIN media_assets a ON a.business_id=l.business_id AND a.id=l.media_id
    JOIN catalog_items i ON i.business_id=l.business_id AND i.id=l.catalog_item_id
    JOIN catalogs c ON c.business_id=i.business_id AND c.id=i.catalog_id
    LEFT JOIN variants v ON v.business_id=l.business_id
       AND v.catalog_item_id=l.catalog_item_id AND v.id=l.variant_id
    WHERE l.business_id=$1::uuid AND l.id=$2::uuid
      AND l.status='active' AND l.approved_at IS NOT NULL
      AND a.status='ready' AND a.deleted_at IS NULL
      AND i.status='active' AND c.status='active'
      AND (l.variant_id IS NULL OR v.status='active')
    LIMIT 1`, businessID, linkID).Scan(
		&m.ID, &m.BusinessID, &m.CatalogItemID, &m.VariantID, &m.MediaID,
		&m.IsPrimary, &m.SortOrder, &m.AltText, &m.Status, &m.ApprovedAt,
		&m.ResourceVersion, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find approved media link: %w", err)
	}
	return &m, nil
}

var _ ports.CatalogMediaRepository = (*CatalogMediaRepository)(nil)

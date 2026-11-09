package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type ConversationMediaIngestionRepository struct {
	adapter *Adapter
}

func NewConversationMediaIngestionRepository(adapter *Adapter) *ConversationMediaIngestionRepository {
	return &ConversationMediaIngestionRepository{adapter: adapter}
}

func (r *ConversationMediaIngestionRepository) executor(ctx context.Context) (SQLExecutor, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	return r.adapter.Executor(ctx)
}

func (r *ConversationMediaIngestionRepository) CreatePendingIngestion(ctx context.Context, draft ports.PendingConversationMediaAsset) (ports.MediaAsset, error) {
	if strings.TrimSpace(draft.ID) == "" || strings.TrimSpace(draft.BusinessID) == "" ||
		strings.TrimSpace(draft.StorageKey) == "" || strings.TrimSpace(draft.Kind) == "" {
		return ports.MediaAsset{}, invalidRepositoryInput("conversation_media.create_pending", "business, asset id, storage key and kind are required")
	}
	// All verified provider kinds can be materialized as pending placeholders.
	// Only the image verifier currently has a path to mark an asset ready.
	switch draft.Kind {
	case "image", "video", "audio", "document", "file":
	default:
		return ports.MediaAsset{}, invalidRepositoryInput("conversation_media.create_pending", "unsupported conversation media kind")
	}
	expectedStoragePrefix := "conversation/" + draft.BusinessID + "/" + draft.ID
	if strings.HasPrefix(draft.StorageKey, "/") || strings.Contains(draft.StorageKey, "..") ||
		(draft.StorageKey != expectedStoragePrefix && !strings.HasPrefix(draft.StorageKey, expectedStoragePrefix+".")) {
		return ports.MediaAsset{}, invalidRepositoryInput("conversation_media.create_pending", "storage key must be generated under the tenant and asset namespace")
	}
	provider := strings.TrimSpace(draft.StorageProvider)
	if provider == "" {
		provider = "r2"
	}
	if provider != "r2" && provider != "backblaze" {
		return ports.MediaAsset{}, invalidRepositoryInput("conversation_media.create_pending", "unsupported storage provider")
	}

	db, err := r.executor(ctx)
	if err != nil {
		return ports.MediaAsset{}, err
	}
	var asset ports.MediaAsset
	err = db.QueryRow(ctx, `
		INSERT INTO media_assets
			(id, business_id, purpose, media_kind, storage_provider, storage_key, status, created_by)
		VALUES ($1::uuid, $2::uuid, 'conversation_attachment', $3, $4, $5, 'pending_ingestion', $6)
		RETURNING id::text, business_id::text, purpose, media_kind, storage_provider,
		          storage_key, status, created_at, updated_at
	`, draft.ID, draft.BusinessID, draft.Kind, provider, draft.StorageKey, draft.CreatedBy).Scan(
		&asset.ID, &asset.BusinessID, &asset.Purpose, &asset.Kind, &asset.StorageProvider,
		&asset.StorageKey, &asset.Status, &asset.CreatedAt, &asset.UpdatedAt,
	)
	if err != nil {
		return ports.MediaAsset{}, classifyRepositoryWriteError("conversation_media.create_pending", err)
	}
	return asset, nil
}

func (r *ConversationMediaIngestionRepository) GetAsset(ctx context.Context, businessID, mediaAssetID string) (ports.MediaAsset, error) {
	db, err := r.executor(ctx)
	if err != nil {
		return ports.MediaAsset{}, err
	}
	var asset ports.MediaAsset
	err = db.QueryRow(ctx, `
		SELECT id::text, business_id::text, purpose, media_kind, storage_provider,
		       storage_key, COALESCE(mime_type,''), COALESCE(size_bytes,0), sha256,
		       COALESCE(width,0), COALESCE(height,0), status, upload_expires_at,
		       deleted_at, created_at, updated_at
		FROM media_assets
		WHERE business_id = $1::uuid AND id = $2::uuid AND purpose = 'conversation_attachment'
	`, businessID, mediaAssetID).Scan(
		&asset.ID, &asset.BusinessID, &asset.Purpose, &asset.Kind, &asset.StorageProvider,
		&asset.StorageKey, &asset.MimeType, &asset.SizeBytes, &asset.SHA256,
		&asset.Width, &asset.Height, &asset.Status, &asset.UploadExpiresAt,
		&asset.DeletedAt, &asset.CreatedAt, &asset.UpdatedAt,
	)
	if err != nil {
		return ports.MediaAsset{}, classifyRepositoryGetError("conversation_media.get", err)
	}
	return asset, nil
}

func (r *ConversationMediaIngestionRepository) MarkImageReady(ctx context.Context, businessID, mediaAssetID string, verified ports.VerifiedImage) (ports.MediaAsset, error) {
	if verified.MimeType != "image/jpeg" && verified.MimeType != "image/png" && verified.MimeType != "image/webp" {
		return ports.MediaAsset{}, invalidRepositoryInput("conversation_media.mark_ready", "unsupported verified image MIME type")
	}
	if verified.SizeBytes < 1 || verified.SizeBytes > 10*1024*1024 ||
		len(verified.SHA256) != 32 || verified.Width < 1 || verified.Height < 1 ||
		verified.Width > 12000 || verified.Height > 12000 ||
		int64(verified.Width)*int64(verified.Height) > 16_000_000 {
		return ports.MediaAsset{}, invalidRepositoryInput("conversation_media.mark_ready", "verified image metadata is outside accepted bounds")
	}
	db, err := r.executor(ctx)
	if err != nil {
		return ports.MediaAsset{}, err
	}
	var asset ports.MediaAsset
	err = db.QueryRow(ctx, `
		UPDATE media_assets
		SET status = 'ready', mime_type = $3, size_bytes = $4, sha256 = $5,
		    width = $6, height = $7, updated_at = now()
		WHERE business_id = $1::uuid AND id = $2::uuid
		  AND purpose = 'conversation_attachment'
		  AND media_kind = 'image' AND status = 'pending_ingestion'
		RETURNING id::text, business_id::text, purpose, media_kind, storage_provider,
		          storage_key, mime_type, size_bytes, sha256, width, height,
		          status, upload_expires_at, deleted_at, created_at, updated_at
	`, businessID, mediaAssetID, verified.MimeType, verified.SizeBytes, verified.SHA256, verified.Width, verified.Height).Scan(
		&asset.ID, &asset.BusinessID, &asset.Purpose, &asset.Kind, &asset.StorageProvider,
		&asset.StorageKey, &asset.MimeType, &asset.SizeBytes, &asset.SHA256,
		&asset.Width, &asset.Height, &asset.Status, &asset.UploadExpiresAt,
		&asset.DeletedAt, &asset.CreatedAt, &asset.UpdatedAt,
	)
	if err != nil {
		return ports.MediaAsset{}, classifyRepositoryWriteError("conversation_media.mark_ready", err)
	}
	return asset, nil
}

func (r *ConversationMediaIngestionRepository) MarkIngestionRejected(ctx context.Context, businessID, mediaAssetID string) (ports.MediaAsset, error) {
	db, err := r.executor(ctx)
	if err != nil {
		return ports.MediaAsset{}, err
	}
	var asset ports.MediaAsset
	err = db.QueryRow(ctx, `
		UPDATE media_assets
		SET status = 'rejected', updated_at = now()
		WHERE business_id = $1::uuid AND id = $2::uuid
		  AND purpose = 'conversation_attachment'
		  AND status = 'pending_ingestion'
		RETURNING id::text, business_id::text, purpose, media_kind, storage_provider,
		          storage_key, status, created_at, updated_at
	`, businessID, mediaAssetID).Scan(
		&asset.ID, &asset.BusinessID, &asset.Purpose, &asset.Kind,
		&asset.StorageProvider, &asset.StorageKey, &asset.Status, &asset.CreatedAt, &asset.UpdatedAt,
	)
	if err != nil {
		return ports.MediaAsset{}, classifyRepositoryWriteError("conversation_media.reject", err)
	}
	return asset, nil
}

var _ ports.ConversationMediaIngestionRepository = (*ConversationMediaIngestionRepository)(nil)

type MessageAttachmentRepository struct {
	adapter *Adapter
}

func NewMessageAttachmentRepository(adapter *Adapter) *MessageAttachmentRepository {
	return &MessageAttachmentRepository{adapter: adapter}
}

func (r *MessageAttachmentRepository) executor(ctx context.Context) (SQLExecutor, error) {
	if r == nil || r.adapter == nil {
		return nil, ErrPoolClosed
	}
	return r.adapter.Executor(ctx)
}

func (r *MessageAttachmentRepository) Create(ctx context.Context, draft ports.MessageAttachmentDraft) (ports.MessageAttachmentRecord, error) {
	if strings.TrimSpace(draft.ID) == "" || strings.TrimSpace(draft.BusinessID) == "" ||
		strings.TrimSpace(draft.CommunicationMessageID) == "" || strings.TrimSpace(draft.MediaAssetID) == "" ||
		strings.TrimSpace(draft.Kind) == "" || draft.SortOrder < 0 || draft.CreatedAt.IsZero() ||
		len([]rune(draft.Caption)) > 2000 {
		return ports.MessageAttachmentRecord{}, invalidRepositoryInput("message_attachment.create", "required identifiers, kind, timestamp or valid order/caption are missing")
	}
	if draft.Kind != "image" && draft.Kind != "video" && draft.Kind != "audio" && draft.Kind != "document" && draft.Kind != "file" {
		return ports.MessageAttachmentRecord{}, invalidRepositoryInput("message_attachment.create", "unsupported attachment kind")
	}
	if draft.SourceProviderMediaRef != nil {
		ref := strings.TrimSpace(*draft.SourceProviderMediaRef)
		if ref == "" || len(ref) > 2048 || strings.HasPrefix(strings.ToLower(ref), "http://") || strings.HasPrefix(strings.ToLower(ref), "https://") {
			return ports.MessageAttachmentRecord{}, invalidRepositoryInput("message_attachment.create", "provider media reference must be a non-URL opaque reference")
		}
		draft.SourceProviderMediaRef = &ref
	}
	db, err := r.executor(ctx)
	if err != nil {
		return ports.MessageAttachmentRecord{}, err
	}
	row := db.QueryRow(ctx, `
		WITH created AS (
			INSERT INTO message_attachments
				(id, business_id, communication_message_id, media_asset_id, kind,
				 caption, sort_order, status, source_provider_media_reference, created_at, updated_at)
			SELECT $1::uuid, $2::uuid, $3::uuid, a.id, $5, $6, $7,
			       CASE WHEN a.status = 'ready' THEN 'ready' ELSE 'pending_ingestion' END,
			       CASE WHEN a.status = 'ready' THEN NULL ELSE $8 END, $9, $9
			FROM media_assets a
			WHERE a.business_id = $2::uuid AND a.id = $4::uuid
			  AND a.purpose = 'conversation_attachment'
			  AND a.status IN ('pending_ingestion', 'ready')
			  AND a.media_kind = $5
			RETURNING id, business_id, communication_message_id, media_asset_id,
			          kind, caption, sort_order, status, created_at, updated_at
		)
		SELECT c.id::text, c.business_id::text, c.communication_message_id::text,
		       c.media_asset_id::text, c.kind, c.caption, c.sort_order, c.status,
		       c.created_at, c.updated_at, COALESCE(a.mime_type,''), COALESCE(a.size_bytes,0)
		FROM created c
		JOIN media_assets a ON a.business_id = c.business_id AND a.id = c.media_asset_id
	`, draft.ID, draft.BusinessID, draft.CommunicationMessageID, draft.MediaAssetID,
		draft.Kind, draft.Caption, draft.SortOrder, optionalStringValue(draft.SourceProviderMediaRef), draft.CreatedAt)
	return scanMessageAttachment(row, "message_attachment.create", true)
}

func (r *MessageAttachmentRepository) ListByMessage(ctx context.Context, businessID, communicationMessageID string) ([]ports.MessageAttachmentRecord, error) {
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(communicationMessageID) == "" {
		return nil, invalidRepositoryInput("message_attachment.list", "business and communication message ids are required")
	}
	db, err := r.executor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `
		SELECT ma.id::text, ma.business_id::text, ma.communication_message_id::text,
		       ma.media_asset_id::text, ma.kind, ma.caption, ma.sort_order, ma.status,
		       ma.created_at, ma.updated_at, COALESCE(a.mime_type,''), COALESCE(a.size_bytes,0)
		FROM message_attachments ma
		JOIN media_assets a
		  ON a.business_id = ma.business_id AND a.id = ma.media_asset_id
		WHERE ma.business_id = $1::uuid AND ma.communication_message_id = $2::uuid
		ORDER BY ma.sort_order, ma.id
	`, businessID, communicationMessageID)
	if err != nil {
		return nil, classifyRepositoryGetError("message_attachment.list", err)
	}
	defer rows.Close()
	result := make([]ports.MessageAttachmentRecord, 0)
	for rows.Next() {
		var item ports.MessageAttachmentRecord
		if err := rows.Scan(&item.ID, &item.BusinessID, &item.CommunicationMessageID,
			&item.MediaAssetID, &item.Kind, &item.Caption, &item.SortOrder,
			&item.Status, &item.CreatedAt, &item.UpdatedAt, &item.MimeType, &item.SizeBytes); err != nil {
			return nil, fmt.Errorf("scan message attachment: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate message attachments: %w", err)
	}
	return result, nil
}

func (r *MessageAttachmentRepository) MarkReady(ctx context.Context, businessID, attachmentID string) (ports.MessageAttachmentRecord, error) {
	db, err := r.executor(ctx)
	if err != nil {
		return ports.MessageAttachmentRecord{}, err
	}
	row := db.QueryRow(ctx, `
		WITH updated AS (
			UPDATE message_attachments ma
			SET status = 'ready', source_provider_media_reference = NULL, updated_at = now()
			WHERE ma.business_id = $1::uuid AND ma.id = $2::uuid
			  AND ma.status = 'pending_ingestion'
			  AND EXISTS (
				SELECT 1 FROM media_assets a
				WHERE a.business_id = ma.business_id AND a.id = ma.media_asset_id
				  AND a.purpose = 'conversation_attachment' AND a.status = 'ready'
				  AND a.media_kind = ma.kind
			  )
			RETURNING ma.*
		)
		SELECT u.id::text, u.business_id::text, u.communication_message_id::text,
		       u.media_asset_id::text, u.kind, u.caption, u.sort_order, u.status,
		       u.created_at, u.updated_at, COALESCE(a.mime_type,''), COALESCE(a.size_bytes,0)
		FROM updated u
		JOIN media_assets a ON a.business_id = u.business_id AND a.id = u.media_asset_id
	`, businessID, attachmentID)
	return scanMessageAttachment(row, "message_attachment.mark_ready", false)
}

func (r *MessageAttachmentRepository) MarkFailed(ctx context.Context, businessID, attachmentID string) (ports.MessageAttachmentRecord, error) {
	db, err := r.executor(ctx)
	if err != nil {
		return ports.MessageAttachmentRecord{}, err
	}
	row := db.QueryRow(ctx, `
		WITH updated AS (
			UPDATE message_attachments
			SET status = 'failed', updated_at = now()
			WHERE business_id = $1::uuid AND id = $2::uuid AND status = 'pending_ingestion'
			RETURNING *
		)
		SELECT u.id::text, u.business_id::text, u.communication_message_id::text,
		       u.media_asset_id::text, u.kind, u.caption, u.sort_order, u.status,
		       u.created_at, u.updated_at, COALESCE(a.mime_type,''), COALESCE(a.size_bytes,0)
		FROM updated u
		JOIN media_assets a ON a.business_id = u.business_id AND a.id = u.media_asset_id
	`, businessID, attachmentID)
	return scanMessageAttachment(row, "message_attachment.mark_failed", false)
}

func scanMessageAttachment(row interface{ Scan(...any) error }, operation string, write bool) (ports.MessageAttachmentRecord, error) {
	var record ports.MessageAttachmentRecord
	err := row.Scan(&record.ID, &record.BusinessID, &record.CommunicationMessageID,
		&record.MediaAssetID, &record.Kind, &record.Caption, &record.SortOrder,
		&record.Status, &record.CreatedAt, &record.UpdatedAt, &record.MimeType, &record.SizeBytes)
	if err == nil {
		return record, nil
	}
	if write {
		return record, classifyRepositoryWriteError(operation, err)
	}
	return record, classifyRepositoryGetError(operation, err)
}

var _ ports.MessageAttachmentRepository = (*MessageAttachmentRepository)(nil)

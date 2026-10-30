package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

const MaxCatalogMediaBytes = 10 * 1024 * 1024

// CatalogImageUploadService handles bounded, server-verified image uploads.
// No browser-supplied MIME/hash/dimensions are trusted. Object storage I/O
// happens outside SQL transactions.
type CatalogImageUploadService struct {
	Repository      ports.CatalogMediaRepository
	Objects         ports.MediaObjectStorage
	StorageProvider string
	Now            func() time.Time
	NewID          func() string
}

func (s CatalogImageUploadService) UploadBase64(ctx context.Context, businessID, encoded string, createdBy *string) (ports.MediaAsset, error) {
	if s.Repository == nil || s.Objects == nil {
		return ports.MediaAsset{}, errors.New("media uploads are not configured")
	}
	if _, err := uuid.Parse(businessID); err != nil {
		return ports.MediaAsset{}, errors.New("invalid business")
	}
	if encoded == "" || len(encoded) > base64.StdEncoding.EncodedLen(MaxCatalogMediaBytes) {
		return ports.MediaAsset{}, errors.New("image upload size exceeds limit")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) == 0 || len(data) > MaxCatalogMediaBytes {
		return ports.MediaAsset{}, errors.New("invalid image base64 or upload size")
	}
	// Only formats whose dimensions are checked by the Go standard library are
	// admitted in the first release. WebP is intentionally rejected, not mislabeled.
	format := http.DetectContentType(data)
	if format != "image/jpeg" && format != "image/png" {
		return ports.MediaAsset{}, errors.New("unsupported image format; use JPEG or PNG")
	}
	config, _, decodeErr := image.DecodeConfig(bytes.NewReader(data))
	if decodeErr != nil || config.Width < 1 || config.Height < 1 || config.Width > 12000 || config.Height > 12000 {
		return ports.MediaAsset{}, errors.New("invalid image dimensions")
	}
	if int64(config.Width)*int64(config.Height) > 16_000_000 {
		return ports.MediaAsset{}, errors.New("image pixel count exceeds limit")
	}
	// Decode the full stream to reject truncated/corrupted image bodies.
	if _, _, decodeErr = image.Decode(bytes.NewReader(data)); decodeErr != nil {
		return ports.MediaAsset{}, errors.New("corrupted image body")
	}
	id := uuid.NewString()
	if s.NewID != nil {
		id = s.NewID()
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	extension := ".jpg"
	if format == "image/png" {
		extension = ".png"
	}
	key := "catalog/" + businessID + "/" + id + extension
	provider := s.StorageProvider
	if provider == "" {
		provider = "r2"
	}
	if provider != "r2" && provider != "backblaze" {
		return ports.MediaAsset{}, errors.New("unsupported image storage provider")
	}
	_, err = s.Repository.CreatePendingAsset(ctx, ports.PendingMediaAsset{
		ID: id, BusinessID: businessID, StorageKey: key, StorageProvider: provider,
		UploadExpiresAt: now.Add(30 * time.Minute), CreatedBy: createdBy,
	})
	if err != nil {
		log.Printf("[CatalogMedia] stage=asset_stage_failed business=%s provider=%s cause=%v", businessID, provider, err)
		return ports.MediaAsset{}, err
	}
	log.Printf("[CatalogMedia] stage=asset_staged business=%s provider=%s asset=%s mime=%s bytes=%d", businessID, provider, id, format, len(data))
	if err = s.Objects.Put(ctx, key, format, data); err != nil {
		log.Printf("[CatalogMedia] stage=object_write_failed business=%s provider=%s asset=%s cause=%v", businessID, provider, id, err)
		return ports.MediaAsset{}, err
	}
	log.Printf("[CatalogMedia] stage=object_written business=%s provider=%s asset=%s", businessID, provider, id)
	digest := sha256.Sum256(data)
	asset, err := s.Repository.MarkAssetReady(ctx, businessID, id, ports.VerifiedImage{
		MimeType: format, SizeBytes: int64(len(data)), SHA256: digest[:], Width: config.Width, Height: config.Height,
	})
	if err != nil {
		log.Printf("[CatalogMedia] stage=asset_ready_failed business=%s provider=%s asset=%s cause=%v", businessID, provider, id, err)
		// Failure remains visible in a staged row for reconciliation. Storage is
		// best-effort deleted to avoid orphan bytes if SQL completion fails.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Objects.Delete(cleanupCtx, key)
		return ports.MediaAsset{}, err
	}
	asset.MimeType = strings.ToLower(asset.MimeType)
	log.Printf("[CatalogMedia] stage=upload_committed business=%s provider=%s asset=%s bytes=%d", businessID, provider, id, len(data))
	return asset, nil
}

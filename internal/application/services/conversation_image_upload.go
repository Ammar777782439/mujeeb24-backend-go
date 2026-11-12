package services

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

// ConversationImageUploadService owns the merchant upload flow for private
// conversation media. Catalog assets deliberately use a separate repository.
type ConversationImageUploadService struct {
	Assets          ports.ConversationMediaIngestionRepository
	Objects         ports.MediaObjectStorage
	StorageProvider string
	NewID           func() string
}

func (s ConversationImageUploadService) UploadBase64(ctx context.Context, businessID, encoded string, createdBy *string) (ports.MediaAsset, error) {
	if s.Assets == nil || s.Objects == nil {
		return ports.MediaAsset{}, appErrors.NotImplemented()
	}
	businessID = strings.TrimSpace(businessID)
	if _, err := uuid.Parse(businessID); err != nil {
		return ports.MediaAsset{}, appErrors.New(appErrors.CodeValidation, "invalid business id")
	}
	if strings.TrimSpace(encoded) == "" || len(encoded) > base64.StdEncoding.EncodedLen(maxConversationImageBytes) {
		return ports.MediaAsset{}, appErrors.New(appErrors.CodeValidation, "image upload is empty or exceeds the size limit")
	}
	body, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(body) == 0 || len(body) > maxConversationImageBytes {
		return ports.MediaAsset{}, appErrors.New(appErrors.CodeValidation, "invalid base64 image or upload size")
	}
	verified, mimeType, err := verifyConversationImage(body)
	if err != nil {
		return ports.MediaAsset{}, appErrors.New(appErrors.CodeValidation, "conversation attachment must be a valid JPEG or PNG image within the supported dimensions")
	}
	writer, ok := s.Objects.(ports.MediaObjectProviderWriter)
	if !ok {
		return ports.MediaAsset{}, appErrors.New(appErrors.CodeExternalDependency, "private conversation media storage is unavailable")
	}

	provider := strings.TrimSpace(s.StorageProvider)
	if provider == "" {
		provider = "r2"
	}
	if provider != "r2" && provider != "backblaze" {
		return ports.MediaAsset{}, appErrors.New(appErrors.CodeValidation, "unsupported conversation media storage provider")
	}
	id := uuid.NewString()
	if s.NewID != nil {
		id = s.NewID()
	}
	if _, err := uuid.Parse(id); err != nil {
		return ports.MediaAsset{}, appErrors.New(appErrors.CodeValidation, "invalid generated media asset id")
	}
	extension := ".jpg"
	if mimeType == "image/png" {
		extension = ".png"
	}
	key := "conversation/" + businessID + "/" + id + extension

	asset, err := s.Assets.CreatePendingIngestion(ctx, ports.PendingConversationMediaAsset{
		ID: id, BusinessID: businessID, StorageProvider: provider,
		StorageKey: key, Kind: "image", CreatedBy: createdBy,
	})
	if err != nil {
		return ports.MediaAsset{}, err
	}
	if asset.ID != id || asset.BusinessID != businessID || asset.Purpose != "conversation_attachment" ||
		asset.Kind != "image" || asset.Status != "pending_ingestion" ||
		asset.StorageProvider != provider || asset.StorageKey != key {
		_, rejectErr := s.Assets.MarkIngestionRejected(ctx, businessID, id)
		return ports.MediaAsset{}, &appErrors.Error{
			Code: appErrors.CodeInvalidState, Message: "conversation media repository returned an inconsistent pending asset",
			Cause: rejectErr,
		}
	}

	// Object storage is intentionally outside the repository's SQL operation.
	if err := writer.PutToProvider(ctx, asset.StorageProvider, asset.StorageKey, mimeType, body); err != nil {
		rejected, rejectErr := s.Assets.MarkIngestionRejected(ctx, businessID, asset.ID)
		var cleanupErr error
		if rejectErr == nil && rejected.Status == "rejected" {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cleanupErr = writer.DeleteFromProvider(cleanupCtx, asset.StorageProvider, asset.StorageKey)
			cancel()
		}
		return ports.MediaAsset{}, &appErrors.Error{
			Code: appErrors.CodeExternalDependency, Message: "conversation media could not be stored",
			Retryable: true, Cause: errors.Join(err, rejectErr, cleanupErr),
		}
	}
	ready, err := s.Assets.MarkImageReady(ctx, businessID, asset.ID, verified)
	if err != nil {
		// The upload is not returned as usable unless the authoritative metadata
		// transition succeeds. Clean the private object and leave a terminal row.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		rejected, rejectErr := s.Assets.MarkIngestionRejected(cleanupCtx, businessID, asset.ID)
		var cleanupErr error
		if rejectErr == nil && rejected.Status == "rejected" {
			cleanupErr = writer.DeleteFromProvider(cleanupCtx, asset.StorageProvider, asset.StorageKey)
		}
		cancel()
		return ports.MediaAsset{}, &appErrors.Error{
			Code: appErrors.CodeExternalDependency, Message: "conversation media metadata could not be committed",
			Retryable: true, Cause: errors.Join(err, cleanupErr, rejectErr),
		}
	}
	return ready, nil
}

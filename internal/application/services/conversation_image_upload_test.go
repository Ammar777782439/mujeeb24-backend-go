package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

const uploadTestBusinessID = "11111111-1111-4111-8111-111111111111"
const uploadTestAssetID = "22222222-2222-4222-8222-222222222222"

type conversationUploadAssetsFake struct {
	asset         ports.MediaAsset
	created       ports.PendingConversationMediaAsset
	createErr     error
	markErr       error
	rejectErr     error
	createdCount  int
	markedCount   int
	rejectedCount int
	verified      ports.VerifiedImage
	rejectStatus  string
}

func (f *conversationUploadAssetsFake) CreatePendingIngestion(_ context.Context, draft ports.PendingConversationMediaAsset) (ports.MediaAsset, error) {
	f.created, f.createdCount = draft, f.createdCount+1
	if f.createErr != nil {
		return ports.MediaAsset{}, f.createErr
	}
	f.asset = ports.MediaAsset{
		ID: draft.ID, BusinessID: draft.BusinessID, Purpose: "conversation_attachment",
		Kind: draft.Kind, StorageProvider: draft.StorageProvider, StorageKey: draft.StorageKey,
		Status: "pending_ingestion", CreatedAt: time.Now().UTC(),
	}
	return f.asset, nil
}
func (f *conversationUploadAssetsFake) GetAsset(context.Context, string, string) (ports.MediaAsset, error) {
	return f.asset, nil
}
func (f *conversationUploadAssetsFake) MarkImageReady(_ context.Context, _, _ string, verified ports.VerifiedImage) (ports.MediaAsset, error) {
	f.markedCount++
	f.verified = verified
	if f.markErr != nil {
		return ports.MediaAsset{}, f.markErr
	}
	f.asset.Status, f.asset.MimeType, f.asset.SizeBytes = "ready", verified.MimeType, verified.SizeBytes
	f.asset.SHA256, f.asset.Width, f.asset.Height = verified.SHA256, verified.Width, verified.Height
	return f.asset, nil
}
func (f *conversationUploadAssetsFake) MarkIngestionRejected(context.Context, string, string) (ports.MediaAsset, error) {
	f.rejectedCount++
	if f.rejectErr != nil {
		return ports.MediaAsset{}, f.rejectErr
	}
	if f.rejectStatus != "" {
		f.asset.Status = f.rejectStatus
	} else if f.asset.Status == "pending_ingestion" {
		f.asset.Status = "rejected"
	}
	return f.asset, nil
}

type conversationUploadObjectsFake struct {
	putProvider    string
	putKey         string
	putMime        string
	body           []byte
	putErr         error
	deleteProvider string
	deleteKey      string
	deleteErr      error
	deletes        int
}
func (f *conversationUploadObjectsFake) Put(context.Context, string, string, []byte) error {
	return errors.New("unpinned writes are forbidden")
}
func (f *conversationUploadObjectsFake) PutToProvider(_ context.Context, provider, key, mime string, data []byte) error {
	f.putProvider, f.putKey, f.putMime, f.body = provider, key, mime, append([]byte(nil), data...)
	return f.putErr
}
func (f *conversationUploadObjectsFake) DeleteFromProvider(_ context.Context, provider, key string) error {
	f.deleteProvider, f.deleteKey = provider, key
	f.deletes++
	return f.deleteErr
}
func (f *conversationUploadObjectsFake) Delete(context.Context, string) error { return nil }
func (f *conversationUploadObjectsFake) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", errors.New("not used")
}

func uploadTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Set(0, 0, color.RGBA{R: 80, G: 120, B: 160, A: 255})
	var output bytes.Buffer
	if err := png.Encode(&output, img); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func uploadTestService(assets *conversationUploadAssetsFake, objects *conversationUploadObjectsFake) ConversationImageUploadService {
	return ConversationImageUploadService{
		Assets: assets, Objects: objects, StorageProvider: "backblaze",
		NewID: func() string { return uploadTestAssetID },
	}
}

func TestConversationImageUploadPersistsVerifiedConversationAssetToSelectedProvider(t *testing.T) {
	assets := &conversationUploadAssetsFake{}
	objects := &conversationUploadObjectsFake{}
	service := uploadTestService(assets, objects)
	body := uploadTestPNG(t)
	actorID := "33333333-3333-4333-8333-333333333333"

	asset, err := service.UploadBase64(context.Background(), uploadTestBusinessID, base64.StdEncoding.EncodeToString(body), &actorID)
	if err != nil {
		t.Fatalf("UploadBase64: %v", err)
	}
	if asset.Status != "ready" || asset.Purpose != "conversation_attachment" || asset.Kind != "image" ||
		asset.StorageProvider != "backblaze" || asset.StorageKey != "conversation/"+uploadTestBusinessID+"/"+uploadTestAssetID+".png" {
		t.Fatalf("unexpected conversation media asset: %#v", asset)
	}
	if assets.createdCount != 1 || assets.markedCount != 1 || assets.rejectedCount != 0 ||
		assets.created.CreatedBy == nil || *assets.created.CreatedBy != actorID {
		t.Fatalf("asset lifecycle was not recorded: %#v", assets)
	}
	if objects.putProvider != "backblaze" || objects.putKey != asset.StorageKey ||
		objects.putMime != "image/png" || !bytes.Equal(objects.body, body) || objects.deletes != 0 {
		t.Fatalf("object was not written to the pinned private provider: %#v", objects)
	}
	if asset.SizeBytes != int64(len(body)) || asset.Width != 4 || asset.Height != 3 ||
		len(asset.SHA256) != 32 || !bytes.Equal(asset.SHA256, assets.verified.SHA256) {
		t.Fatalf("server-verified metadata is missing: %#v", asset)
	}
}

func TestConversationImageUploadRejectsInvalidPayloadBeforeCreatingAsset(t *testing.T) {
	assets := &conversationUploadAssetsFake{}
	objects := &conversationUploadObjectsFake{}
	service := uploadTestService(assets, objects)

	_, err := service.UploadBase64(context.Background(), uploadTestBusinessID, base64.StdEncoding.EncodeToString([]byte("not an image")), nil)
	var appErr *appErrors.Error
	if !errors.As(err, &appErr) || appErr.Code != appErrors.CodeValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
	if assets.createdCount != 0 || objects.putKey != "" {
		t.Fatalf("invalid bytes must not create metadata or reach storage: assets=%#v objects=%#v", assets, objects)
	}
}

func TestConversationImageUploadRejectsOversizedEncodedPayloadBeforeDecode(t *testing.T) {
	assets := &conversationUploadAssetsFake{}
	objects := &conversationUploadObjectsFake{}
	service := uploadTestService(assets, objects)
	encoded := strings.Repeat("A", base64.StdEncoding.EncodedLen(maxConversationImageBytes)+4)

	_, err := service.UploadBase64(context.Background(), uploadTestBusinessID, encoded, nil)
	var appErr *appErrors.Error
	if !errors.As(err, &appErr) || appErr.Code != appErrors.CodeValidation {
		t.Fatalf("expected bounded validation error, got %v", err)
	}
	if assets.createdCount != 0 || objects.putKey != "" {
		t.Fatal("oversized payload must be rejected before database or object-storage writes")
	}
}

func TestConversationImageUploadMarksAssetRejectedAndCleansPartialObjectOnStorageFailure(t *testing.T) {
	assets := &conversationUploadAssetsFake{}
	objects := &conversationUploadObjectsFake{putErr: errors.New("storage unavailable")}
	service := uploadTestService(assets, objects)

	_, err := service.UploadBase64(context.Background(), uploadTestBusinessID, base64.StdEncoding.EncodeToString(uploadTestPNG(t)), nil)
	var appErr *appErrors.Error
	if !errors.As(err, &appErr) || appErr.Code != appErrors.CodeExternalDependency || !appErr.Retryable {
		t.Fatalf("expected retryable storage error, got %v", err)
	}
	if assets.createdCount != 1 || assets.rejectedCount != 1 || assets.asset.Status != "rejected" ||
		objects.deletes != 1 || objects.deleteProvider != "backblaze" {
		t.Fatalf("failed upload did not converge to rejected and cleaned state: assets=%#v objects=%#v", assets, objects)
	}
}

func TestConversationImageUploadDoesNotDeleteObjectWhenAssetRejectionDidNotSucceed(t *testing.T) {
	assets := &conversationUploadAssetsFake{
		markErr:      errors.New("mark ready query failed"),
		rejectStatus: "ready",
	}
	objects := &conversationUploadObjectsFake{}
	service := uploadTestService(assets, objects)

	_, err := service.UploadBase64(context.Background(), uploadTestBusinessID, base64.StdEncoding.EncodeToString(uploadTestPNG(t)), nil)
	var appErr *appErrors.Error
	if !errors.As(err, &appErr) || appErr.Code != appErrors.CodeExternalDependency {
		t.Fatalf("expected metadata commit error, got %v", err)
	}
	if objects.deletes != 0 {
		t.Fatalf("must not delete bytes while the database reports the asset ready: deletes=%d", objects.deletes)
	}
}

var (
	_ ports.ConversationMediaIngestionRepository = (*conversationUploadAssetsFake)(nil)
	_ ports.MediaObjectStorage                   = (*conversationUploadObjectsFake)(nil)
	_ ports.MediaObjectProviderWriter            = (*conversationUploadObjectsFake)(nil)
)

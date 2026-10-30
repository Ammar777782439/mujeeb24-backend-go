package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

type uploadMediaRepository struct {
	staged     bool
	ready      bool
	failReady  bool
	lastDraft  ports.PendingMediaAsset
	lastVerify ports.VerifiedImage
}

func (m *uploadMediaRepository) CreatePendingAsset(_ context.Context, d ports.PendingMediaAsset) (ports.MediaAsset, error) {
	m.staged = true
	m.lastDraft = d
	return ports.MediaAsset{ID: d.ID, BusinessID: d.BusinessID, StorageKey: d.StorageKey, Status: "pending_upload"}, nil
}
func (m *uploadMediaRepository) GetAsset(context.Context, string, string) (ports.MediaAsset, error) {
	return ports.MediaAsset{}, nil
}
func (m *uploadMediaRepository) MarkAssetReady(_ context.Context, business, id string, verified ports.VerifiedImage) (ports.MediaAsset, error) {
	m.lastVerify = verified
	if m.failReady {
		return ports.MediaAsset{}, errors.New("database update failed")
	}
	m.ready = true
	return ports.MediaAsset{ID: id, BusinessID: business, Status: "ready", MimeType: verified.MimeType, Width: verified.Width, Height: verified.Height}, nil
}
func (m *uploadMediaRepository) LockGallery(context.Context, string, string, *string) error {
	return nil
}
func (m *uploadMediaRepository) ListGallery(context.Context, string, string, *string) ([]ports.CatalogMediaLink, error) {
	return nil, nil
}
func (m *uploadMediaRepository) Attach(context.Context, ports.CatalogMediaAttach, bool) (ports.CatalogMediaLink, error) {
	return ports.CatalogMediaLink{}, nil
}
func (m *uploadMediaRepository) SetPrimary(context.Context, string, string, string, *string, string) error {
	return nil
}
func (m *uploadMediaRepository) Remove(context.Context, string, string, string, *string, string) error {
	return nil
}
func (m *uploadMediaRepository) PrimaryForReply(context.Context, string, string, *string) (*ports.CatalogMediaLink, error) {
	return nil, nil
}
func (m *uploadMediaRepository) FindApprovedMediaLink(context.Context, string, string) (*ports.CatalogMediaLink, error) {
	return nil, nil
}

type uploadObjectStorage struct {
	putCount    int
	deleteCount int
	putKey      string
	putMime     string
	putData     []byte
	failPut     bool
}

func (o *uploadObjectStorage) Put(_ context.Context, key, mime string, data []byte) error {
	o.putCount++
	o.putKey, o.putMime, o.putData = key, mime, append([]byte(nil), data...)
	if o.failPut {
		return errors.New("R2 write failed")
	}
	return nil
}
func (o *uploadObjectStorage) Delete(context.Context, string) error {
	o.deleteCount++
	return nil
}
func (o *uploadObjectStorage) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", nil
}

func validTestPNG(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.RGBA{R: 240, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestCatalogImageUploadStagesAndVerifiesRealPixels(t *testing.T) {
	owner, assetID := uuid.NewString(), uuid.NewString()
	repo, storage := &uploadMediaRepository{}, &uploadObjectStorage{}
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	svc := CatalogImageUploadService{Repository: repo, Objects: storage, Now: func() time.Time { return now }, NewID: func() string { return assetID }}
	got, err := svc.UploadBase64(context.Background(), owner, validTestPNG(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != assetID || got.Status != "ready" || got.Width != 3 || got.Height != 2 || got.MimeType != "image/png" {
		t.Fatalf("image must be verified before ready: %+v", got)
	}
	if !repo.staged || !repo.ready || repo.lastVerify.SizeBytes == 0 || len(repo.lastVerify.SHA256) != 32 {
		t.Fatalf("upload did not verify file: %+v", repo.lastVerify)
	}
	if storage.putCount != 1 || storage.deleteCount != 0 || storage.putMime != "image/png" ||
		storage.putKey != "catalog/"+owner+"/"+assetID+".png" || !repo.lastDraft.UploadExpiresAt.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("unexpected R2 upload / staging: %+v", storage)
	}
}

func TestCatalogImageUploadRejectsCorruptedImageBeforeStorage(t *testing.T) {
	repo, storage := &uploadMediaRepository{}, &uploadObjectStorage{}
	svc := CatalogImageUploadService{Repository: repo, Objects: storage}
	// Has an image/png header but is not decodable.
	broken := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\n" + "corrupt"))
	if _, err := svc.UploadBase64(context.Background(), uuid.NewString(), broken, nil); err == nil {
		t.Fatal("corrupted image accepted")
	}
	if repo.staged || storage.putCount != 0 {
		t.Fatal("corrupted image must never be staged or written")
	}
}

func TestCatalogImageUploadRejectsMissingOrOversizedPayload(t *testing.T) {
	repo, storage := &uploadMediaRepository{}, &uploadObjectStorage{}
	svc := CatalogImageUploadService{Repository: repo, Objects: storage}
	for _, b64 := range []string{"", "not-base64!!!!", base64.StdEncoding.EncodeToString([]byte("not an image"))} {
		if _, err := svc.UploadBase64(context.Background(), uuid.NewString(), b64, nil); err == nil {
			t.Fatalf("invalid payload accepted: %q", b64)
		}
	}
	if repo.staged || storage.putCount != 0 {
		t.Fatal("invalid image reached persistence")
	}
}

func TestCatalogImageUploadR2FailureNeverMarksReady(t *testing.T) {
	repo, storage := &uploadMediaRepository{}, &uploadObjectStorage{failPut: true}
	svc := CatalogImageUploadService{Repository: repo, Objects: storage}
	_, err := svc.UploadBase64(context.Background(), uuid.NewString(), validTestPNG(t), nil)
	if err == nil || !repo.staged || repo.ready {
		t.Fatal("failed R2 PUT must not mark an asset ready")
	}
}

func TestCatalogImageUploadDBFailureAttemptsR2Cleanup(t *testing.T) {
	repo, storage := &uploadMediaRepository{failReady: true}, &uploadObjectStorage{}
	svc := CatalogImageUploadService{Repository: repo, Objects: storage}
	_, err := svc.UploadBase64(context.Background(), uuid.NewString(), validTestPNG(t), nil)
	if err == nil || repo.ready || storage.deleteCount != 1 {
		t.Fatalf("failed media commit must attempt R2 cleanup; err=%v delete=%d", err, storage.deleteCount)
	}
}

func TestCatalogImageUploadRecordsSelectedBackblazeProvider(t *testing.T) {
	repo := &uploadMediaRepository{}
	store := &uploadObjectStorage{}
	svc := CatalogImageUploadService{
		Repository: repo, Objects: store, StorageProvider: "backblaze",
	}
	if _, err := svc.UploadBase64(context.Background(), uuid.NewString(), validTestPNG(t), nil); err != nil {
		t.Fatal(err)
	}
	if repo.lastDraft.StorageProvider != "backblaze" || store.putCount != 1 {
		t.Fatalf("B2 provider was not persisted: %+v puts=%d", repo.lastDraft, store.putCount)
	}
}

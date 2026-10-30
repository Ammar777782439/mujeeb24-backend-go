package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type mediaTestTx struct {
	calls int
	fail  bool
}

func (t *mediaTestTx) Within(ctx context.Context, fn func(context.Context) error) error {
	t.calls++
	if t.fail {
		return errors.New("transaction unavailable")
	}
	return fn(ctx)
}

type mediaTestRepo struct {
	gallery     []ports.CatalogMediaLink
	locked      int
	failLock    bool
	attached    bool
	lastPrimary bool
	selected    string
	removed     string
}

func (m *mediaTestRepo) CreatePendingAsset(context.Context, ports.PendingMediaAsset) (ports.MediaAsset, error) {
	return ports.MediaAsset{}, nil
}
func (m *mediaTestRepo) GetAsset(context.Context, string, string) (ports.MediaAsset, error) {
	return ports.MediaAsset{}, nil
}
func (m *mediaTestRepo) MarkAssetReady(context.Context, string, string, ports.VerifiedImage) (ports.MediaAsset, error) {
	return ports.MediaAsset{}, nil
}
func (m *mediaTestRepo) LockGallery(_ context.Context, _, _ string, _ *string) error {
	m.locked++
	if m.failLock {
		return errors.New("wrong gallery")
	}
	return nil
}
func (m *mediaTestRepo) ListGallery(context.Context, string, string, *string) ([]ports.CatalogMediaLink, error) {
	return m.gallery, nil
}
func (m *mediaTestRepo) Attach(_ context.Context, d ports.CatalogMediaAttach, primary bool) (ports.CatalogMediaLink, error) {
	m.attached = true
	m.lastPrimary = primary
	return ports.CatalogMediaLink{ID: d.ID, BusinessID: d.BusinessID, CatalogItemID: d.CatalogItemID,
		VariantID: d.VariantID, MediaID: d.MediaID, IsPrimary: primary}, nil
}
func (m *mediaTestRepo) SetPrimary(_ context.Context, _, _, linkID string, _ *string, _ string) error {
	m.selected = linkID
	return nil
}
func (m *mediaTestRepo) Remove(_ context.Context, _, _, linkID string, _ *string, _ string) error {
	m.removed = linkID
	return nil
}
func (m *mediaTestRepo) PrimaryForReply(context.Context, string, string, *string) (*ports.CatalogMediaLink, error) {
	return nil, nil
}

func TestCatalogMediaAttachFirstImageBecomesPrimary(t *testing.T) {
	repo := &mediaTestRepo{}
	tx := &mediaTestTx{}
	service := CatalogMediaService{Media: repo, Transactions: tx, NewID: func() string { return "link-1" },
		Now: func() time.Time { return time.Unix(1700000000, 0) }}
	got, err := service.Attach(context.Background(), "business", "item", nil, "media", false, 0, "صورة المنتج", nil)
	if err != nil || !got.IsPrimary || got.ID != "link-1" || repo.locked != 1 || tx.calls != 1 {
		t.Fatalf("first media link must be primary and serialized: got=%+v err=%v", got, err)
	}
}

func TestCatalogMediaAttachAdditionalImageIsNotPrimaryByDefault(t *testing.T) {
	repo := &mediaTestRepo{gallery: []ports.CatalogMediaLink{{ID: "already", IsPrimary: true}}}
	service := CatalogMediaService{Media: repo, Transactions: &mediaTestTx{}}
	got, err := service.Attach(context.Background(), "business", "item", nil, "media", false, 1, "", nil)
	if err != nil || got.IsPrimary {
		t.Fatalf("additional image must not take primary: %+v %v", got, err)
	}
}

func TestCatalogMediaAttachDoesNotWriteAfterFailedLock(t *testing.T) {
	repo := &mediaTestRepo{failLock: true}
	service := CatalogMediaService{Media: repo, Transactions: &mediaTestTx{}}
	_, err := service.Attach(context.Background(), "business", "item", nil, "media", true, 0, "", nil)
	if err == nil || repo.attached {
		t.Fatal("lock failure must prevent write")
	}
}

func TestCatalogMediaMutationsRequireTransaction(t *testing.T) {
	repo := &mediaTestRepo{}
	tx := &mediaTestTx{fail: true}
	service := CatalogMediaService{Media: repo, Transactions: tx}
	if err := service.SetPrimary(context.Background(), "business", "item", nil, "link"); err == nil {
		t.Fatal("expected transactional error")
	}
	if err := service.Remove(context.Background(), "business", "item", nil, "link"); err == nil {
		t.Fatal("expected transactional error")
	}
	if repo.locked != 0 || repo.selected != "" || repo.removed != "" {
		t.Fatal("operation ran outside transaction")
	}
}

func TestCatalogMediaRejectsInvalidLinkBeforeTransaction(t *testing.T) {
	tx := &mediaTestTx{}
	service := CatalogMediaService{Media: &mediaTestRepo{}, Transactions: tx}
	_, err := service.Attach(context.Background(), "business", "item", nil, "media", true, -1, "", nil)
	if err == nil || tx.calls != 0 {
		t.Fatal("invalid request opened transaction")
	}
}

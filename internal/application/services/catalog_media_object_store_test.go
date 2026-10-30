package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type namedObjectStore struct {
	name  string
	puts  int
	reads int
}

func (s *namedObjectStore) Put(_ context.Context, _, _ string, _ []byte) error {
	s.puts++
	return nil
}
func (s *namedObjectStore) Delete(context.Context, string) error { return nil }
func (s *namedObjectStore) PresignGet(_ context.Context, _, _ time.Duration) (string, error) {
	s.reads++
	return "https://" + s.name + ".example.test/asset", nil
}

func TestCatalogStorageReadsAssetFromRecordedProvider(t *testing.T) {
	r2 := &namedObjectStore{name: "r2"}
	b2 := &namedObjectStore{name: "backblaze"}
	storage := &CatalogMediaObjectStore{WriteProvider: "backblaze", Providers: map[string]ports.MediaObjectStorage{"r2": r2, "backblaze": b2}}
	if err := storage.Put(context.Background(), "catalog/biz/new", "image/png", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if b2.puts != 1 || r2.puts != 0 {
		t.Fatal("new uploads must use selected B2")
	}
	oldURL, err := PresignCatalogAsset(context.Background(), storage, ports.MediaAsset{StorageProvider: "r2", StorageKey: "catalog/biz/old"}, time.Minute)
	if err != nil || !strings.Contains(oldURL, "r2.example.test") || r2.reads != 1 {
		t.Fatalf("old R2 object misrouted: %q %v", oldURL, err)
	}
	newURL, err := PresignCatalogAsset(context.Background(), storage, ports.MediaAsset{StorageProvider: "backblaze", StorageKey: "catalog/biz/new"}, time.Minute)
	if err != nil || !strings.Contains(newURL, "backblaze.example.test") || b2.reads != 1 {
		t.Fatalf("B2 object misrouted: %q %v", newURL, err)
	}
	if _, err := PresignCatalogAsset(context.Background(), storage, ports.MediaAsset{StorageProvider: "unknown", StorageKey: "catalog/biz/old"}, time.Minute); err == nil {
		t.Fatal("unknown storage provider should fail closed")
	}
}

func TestCatalogStorageDoesNotFallBackToWrongBucket(t *testing.T) {
	storage := &CatalogMediaObjectStore{WriteProvider: "backblaze", Providers: map[string]ports.MediaObjectStorage{"backblaze": &namedObjectStore{name: "backblaze"}}}
	_, err := PresignCatalogAsset(context.Background(), storage, ports.MediaAsset{StorageProvider: "r2", StorageKey: "catalog/biz/legacy"}, time.Minute)
	if err == nil {
		t.Fatal("missing R2 credentials must not return a B2 URL")
	}
}

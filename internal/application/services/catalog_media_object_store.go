package services

import (
	"context"
	"errors"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// CatalogMediaObjectStore chooses the storage for writes and the original
// provider for every read. Switching the upload provider never migrates bytes.
type CatalogMediaObjectStore struct {
	WriteProvider string
	Providers     map[string]ports.MediaObjectStorage
}

func (s *CatalogMediaObjectStore) provider(name string) (ports.MediaObjectStorage, error) {
	if s == nil || name == "" {
		return nil, errors.New("media storage provider is missing")
	}
	p := s.Providers[name]
	if p == nil {
		return nil, errors.New("recorded media storage provider is unavailable")
	}
	return p, nil
}

func (s *CatalogMediaObjectStore) Put(ctx context.Context, key, mime string, body []byte) error {
	p, err := s.provider(s.WriteProvider)
	if err != nil {
		return err
	}
	return p.Put(ctx, key, mime, body)
}
func (s *CatalogMediaObjectStore) Delete(ctx context.Context, key string) error {
	p, err := s.provider(s.WriteProvider)
	if err != nil {
		return err
	}
	return p.Delete(ctx, key)
}
func (s *CatalogMediaObjectStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return s.PresignFromProvider(ctx, s.WriteProvider, key, ttl)
}
func (s *CatalogMediaObjectStore) PresignFromProvider(ctx context.Context, provider, key string, ttl time.Duration) (string, error) {
	p, err := s.provider(provider)
	if err != nil {
		return "", err
	}
	return p.PresignGet(ctx, key, ttl)
}

// GetFromProvider fetches bytes from the *recorded* storage provider only.
// It never falls back to today's write provider for historical images.
func (s *CatalogMediaObjectStore) GetFromProvider(ctx context.Context, provider, key string) ([]byte, error) {
	storage, err := s.provider(provider)
	if err != nil {
		return nil, err
	}
	reader, ok := storage.(ports.MediaObjectReader)
	if !ok {
		return nil, errors.New("private media reader is not configured")
	}
	return reader.Get(ctx, key)
}

// ReadCatalogAsset uses direct, SDK-authenticated S3 reads for dashboard
// previews rather than fetching a short-lived presigned URL over HTTP.
func ReadCatalogAsset(ctx context.Context, objects ports.MediaObjectStorage, asset ports.MediaAsset) ([]byte, error) {
	if objects == nil {
		return nil, errors.New("catalog object storage not configured")
	}
	if routed, ok := objects.(interface {
		GetFromProvider(context.Context, string, string) ([]byte, error)
	}); ok {
		return routed.GetFromProvider(ctx, asset.StorageProvider, asset.StorageKey)
	}
	if asset.StorageProvider != "r2" {
		return nil, errors.New("no private media reader for recorded provider")
	}
	reader, ok := objects.(ports.MediaObjectReader)
	if !ok {
		return nil, errors.New("private media reader is not configured")
	}
	return reader.Get(ctx, asset.StorageKey)
}

// PresignCatalogAsset never guesses a source from the key or the configured
// default. Historical R2 objects remain R2 even after switching new uploads.
func PresignCatalogAsset(ctx context.Context, objects ports.MediaObjectStorage, asset ports.MediaAsset, ttl time.Duration) (string, error) {
	if objects == nil {
		return "", errors.New("catalog object storage not configured")
	}
	if routed, ok := objects.(interface {
		PresignFromProvider(context.Context, string, string, time.Duration) (string, error)
	}); ok {
		return routed.PresignFromProvider(ctx, asset.StorageProvider, asset.StorageKey, ttl)
	}
	// Legacy non-routed deployments can only read assets tagged as R2.
	if asset.StorageProvider != "r2" {
		return "", errors.New("media asset storage provider has no configured reader")
	}
	return objects.PresignGet(ctx, asset.StorageKey, ttl)
}

var _ ports.MediaObjectStorage = (*CatalogMediaObjectStore)(nil)

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

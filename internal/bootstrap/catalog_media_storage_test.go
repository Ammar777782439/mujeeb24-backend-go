package bootstrap

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
)

func TestCatalogMediaStorageCanWriteB2AndReadHistoricalR2(t *testing.T) {
	t.Setenv("CATALOG_MEDIA_ENABLED", "true")
	t.Setenv("S3_PROVIDER", "backblaze")
	t.Setenv("S3_ENDPOINT", "https://s3.us-west-004.backblazeb2.com")
	t.Setenv("S3_REGION", "us-west-004")
	t.Setenv("S3_BUCKET", "mujeeb24-images")
	t.Setenv("S3_ACCESS_KEY_ID", "test-id")
	t.Setenv("S3_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("R2_ACCOUNT_ID", "test-account")
	t.Setenv("R2_BUCKET", "mujeeb-old")
	t.Setenv("R2_ACCESS_KEY_ID", "test-r2-id")
	t.Setenv("R2_SECRET_ACCESS_KEY", "test-r2-secret")
	objects, provider, err := buildCatalogMediaStorageFromEnv()
	if err != nil || provider != "backblaze" {
		t.Fatalf("storage config: provider=%s err=%v", provider, err)
	}
	router, ok := objects.(*services.CatalogMediaObjectStore)
	if !ok {
		t.Fatal("expected provider router")
	}
	r2URL, err := router.PresignFromProvider(context.Background(), "r2", "catalog/biz/legacy.jpg", time.Minute)
	if err != nil || !strings.Contains(r2URL, "r2.cloudflarestorage.com") {
		t.Fatalf("legacy R2 URL missing: %v", err)
	}
	b2URL, err := router.PresignFromProvider(context.Background(), "backblaze", "catalog/biz/new.jpg", time.Minute)
	if err != nil || !strings.Contains(b2URL, "s3.us-west-004.backblazeb2.com") {
		t.Fatalf("new B2 URL missing: %v", err)
	}
	t.Setenv("S3_PROVIDER", "r2")
	_, provider, err = buildCatalogMediaStorageFromEnv()
	if err != nil || provider != "r2" {
		t.Fatalf("legacy default broken: %s %v", provider, err)
	}
}
func TestCatalogMediaStorageRejectsMissingOrUnsafeB2Config(t *testing.T) {
	t.Setenv("CATALOG_MEDIA_ENABLED", "true")
	t.Setenv("S3_PROVIDER", "backblaze")
	t.Setenv("S3_ENDPOINT", "http://127.0.0.1:9000")
	t.Setenv("S3_REGION", "us-west-004")
	t.Setenv("S3_BUCKET", "catalog")
	t.Setenv("S3_ACCESS_KEY_ID", "key")
	t.Setenv("S3_SECRET_ACCESS_KEY", "secret")
	for _, key := range []string{"R2_ACCOUNT_ID", "R2_BUCKET", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY"} {
		t.Setenv(key, "")
	}
	if _, _, err := buildCatalogMediaStorageFromEnv(); err == nil {
		t.Fatal("unsafe endpoint accepted")
	}
	t.Setenv("S3_ENDPOINT", "https://s3.us-west-004.backblazeb2.com")
	t.Setenv("S3_SECRET_ACCESS_KEY", "")
	if _, _, err := buildCatalogMediaStorageFromEnv(); err == nil {
		t.Fatal("missing secret accepted")
	}
}

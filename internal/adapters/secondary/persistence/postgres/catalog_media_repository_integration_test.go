//go:build integration

package postgres

import (
	"context"
	"crypto/sha256"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
)

func TestCatalogMediaTenantAndPrimaryInPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a, b := uuid.NewString(), uuid.NewString()
	catA, catB := uuid.NewString(), uuid.NewString()
	itemA, itemB := uuid.NewString(), uuid.NewString()
	variantB := uuid.NewString()
	assetA, assetB := uuid.NewString(), uuid.NewString()
	linkA, linkA2 := uuid.NewString(), uuid.NewString()
	cleanup := func() {
		c := context.Background()
		_, _ = db.Pool().Exec(c, `DELETE FROM catalog_media_links WHERE business_id IN ($1::uuid,$2::uuid)`, a, b)
		_, _ = db.Pool().Exec(c, `DELETE FROM media_assets WHERE business_id IN ($1::uuid,$2::uuid)`, a, b)
		_, _ = db.Pool().Exec(c, `DELETE FROM variants WHERE id=$1::uuid`, variantB)
		_, _ = db.Pool().Exec(c, `DELETE FROM catalog_items WHERE id IN ($1::uuid,$2::uuid)`, itemA, itemB)
		_, _ = db.Pool().Exec(c, `DELETE FROM catalogs WHERE id IN ($1::uuid,$2::uuid)`, catA, catB)
		_, _ = db.Pool().Exec(c, `DELETE FROM businesses WHERE id IN ($1::uuid,$2::uuid)`, a, b)
	}
	defer cleanup()
	for _, id := range []string{a, b} {
		_, err = db.Pool().Exec(ctx, `INSERT INTO businesses
         (id,name,slug,status,vertical_type,timezone,default_currency,locale,created_at,updated_at)
         VALUES ($1::uuid,'Media Integration','media-it-'||$2,'active','retail','Asia/Aden','YER','ar-YE',now(),now())`, id, id)
		if err != nil {
			t.Fatalf("business: %v", err)
		}
	}
	for i, id := range []string{catA, catB} {
		owner := []string{a, b}[i]
		_, err = db.Pool().Exec(ctx, `INSERT INTO catalogs(id,business_id,name,status,created_at,updated_at)
         VALUES($1::uuid,$2::uuid,'Photos','active',now(),now())`, id, owner)
		if err != nil {
			t.Fatalf("catalog: %v", err)
		}
	}
	for i, id := range []string{itemA, itemB} {
		owner := []string{a, b}[i]
		catalog := []string{catA, catB}[i]
		_, err = db.Pool().Exec(ctx, `INSERT INTO catalog_items
         (id,business_id,catalog_id,item_type,name,status,pricing_mode,availability_mode,fulfillment_mode,requires_confirmation,created_at,updated_at)
         VALUES($1::uuid,$2::uuid,$3::uuid,'product','Jacket','active','fixed','unknown','manual',false,now(),now())`,
			id, owner, catalog)
		if err != nil {
			t.Fatalf("item: %v", err)
		}
	}
	_, err = db.Pool().Exec(ctx, `INSERT INTO variants(id,business_id,catalog_item_id,name,attributes,status,created_at,updated_at)
      VALUES($1::uuid,$2::uuid,$3::uuid,'brown','{}'::jsonb,'active',now(),now())`, variantB, b, itemB)
	if err != nil {
		t.Fatalf("variant: %v", err)
	}
	repo := NewCatalogMediaRepository(db)
	for _, draft := range []ports.PendingMediaAsset{
		{ID: assetA, BusinessID: a, StorageKey: "catalog/" + a + "/" + assetA, UploadExpiresAt: time.Now().Add(time.Hour)},
		{ID: assetB, BusinessID: b, StorageKey: "catalog/" + b + "/" + assetB, UploadExpiresAt: time.Now().Add(time.Hour)},
	} {
		if _, err := repo.CreatePendingAsset(ctx, draft); err != nil {
			t.Fatalf("stage: %v", err)
		}
		digest := sha256.Sum256([]byte("example"))
		if _, err := repo.MarkAssetReady(ctx, draft.BusinessID, draft.ID, ports.VerifiedImage{
			MimeType: "image/png", SizeBytes: 123, SHA256: digest[:], Width: 10, Height: 10,
		}); err != nil {
			t.Fatalf("ready: %v", err)
		}
	}
	// No business can link another business's asset, even bypassing the service.
	_, err = db.Pool().Exec(ctx, `INSERT INTO catalog_media_links
        (id,business_id,catalog_item_id,media_id,is_primary,approved_at)
        VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,true,now())`,
		uuid.NewString(), a, itemA, assetB)
	if err == nil {
		t.Fatal("cross-tenant asset FK did not reject")
	}
	// Variant belongs to B's item, not A's item.
	_, err = db.Pool().Exec(ctx, `INSERT INTO catalog_media_links
        (id,business_id,catalog_item_id,variant_id,media_id,is_primary,approved_at)
        VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,true,now())`,
		uuid.NewString(), a, itemA, variantB, assetA)
	if err == nil {
		t.Fatal("cross-item variant FK did not reject")
	}
	// Empty gallery gets a primary even if caller asks for nonprimary.
	svc := services.CatalogMediaService{Media: repo, Transactions: db, NewID: func() string { return linkA }}
	first, err := svc.Attach(ctx, a, itemA, nil, assetA, false, 0, "Brown jacket", nil)
	if err != nil || !first.IsPrimary {
		t.Fatalf("first primary: %+v %v", first, err)
	}
	primary, err := repo.PrimaryForReply(ctx, a, itemA, nil)
	if err != nil || primary == nil || primary.MediaID != assetA {
		t.Fatalf("primary resolver: %+v %v", primary, err)
	}
	// Underlying partial unique index rejects a second primary.
	_, err = db.Pool().Exec(ctx, `INSERT INTO catalog_media_links
        (id,business_id,catalog_item_id,media_id,is_primary,approved_at)
        VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,true,now())`,
		linkA2, a, itemA, assetA)
	if err == nil {
		t.Fatal("duplicate active primary/asset was accepted")
	}
	if err := svc.Remove(ctx, a, itemA, nil, linkA); err != nil {
		t.Fatalf("remove: %v", err)
	}
	primary, err = repo.PrimaryForReply(ctx, a, itemA, nil)
	if err != nil || primary != nil {
		t.Fatalf("removed image should not resolve: %+v %v", primary, err)
	}
}

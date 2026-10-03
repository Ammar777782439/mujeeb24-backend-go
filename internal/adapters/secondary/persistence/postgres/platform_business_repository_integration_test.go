//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
)

func TestPlatformBusinessRepositoryProjectsActiveOwner(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open adapter: %v", err)
	}
	defer adapter.Close()

	const businessID = "00000000-0000-0000-0000-000000000701"
	const principalID = "00000000-0000-0000-0000-000000000702"
	_, _ = adapter.Pool().Exec(ctx, "DELETE FROM business_memberships WHERE business_id = $1::uuid", businessID)
	_, _ = adapter.Pool().Exec(ctx, "DELETE FROM principals WHERE id = $1::uuid", principalID)
	_, _ = adapter.Pool().Exec(ctx, "DELETE FROM businesses WHERE id = $1::uuid", businessID)
	defer func() {
		_, _ = adapter.Pool().Exec(context.Background(), "DELETE FROM business_memberships WHERE business_id = $1::uuid", businessID)
		_, _ = adapter.Pool().Exec(context.Background(), "DELETE FROM principals WHERE id = $1::uuid", principalID)
		_, _ = adapter.Pool().Exec(context.Background(), "DELETE FROM businesses WHERE id = $1::uuid", businessID)
	}()

	now := time.Now().UTC()
	if _, err := adapter.Pool().Exec(ctx, `
		INSERT INTO businesses
			(id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at)
		VALUES
			($1::uuid, $2, $3, 'pending_setup', 'retail', 'Asia/Aden', 'YER', 'ar-YE', $4, $4)`,
		businessID, "Platform Owner Projection Shop", "platform-owner-projection-shop", now,
	); err != nil {
		t.Fatalf("insert business: %v", err)
	}

	if _, err := adapter.Pool().Exec(ctx, `
		INSERT INTO principals
			(id, email, display_name, password_hash, status, created_at, updated_at)
		VALUES
			($1::uuid, $2, $3, $4, 'active', $5, $5)`,
		principalID, "owner.platform@example.test", "Platform Owner", "$2a$10$owner-placeholder", now,
	); err != nil {
		t.Fatalf("insert principal: %v", err)
	}

	repo := NewPlatformBusinessRepository(adapter)
	before, err := repo.GetByID(ctx, businessID)
	if err != nil {
		t.Fatalf("get pending business: %v", err)
	}
	if before.PlatformStatus != "pending_setup" {
		t.Fatalf("status = %q, want pending_setup", before.PlatformStatus)
	}
	if before.OwnerIdentitySummary != nil {
		t.Fatalf("owner summary before membership = %v, want nil", *before.OwnerIdentitySummary)
	}

	if _, err := adapter.Pool().Exec(ctx, `
		INSERT INTO business_memberships
			(business_id, principal_id, role, permissions, status, created_at, updated_at)
		VALUES
			($1::uuid, $2::uuid, 'owner', '["*"]'::jsonb, 'active', $3, $3)`,
		businessID, principalID, now,
	); err != nil {
		t.Fatalf("insert owner membership: %v", err)
	}

	withOwner, err := repo.GetByID(ctx, businessID)
	if err != nil {
		t.Fatalf("get business with owner: %v", err)
	}
	if withOwner.OwnerIdentitySummary == nil || *withOwner.OwnerIdentitySummary != "Platform Owner <owner.platform@example.test>" {
		t.Fatalf("owner summary = %v, want expected identity", withOwner.OwnerIdentitySummary)
	}
	page, err := repo.List(ctx, ports.PlatformBusinessListFilter{Status: "PENDING_SETUP", Limit: 10})
	if err != nil {
		t.Fatalf("list pending businesses: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != businessID {
		t.Fatalf("pending status filter did not match business: %#v", page.Items)
	}

}

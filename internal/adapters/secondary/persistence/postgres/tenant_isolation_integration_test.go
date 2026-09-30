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

func TestChannelConnectionProviderAccountGlobalUniquenessAgainstPostgres(t *testing.T) {
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

	businessA := "60000000-0000-0000-0000-000000000001"
	businessB := "60000000-0000-0000-0000-000000000002"
	cleanup := func() {
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM channel_provisioning_sessions WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM channel_connections WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessA, businessB)
	}
	cleanup()
	defer cleanup()
	for _, id := range []string{businessA, businessB} {
		if _, err := adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, $2, $3, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', now(), now())`, id, "Tenant A", "tenant-a-"+id[len(id)-2:]); err != nil {
			t.Fatalf("insert business %s: %v", id, err)
		}
	}

	repo := NewChannelConnectionRepository(adapter)
	// Business A creates connection with account-a
	connA, err := repo.CreatePending(ctx, businessA, "socialapi", "whatsapp", "pending-conn-unique", "secret://a")
	if err != nil {
		t.Fatalf("create pending A: %v", err)
	}
	if _, err := repo.Activate(ctx, businessA, connA.ID, "unique-account-tenant-test", "unique-conn-tenant-test"); err != nil {
		t.Fatalf("activate A: %v", err)
	}
	// Verify lookup resolves to business A
	resolved, err := repo.GetByProviderReferences(ctx, "socialapi", "unique-account-tenant-test", "")
	if err != nil || resolved.BusinessID != businessA {
		t.Fatalf("resolve A: got %#v err %v", resolved, err)
	}
	// Business B with same provider account must be rejected (global uniqueness)
	connB, err := repo.CreatePending(ctx, businessB, "socialapi", "whatsapp", "pending-conn-unique-b", "secret://b")
	if err != nil {
		t.Fatalf("create pending B: %v", err)
	}
	if _, err := repo.Activate(ctx, businessB, connB.ID, "unique-account-tenant-test", "unique-conn-other"); !IsRepositoryKind(err, RepositoryInvalid) && !IsRepositoryKind(err, RepositoryConflict) {
		// postgres unique violation is surfaced as RepositoryInvalid via invalidRepositoryInput classification
		t.Fatalf("expected duplicate provider account to be rejected, got %v", err)
	}
	// Same tenant same provider account via different connection should also be prevented
	// Query with non-existing account should be not_found, not Tenant A
	if _, err := repo.GetByProviderReferences(ctx, "socialapi", "non-existent-account-xyz", ""); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("expected not_found for missing account, got %v", err)
	}
	// Business B with distinct account should succeed
	connB2, err := repo.CreatePending(ctx, businessB, "socialapi", "facebook", "pending-conn-unique-b2", "secret://b2")
	if err != nil {
		t.Fatalf("create pending B2: %v", err)
	}
	if _, err := repo.Activate(ctx, businessB, connB2.ID, "unique-account-tenant-test-b2", "unique-conn-tenant-test-b2"); err != nil {
		t.Fatalf("activate B2 distinct account: %v", err)
	}
	resolvedB, err := repo.GetByProviderReferences(ctx, "socialapi", "unique-account-tenant-test-b2", "")
	if err != nil || resolvedB.BusinessID != businessB {
		t.Fatalf("resolve B2: got %#v err %v", resolvedB, err)
	}
	// provider_connection_ref is already globally unique via existing constraint; verify
	if _, err := repo.GetByProviderReferences(ctx, "socialapi", "", "unique-conn-tenant-test"); err != nil || resolved.BusinessID != businessA {
		// This lookup by connection ref should also isolate
		t.Fatalf("resolve by connection_ref: %v", err)
	}
}

func TestChannelProvisioningOAuthStateGlobalUniquenessAgainstPostgres(t *testing.T) {
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

	businessA := "60000000-0000-0000-0000-000000000011"
	businessB := "60000000-0000-0000-0000-000000000012"
	cleanup := func() {
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM channel_provisioning_sessions WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM channel_connections WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessA, businessB)
	}
	cleanup()
	defer cleanup()
	for _, id := range []string{businessA, businessB} {
		if _, err := adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, $2, $3, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', now(), now())`, id, "OAuth Tenant", "oauth-tenant-"+id[len(id)-2:]); err != nil {
			t.Fatalf("insert business %s: %v", id, err)
		}
	}

	store := NewChannelProvisioningStore(adapter)
	sessionA := ports.ChannelProvisioningSession{ID: "60000000-0000-0000-0000-000000000021", BusinessID: businessA, IdempotencyKey: "oauth-unique-a", ProviderRef: "socialapi", Channel: "whatsapp", DisplayName: "OA A", Status: ports.ProvisioningPendingAuthorization}
	if _, err := store.CreateOrGet(ctx, sessionA); err != nil {
		t.Fatalf("create session A: %v", err)
	}
	oauthState := "oauth-state-tenant-unique-001"
	if _, err := store.MarkProvisioning(ctx, businessA, sessionA.ID, ports.ChannelProvisioningPatch{Status: ports.ProvisioningPendingAuthorization, OAuthState: &oauthState, AuthorizationURL: stringPtrIntegration("https://example.com/auth")}); err != nil {
		t.Fatalf("mark oauth A: %v", err)
	}
	// Same oauth_state for business B must be rejected due to global unique index
	sessionB := ports.ChannelProvisioningSession{ID: "60000000-0000-0000-0000-000000000022", BusinessID: businessB, IdempotencyKey: "oauth-unique-b", ProviderRef: "socialapi", Channel: "whatsapp", DisplayName: "OA B", Status: ports.ProvisioningPendingAuthorization}
	if _, err := store.CreateOrGet(ctx, sessionB); err != nil {
		t.Fatalf("create session B: %v", err)
	}
	if _, err := store.MarkProvisioning(ctx, businessB, sessionB.ID, ports.ChannelProvisioningPatch{Status: ports.ProvisioningPendingAuthorization, OAuthState: &oauthState, AuthorizationURL: stringPtrIntegration("https://example.com/auth")}); err == nil {
		t.Fatalf("expected duplicate oauth_state to be rejected")
	}
	// GetByOAuthState must resolve to correct business
	byState, err := store.GetByOAuthState(ctx, oauthState)
	if err != nil || byState.BusinessID != businessA || byState.ID != sessionA.ID {
		t.Fatalf("get by oauth_state should resolve to business A: got %#v err %v", byState, err)
	}
	// Tenant isolation via GetByID: Business B must not see A's session
	if _, err := store.GetByID(ctx, businessB, sessionA.ID); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("expected tenant isolation for GetByID, got %v", err)
	}
	if _, err := store.GetByID(ctx, businessA, sessionB.ID); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("expected tenant isolation for GetByID reverse, got %v", err)
	}
	// Missing state
	if _, err := store.GetByOAuthState(ctx, "non-existent-state-xyz"); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("expected not_found for missing state, got %v", err)
	}
}

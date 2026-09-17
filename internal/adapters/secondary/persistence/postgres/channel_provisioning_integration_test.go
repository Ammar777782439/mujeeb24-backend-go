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

func TestChannelProvisioningRepositoriesAgainstPostgres(t *testing.T) {
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

	businessID := "00000000-0000-0000-0000-000000000037"
	otherBusinessID := "00000000-0000-0000-0000-000000000038"
	cleanup := func() {
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM channel_provisioning_sessions WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM channel_connections WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
	}
	cleanup()
	defer cleanup()
	for _, id := range []string{businessID, otherBusinessID} {
		if _, err := adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, $2, $3, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', now(), now())`, id, "Provisioning Shop", "provisioning-"+id[len(id)-2:]); err != nil {
			t.Fatalf("insert business %s: %v", id, err)
		}
	}

	store := NewChannelProvisioningStore(adapter)
	session := ports.ChannelProvisioningSession{ID: "00000000-0000-0000-0000-000000000137", BusinessID: businessID, IdempotencyKey: "connect-1", ProviderRef: "socialapi", Channel: "facebook", DisplayName: "Provisioning Shop", Status: ports.ProvisioningPendingAuthorization}
	created, err := store.CreateOrGet(ctx, session)
	if err != nil || created.ID != session.ID || created.Status != ports.ProvisioningPendingAuthorization {
		t.Fatalf("create session=%#v err=%v", created, err)
	}
	second, err := store.CreateOrGet(ctx, ports.ChannelProvisioningSession{ID: "00000000-0000-0000-0000-000000000237", BusinessID: businessID, IdempotencyKey: "connect-1", ProviderRef: "socialapi", Channel: "facebook", DisplayName: "Other", Status: ports.ProvisioningPendingAuthorization})
	if err != nil || second.ID != session.ID || second.DisplayName != session.DisplayName {
		t.Fatalf("idempotent session=%#v err=%v", second, err)
	}
	state, authURL := "state-1", "https://social.example/auth"
	updated, err := store.MarkProvisioning(ctx, businessID, session.ID, ports.ChannelProvisioningPatch{Status: ports.ProvisioningPendingAuthorization, OAuthState: &state, AuthorizationURL: &authURL})
	if err != nil || updated.OAuthState != state || updated.AuthorizationURL != authURL {
		t.Fatalf("mark session=%#v err=%v", updated, err)
	}

	// Test superseding previous pending session with a new idempotency key
	newSession := ports.ChannelProvisioningSession{ID: "00000000-0000-0000-0000-000000000337", BusinessID: businessID, IdempotencyKey: "connect-2-new", ProviderRef: "socialapi", Channel: "facebook", DisplayName: "Provisioning Shop New", Status: ports.ProvisioningPendingAuthorization}
	supersededOld, err := store.CreateOrGet(ctx, newSession)
	if err != nil || supersededOld.ID != newSession.ID {
		t.Fatalf("create new session after previous: %#v err=%v", supersededOld, err)
	}
	oldCheck, err := store.GetByID(ctx, businessID, session.ID)
	if err != nil || oldCheck.Status != ports.ProvisioningFailed || oldCheck.FailureCode != ports.FailureCodeSuperseded {
		t.Fatalf("expected old session to be superseded, got %#v err=%v", oldCheck, err)
	}
	byState, err := store.GetByOAuthState(ctx, state)
	if err != nil || byState.ID != session.ID || byState.BusinessID != businessID {
		t.Fatalf("get state=%#v err=%v", byState, err)
	}
	if _, err := store.GetByOAuthState(ctx, "state-other"); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("expected missing state, got %v", err)
	}

	connections := NewChannelConnectionRepository(adapter)
	connection, err := connections.CreatePending(ctx, businessID, "socialapi", "facebook", "pending-connection-37", "channel-provisioning/"+newSession.ID)
	if err != nil || connection.Status != "pending" || connection.BusinessID != businessID {
		t.Fatalf("pending connection=%#v err=%v", connection, err)
	}
	active, err := connections.Activate(ctx, businessID, connection.ID, "account-37", "connection-37")
	if err != nil || active.Status != "active" || active.ProviderAccountReference == nil || *active.ProviderAccountReference != "account-37" {
		t.Fatalf("active connection=%#v err=%v", active, err)
	}
	if _, err := connections.GetByID(ctx, otherBusinessID, connection.ID); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("expected tenant isolation, got %v", err)
	}
	final, err := store.MarkProvisioning(ctx, businessID, newSession.ID, ports.ChannelProvisioningPatch{Status: ports.ProvisioningConnected, ProviderAccountRef: stringPtrIntegration("account-37"), ProviderConnectionRef: stringPtrIntegration("connection-37"), ChannelConnectionID: stringPtrIntegration(connection.ID)})
	if err != nil || final.Status != ports.ProvisioningConnected || final.ChannelConnectionID != connection.ID || final.ProviderAccountRef != "account-37" {
		t.Fatalf("final session=%#v err=%v", final, err)
	}
}

func stringPtrIntegration(value string) *string { return &value }

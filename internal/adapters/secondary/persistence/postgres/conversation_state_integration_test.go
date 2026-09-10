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

func TestConversationStateTenantIsolationAgainstPostgres(t *testing.T) {
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
	businessA := "70000000-0000-0000-0000-000000000001"
	businessB := "70000000-0000-0000-0000-000000000002"
	convA := "70000000-0000-0000-0000-000000000011"
	cleanup := func() {
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM conversation_state WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM conversations WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM customers WHERE business_id IN ($1::uuid, $2::uuid)`, businessA, businessB)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessA, businessB)
	}
	cleanup()
	defer cleanup()
	for _, id := range []string{businessA, businessB} {
		if _, err := adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, $2, $3, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', now(), now()) ON CONFLICT (id) DO NOTHING`, id, "State "+id[len(id)-2:], "state-"+id[len(id)-2:]); err != nil {
			t.Fatalf("insert business: %v", err)
		}
	}
	custA := "70000000-0000-0000-0000-000000000021"
	custB := "70000000-0000-0000-0000-000000000022"
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO customers (id, business_id, profile, contact_points, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, '{}'::jsonb, '[]'::jsonb, 'active', now(), now()) ON CONFLICT (id) DO NOTHING`, custA, businessA); err != nil {
		t.Fatalf("insert customer A: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO customers (id, business_id, profile, contact_points, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, '{}'::jsonb, '[]'::jsonb, 'active', now(), now()) ON CONFLICT (id) DO NOTHING`, custB, businessB); err != nil {
		t.Fatalf("insert customer B: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO conversations (id, business_id, customer_id, state, ownership, priority, last_activity_at, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'open', 'ai', 'normal', now(), now(), now()) ON CONFLICT (id) DO NOTHING`, convA, businessA, custA); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}

	repo := NewConversationStateRepository(adapter)
	focusID := "70000000-0000-0000-0000-000000000031"
	rec, err := repo.UpsertValidated(ctx, ports.ConversationStateRecord{
		BusinessID: businessA, ConversationID: convA,
		Focus: &ports.ConversationFocus{Type: "offer", ID: focusID},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if rec.Focus == nil || rec.Focus.ID != focusID {
		t.Fatalf("upsert focus: %#v", rec)
	}
	got, err := repo.Get(ctx, businessA, convA)
	if err != nil || got.Focus == nil || got.Focus.ID != focusID {
		t.Fatalf("get: %#v err=%v", got, err)
	}
	// Tenant isolation: B must not see A's state
	if _, err := repo.Get(ctx, businessB, convA); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("cross-tenant get must be not_found, got %v", err)
	}
}

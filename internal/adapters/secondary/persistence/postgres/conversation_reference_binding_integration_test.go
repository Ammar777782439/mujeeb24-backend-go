package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
)

func TestConversationReferenceProviderChatwootBindingAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open adapter: %v", err)
	}
	defer adapter.Close()

	businessID := uuid.NewString()
	otherBusinessID := uuid.NewString()
	customerID := uuid.NewString()
	conversationID := uuid.NewString()
	connectionID := uuid.NewString()
	referenceID := uuid.NewString()
	base := time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC)
	cleanup := func() {
		cleanupCtx := context.Background()
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM conversation_references WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM channel_connections WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM conversations WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM customers WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
	}
	defer cleanup()

	for _, business := range []struct {
		id   string
		name string
		slug string
	}{{businessID, "Binding Shop", "binding-shop-" + businessID[:8]}, {otherBusinessID, "Other Shop", "other-shop-" + otherBusinessID[:8]}} {
		if _, err := adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, $2, $3, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', $4, $4)`, business.id, business.name, business.slug, base); err != nil {
			t.Fatalf("insert business: %v", err)
		}
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO customers (id, business_id, profile, contact_points, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, '{}'::jsonb, '[]'::jsonb, 'active', $3, $3)`, customerID, businessID, base); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO conversations (id, business_id, customer_id, state, ownership, priority, last_activity_at, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'open', 'none', 'normal', $4, $4, $4)`, conversationID, businessID, customerID, base); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'socialapi', 'facebook', 'account-1', 'provider-connection-1', 'active', 'test-secret-reference', $3, $3)`, connectionID, businessID, base); err != nil {
		t.Fatalf("insert connection: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO conversation_references (id, business_id, conversation_id, system, provider_ref, resource_type, resource_id, connection_id, conversation_kind, is_current, mapping_status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'provider', 'socialapi', 'conversation', 'provider-conversation-99', $4::uuid, 'dm', true, 'active', $5, $5)`, referenceID, businessID, conversationID, connectionID, base); err != nil {
		t.Fatalf("insert provider reference: %v", err)
	}

	repo := NewConversationReferenceRepository(adapter)
	bound, err := repo.BindProviderToChatwoot(ctx, ports.ProviderChatwootBindingDraft{BusinessID: businessID, ReferenceID: referenceID, AccountID: "account-1", InboxID: "inbox-8", ChatwootConversationID: "chatwoot-501"})
	if err != nil {
		t.Fatalf("bind provider reference to Chatwoot: %v", err)
	}
	if bound.ChatwootAccountID == nil || *bound.ChatwootAccountID != "account-1" || bound.ChatwootInboxID == nil || *bound.ChatwootInboxID != "inbox-8" || bound.ChatwootConversationID == nil || *bound.ChatwootConversationID != "chatwoot-501" {
		t.Fatalf("unexpected bound reference: %#v", bound)
	}
	record, err := repo.GetCurrentProviderByChatwoot(ctx, businessID, "account-1", "inbox-8", "chatwoot-501")
	if err != nil {
		t.Fatalf("valid binding lookup: %v", err)
	}
	if record.ID != referenceID || record.BusinessID != businessID || record.ConversationID != conversationID || record.ResourceID != "provider-conversation-99" || record.ChatwootConversationID == nil || *record.ChatwootConversationID != "chatwoot-501" {
		t.Fatalf("unexpected binding record: %#v", record)
	}
	if _, err := repo.GetCurrentProviderByChatwoot(ctx, otherBusinessID, "account-1", "inbox-8", "chatwoot-501"); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("expected tenant-isolated not found, got %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `UPDATE conversation_references SET is_current = false WHERE id = $1::uuid`, referenceID); err != nil {
		t.Fatalf("mark stale: %v", err)
	}
	if _, err := repo.GetCurrentProviderByChatwoot(ctx, businessID, "account-1", "inbox-8", "chatwoot-501"); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("expected stale reference not found, got %v", err)
	}

	if _, err := adapter.Pool().Exec(ctx, `UPDATE conversation_references SET is_current = true WHERE id = $1::uuid`, referenceID); err != nil {
		t.Fatalf("restore current: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO conversation_references (id, business_id, conversation_id, system, provider_ref, resource_type, resource_id, connection_id, conversation_kind, is_current, mapping_status, chatwoot_account_id, chatwoot_inbox_id, chatwoot_conversation_id, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'provider', 'socialapi', 'conversation', 'provider-conversation-duplicate', $4::uuid, 'dm', true, 'active', 'account-1', 'inbox-8', 'chatwoot-501', $5, $5)`, uuid.NewString(), businessID, conversationID, connectionID, base); err == nil {
		t.Fatal("expected duplicate current Chatwoot mapping constraint")
	}
}

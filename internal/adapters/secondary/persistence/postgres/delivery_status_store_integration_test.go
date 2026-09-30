//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
)

func TestDeliveryStatusStoreAgainstPostgres(t *testing.T) {
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

	businessID, otherBusinessID, connectionID, customerID, conversationID, referenceID, outboundID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	base := time.Date(2026, 8, 25, 14, 0, 0, 0, time.UTC)
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, 'Delivery Business', $1, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', $3, $3), ($2::uuid, 'Other Delivery Business', $2, 'active', 'travel', 'Asia/Aden', 'YER', 'ar-YE', $3, $3)`, businessID, otherBusinessID, base); err != nil {
		t.Fatalf("insert businesses: %v", err)
	}
	defer func() {
		cleanup := context.Background()
		_, _ = adapter.Pool().Exec(cleanup, `DELETE FROM outbound_delivery_status_updates WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanup, `DELETE FROM outbound_messages WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanup, `DELETE FROM conversation_references WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanup, `DELETE FROM conversations WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanup, `DELETE FROM customers WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanup, `DELETE FROM inbound_event_ledger WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanup, `DELETE FROM channel_connections WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanup, `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
	}()
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'socialapi', 'whatsapp', 'account-a', 'connection-a', 'active', 'secret://delivery', $3, $3)`, connectionID, businessID, base); err != nil {
		t.Fatalf("connection: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO customers (id, business_id, profile, contact_points, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, '{}'::jsonb, '[]'::jsonb, 'active', $3, $3)`, customerID, businessID, base); err != nil {
		t.Fatalf("customer: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO conversations (id, business_id, customer_id, state, ownership, priority, last_activity_at, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'open', 'none', 'normal', $4, $4, $4)`, conversationID, businessID, customerID, base); err != nil {
		t.Fatalf("conversation: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO conversation_references (id, business_id, conversation_id, system, provider_ref, resource_type, resource_id, connection_id, conversation_kind, is_current, mapping_status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'provider', 'socialapi', 'conversation', 'conversation-a', $4::uuid, 'dm', true, 'active', $5, $5)`, referenceID, businessID, conversationID, connectionID, base); err != nil {
		t.Fatalf("reference: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO outbound_messages (id, business_id, conversation_id, conversation_reference_id, connection_id, provider_ref, channel, origin, direction, transport, content_reference, provider_idempotency_key, status, provider_message_id, attempt_count, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5::uuid, 'socialapi', 'whatsapp', 'ai', 'outbound', 'provider', 'outbound://test', 'idempotency-1', 'accepted', 'provider-message-1', 1, $6, $6)`, outboundID, businessID, conversationID, referenceID, connectionID, base); err != nil {
		t.Fatalf("outbound: %v", err)
	}

	store := NewDeliveryStatusStore(adapter)
	eventID := insertDeliveryStatusEvent(t, ctx, adapter, businessID, connectionID, "status-event-1", base)
	first, err := store.Apply(ctx, ports.DeliveryStatusDraft{InboundEventID: eventID, BusinessID: businessID, ConnectionID: connectionID, ProviderRef: "socialapi", ProviderAccountRef: "account-a", ProviderMessageID: "provider-message-1", Status: "delivered", OccurredAt: base.Add(time.Minute)})
	if err != nil || !first.Applied || first.OutboundMessageID != outboundID || first.Status != "delivered" {
		t.Fatalf("first apply result=%#v err=%v", first, err)
	}
	duplicate, err := store.Apply(ctx, ports.DeliveryStatusDraft{InboundEventID: eventID, BusinessID: businessID, ConnectionID: connectionID, ProviderRef: "socialapi", ProviderAccountRef: "account-a", ProviderMessageID: "provider-message-1", Status: "delivered", OccurredAt: base.Add(time.Minute)})
	if err != nil || !duplicate.Duplicate {
		t.Fatalf("duplicate apply result=%#v err=%v", duplicate, err)
	}
	eventID2 := insertDeliveryStatusEvent(t, ctx, adapter, businessID, connectionID, "status-event-2", base.Add(2*time.Minute))
	second, err := store.Apply(ctx, ports.DeliveryStatusDraft{InboundEventID: eventID2, BusinessID: businessID, ConnectionID: connectionID, ProviderRef: "socialapi", ProviderAccountRef: "account-a", ProviderMessageID: "provider-message-1", Status: "sent", OccurredAt: base.Add(2 * time.Minute)})
	if err != nil || !second.Applied || second.Status != "delivered" {
		t.Fatalf("downgrade apply result=%#v err=%v", second, err)
	}
	eventID3 := insertDeliveryStatusEvent(t, ctx, adapter, businessID, connectionID, "status-event-3", base.Add(3*time.Minute))
	unmatched, err := store.Apply(ctx, ports.DeliveryStatusDraft{InboundEventID: eventID3, BusinessID: businessID, ConnectionID: connectionID, ProviderRef: "socialapi", ProviderAccountRef: "account-a", ProviderMessageID: "unmatched-provider-message", Status: "delivered", OccurredAt: base.Add(3 * time.Minute)})
	if err != nil || !unmatched.Ignored || unmatched.OutboundMessageID != "" {
		t.Fatalf("unmatched apply result=%#v err=%v", unmatched, err)
	}
	if _, err := store.Apply(ctx, ports.DeliveryStatusDraft{InboundEventID: eventID3, BusinessID: otherBusinessID, ConnectionID: connectionID, ProviderRef: "socialapi", ProviderAccountRef: "account-a", ProviderMessageID: "unmatched-provider-message", Status: "delivered", OccurredAt: base.Add(3 * time.Minute)}); !IsRepositoryKind(err, RepositoryConflict) {
		t.Fatalf("expected tenant conflict, got %v", err)
	}
	var status, processedState string
	if err := adapter.Pool().QueryRow(ctx, `SELECT status FROM outbound_messages WHERE business_id = $1::uuid AND id = $2::uuid`, businessID, outboundID).Scan(&status); err != nil || status != "delivered" {
		t.Fatalf("outbound status=%q err=%v", status, err)
	}
	if err := adapter.Pool().QueryRow(ctx, `SELECT processing_state FROM inbound_event_ledger WHERE id = $1::uuid`, eventID3).Scan(&processedState); err != nil || processedState != "processed" {
		t.Fatalf("unmatched ledger state=%q err=%v", processedState, err)
	}
}

func insertDeliveryStatusEvent(t *testing.T, ctx context.Context, adapter *Adapter, businessID, connectionID, providerEventID string, at time.Time) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO inbound_event_ledger (id, provider_ref, provider_connection_ref, provider_event_id, dedupe_strategy, business_id, connection_id, event_type, provider_message_id, received_at, raw_payload_reference, payload_hash, signature_verified, processing_state, attempt_count, created_at, updated_at) VALUES ($1::uuid, 'socialapi', 'account-a', $2, 'provider_event_id', $3::uuid, $4::uuid, 'delivery_status_changed', 'provider-message-1', $5, 'payload://delivery-status', repeat('a', 64), true, 'received', 0, $5, $5)`, id, providerEventID, businessID, connectionID, at); err != nil {
		t.Fatalf("insert delivery status event: %v", err)
	}
	return id
}

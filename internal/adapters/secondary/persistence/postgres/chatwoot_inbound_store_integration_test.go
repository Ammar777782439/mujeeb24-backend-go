//go:build integration

package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
)

func TestChatwootInboundStoreAgainstPostgres(t *testing.T) {
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
	bindingID := uuid.NewString()
	otherBindingID := uuid.NewString()
	routeKey := "chatwoot-test-" + uuid.NewString()
	otherRouteKey := "other-route-" + uuid.NewString()
	base := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	_, err = adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, 'Chatwoot Business', $1, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', $3, $3), ($2::uuid, 'Other Business', $2, 'active', 'travel', 'Asia/Aden', 'YER', 'ar-YE', $3, $3)`, businessID, otherBusinessID, base)
	if err != nil {
		t.Fatalf("insert businesses: %v", err)
	}
	defer func() {
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM communication_messages WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM conversation_references WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM conversations WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM chatwoot_contact_links WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM customers WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM inbound_event_ledger WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM chatwoot_workspace_bindings WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(context.Background(), `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
	}()
	_, err = adapter.Pool().Exec(ctx, `INSERT INTO chatwoot_workspace_bindings (id, business_id, route_key, account_id, inbox_id, channel, active, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3, '12', '34', 'other', true, $4, $4), ($5::uuid, $6::uuid, $7, '12', '35', 'other', true, $4, $4)`, bindingID, businessID, routeKey, base, otherBindingID, otherBusinessID, otherRouteKey)
	if err != nil {
		t.Fatalf("insert bindings: %v", err)
	}

	store := NewChatwootInboundStore(adapter)
	draft := ports.ChatwootInboundDraft{EventID: "cw-event-901", RouteKey: routeKey, AccountID: "12", InboxID: "34", ConversationID: "78", ExternalUserID: "56", ProviderMessageID: "901", Content: "hello", EventType: "interaction_received", OccurredAt: base, ReceivedAt: base, RawPayloadReference: "chatwoot://webhook/901", PayloadHash: "hash-901"}
	first, err := store.Materialize(ctx, draft)
	if err != nil {
		t.Fatalf("first materialize: %v", err)
	}
	if first.Duplicate || first.CustomerID == "" || first.ConversationID == "" || first.ConversationReferenceID == "" || first.CommunicationMessageID == "" || first.InboundEventID == "" {
		t.Fatalf("unexpected first result: %#v", first)
	}

	second, err := store.Materialize(ctx, draft)
	if err != nil {
		t.Fatalf("duplicate materialize: %v", err)
	}
	if !second.Duplicate || second.CustomerID != first.CustomerID || second.ConversationID != first.ConversationID || second.ConversationReferenceID != first.ConversationReferenceID || second.CommunicationMessageID != first.CommunicationMessageID || second.InboundEventID != first.InboundEventID {
		t.Fatalf("duplicate was not idempotent: first=%#v second=%#v", first, second)
	}

	var customerCount, conversationCount, referenceCount, messageCount, processedCount int
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM customers WHERE business_id = $1::uuid`, businessID).Scan(&customerCount); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM conversations WHERE business_id = $1::uuid`, businessID).Scan(&conversationCount); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM conversation_references WHERE business_id = $1::uuid AND system = 'chatwoot'`, businessID).Scan(&referenceCount); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM communication_messages WHERE business_id = $1::uuid AND chatwoot_message_id = '901'`, businessID).Scan(&messageCount); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM inbound_event_ledger WHERE business_id = $1::uuid AND provider_ref = 'chatwoot' AND provider_event_id = 'cw-event-901' AND processing_state = 'processed'`, businessID).Scan(&processedCount); err != nil {
		t.Fatal(err)
	}
	if customerCount != 1 || conversationCount != 1 || referenceCount != 1 || messageCount != 1 || processedCount != 1 {
		t.Fatalf("unexpected materialized counts customer=%d conversation=%d reference=%d message=%d processed=%d", customerCount, conversationCount, referenceCount, messageCount, processedCount)
	}

	_, err = store.Materialize(ctx, ports.ChatwootInboundDraft{EventID: "cw-event-902", RouteKey: routeKey, AccountID: "12", InboxID: "35", ConversationID: "79", ExternalUserID: "57", ProviderMessageID: "902", Content: "other", EventType: "interaction_received", OccurredAt: base, ReceivedAt: base, RawPayloadReference: "chatwoot://webhook/902", PayloadHash: "hash-902"})
	if !containsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("expected binding isolation/not found, got %v", err)
	}
}

func containsRepositoryKind(err error, kind RepositoryErrorKind) bool {
	var repositoryErr *RepositoryError
	return errors.As(err, &repositoryErr) && repositoryErr.Kind == kind
}

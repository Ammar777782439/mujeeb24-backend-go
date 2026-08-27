//go:build integration

package postgres

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/providers/socialapi"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
)

func TestProviderInboundStoreAgainstPostgres(t *testing.T) {
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
	connectionID := uuid.NewString()
	otherConnectionID := uuid.NewString()
	base := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	_, err = adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, 'Provider Business', $1, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', $3, $3), ($2::uuid, 'Other Provider Business', $2, 'active', 'travel', 'Asia/Aden', 'YER', 'ar-YE', $3, $3)`, businessID, otherBusinessID, base)
	if err != nil {
		t.Fatalf("insert businesses: %v", err)
	}
	defer func() {
		cleanupCtx := context.Background()
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM chatwoot_mirror_jobs WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM chatwoot_workspace_bindings WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM communication_messages WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM conversation_references WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM external_identities WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM conversations WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM customers WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM inbound_event_ledger WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM channel_connections WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM inbound_webhook_payloads WHERE provider_ref = 'socialapi' AND delivery_id LIKE 'provider-test-%'`)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM businesses WHERE id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
	}()
	_, err = adapter.Pool().Exec(ctx, `INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'socialapi', 'whatsapp', 'account-provider-a', 'connection-provider-a', 'active', 'secret://provider-a', $5, $5), ($3::uuid, $4::uuid, 'socialapi', 'whatsapp', 'account-provider-b', 'connection-provider-b', 'active', 'secret://provider-b', $5, $5)`, connectionID, businessID, otherConnectionID, otherBusinessID, base)
	if err != nil {
		t.Fatalf("insert connections: %v", err)
	}

	rawStore := NewRawPayloadStore(adapter)
	raw, err := rawStore.Put(ctx, "socialapi", "provider-test-1", []byte(`{"event":"dm.received"}`))
	if err != nil {
		t.Fatalf("store raw payload: %v", err)
	}
	rawAgain, err := rawStore.Put(ctx, "socialapi", "provider-test-1", []byte(`{"event":"dm.received"}`))
	if err != nil || rawAgain.Reference != raw.Reference || rawAgain.SHA256 != raw.SHA256 {
		t.Fatalf("raw payload was not idempotent: first=%#v second=%#v err=%v", raw, rawAgain, err)
	}
	if _, err := rawStore.Put(ctx, "socialapi", "provider-test-1", []byte(`{"event":"different"}`)); !containsRepositoryKind(err, RepositoryConflict) {
		t.Fatalf("expected raw payload conflict, got %v", err)
	}

	eventID := uuid.NewString()
	businessPtr := businessID
	connectionPtr := connectionID
	eventStore := NewInboundEventStore(adapter)
	eventDraft := ports.InboundEventDraft{ID: eventID, ProviderRef: "socialapi", ProviderConnectionRef: "account-provider-a", ProviderEventID: "provider-event-1", DedupeStrategy: "provider_event_id", BusinessID: &businessPtr, ConnectionID: &connectionPtr, EventType: "interaction_received", InteractionKind: stringPointerForProviderInbound("dm"), ProviderMessageID: stringPointerForProviderInbound("provider-message-1"), ProviderConversationID: stringPointerForProviderInbound("provider-conversation-1"), ExternalUserID: stringPointerForProviderInbound("provider-user-1"), ReceivedAt: base, RawPayloadReference: raw.Reference, PayloadHash: raw.SHA256, SignatureVerified: true, ProcessingState: "received", CreatedAt: base, UpdatedAt: base}
	created, eventRecord, err := eventStore.RecordIfAbsent(ctx, eventDraft)
	if err != nil || !created || eventRecord.ID != eventID {
		t.Fatalf("record event: created=%v record=%#v err=%v", created, eventRecord, err)
	}

	materializer := NewProviderInboundStoreWithMirror(adapter, true)
	draft := ports.ProviderInboundDraft{InboundEventID: eventRecord.ID, BusinessID: businessID, ConnectionID: connectionID, ProviderRef: "socialapi", ProviderEventID: "provider-event-1", Channel: "whatsapp", ProviderAccountRef: "account-provider-a", ProviderConversationID: "provider-conversation-1", ExternalUserID: "provider-user-1", ProviderMessageID: "provider-message-1", EventType: "interaction_received", InteractionKind: "dm", Text: "مرحبا", ExternalCreatedAt: providerInboundTimePtr(base), ReceivedAt: base, RawPayloadReference: raw.Reference, PayloadHash: raw.SHA256}
	first, err := materializer.Materialize(ctx, draft)
	if err != nil {
		t.Fatalf("first materialize: %v", err)
	}
	if first.Duplicate || first.Ignored || first.CustomerID == "" || first.ConversationID == "" || first.ConversationReferenceID == "" || first.CommunicationMessageID == "" || first.InboundEventID != eventID {
		t.Fatalf("unexpected first materialization: %#v", first)
	}
	second, err := materializer.Materialize(ctx, draft)
	if err != nil {
		t.Fatalf("duplicate materialize: %v", err)
	}
	if !second.Duplicate || second.CustomerID != first.CustomerID || second.ConversationID != first.ConversationID || second.ConversationReferenceID != first.ConversationReferenceID || second.CommunicationMessageID != first.CommunicationMessageID || second.InboundEventID != eventID {
		t.Fatalf("duplicate was not idempotent: first=%#v second=%#v", first, second)
	}
	routeKey := "cw_" + strings.ReplaceAll(connectionID, "-", "")
	if _, err := NewChatwootWorkspaceBindingRepository(adapter).EnsureBinding(ctx, businessID, routeKey, "901", "902", "whatsapp"); err != nil {
		t.Fatalf("ensure workspace binding: %v", err)
	}
	mirrorStore := NewChatwootMirrorStore(adapter)
	jobs, err := mirrorStore.ListClaimable(ctx, 10)
	if err != nil || len(jobs) != 1 || jobs[0].CommunicationMessageID != first.CommunicationMessageID {
		t.Fatalf("unexpected mirror jobs=%#v err=%v", jobs, err)
	}
	lease := ports.MirrorLease{Owner: "mirror-test", Token: uuid.NewString(), ExpiresAt: base.Add(time.Minute)}
	claim, err := mirrorStore.Claim(ctx, jobs[0].ID, lease)
	if err != nil || !claim.Claimed || claim.Record.AttemptCount != 1 {
		t.Fatalf("claim mirror: result=%#v err=%v", claim, err)
	}
	delivery, err := mirrorStore.Resolve(ctx, businessID, jobs[0].ID)
	if err != nil || delivery.CustomerIdentifier != "provider-user-1" || delivery.ProviderConversationID != "provider-conversation-1" || delivery.Text != "مرحبا" || delivery.AccountID != 901 || delivery.InboxID != 902 {
		t.Fatalf("resolve mirror: delivery=%#v err=%v", delivery, err)
	}
	completed, err := mirrorStore.MarkCompleted(ctx, ports.ChatwootMirrorCompletion{JobID: jobs[0].ID, Owner: lease.Owner, Token: lease.Token, AccountID: 901, InboxID: 902, ChatwootContactID: "1001", ChatwootConversationID: "1002", ChatwootMessageID: "1003", CompletedAt: base.Add(2 * time.Minute), UpdatedAt: base.Add(2 * time.Minute)})
	if err != nil || completed.Status != "completed" || completed.ChatwootMessageID == nil || *completed.ChatwootMessageID != "1003" {
		t.Fatalf("complete mirror: result=%#v err=%v", completed, err)
	}
	var mirroredMessageID, mirroredAccountID, mirroredInboxID, mirroredConversationID string
	if err := adapter.Pool().QueryRow(ctx, `SELECT m.chatwoot_message_id, r.chatwoot_account_id, r.chatwoot_inbox_id, r.chatwoot_conversation_id FROM communication_messages m JOIN conversation_references r ON r.business_id = m.business_id AND r.id = m.conversation_reference_id WHERE m.business_id = $1::uuid AND m.id = $2::uuid`, businessID, first.CommunicationMessageID).Scan(&mirroredMessageID, &mirroredAccountID, &mirroredInboxID, &mirroredConversationID); err != nil {
		t.Fatalf("read mirror projections: %v", err)
	}
	if mirroredMessageID != "1003" || mirroredAccountID != "901" || mirroredInboxID != "902" || mirroredConversationID != "1002" {
		t.Fatalf("unexpected mirror projections message=%s account=%s inbox=%s conversation=%s", mirroredMessageID, mirroredAccountID, mirroredInboxID, mirroredConversationID)
	}

	var customerCount, identityCount, conversationCount, referenceCount, messageCount, processedCount int
	queries := []struct {
		name  string
		query string
		out   *int
	}{
		{"customers", `SELECT count(*) FROM customers WHERE business_id = $1::uuid`, &customerCount},
		{"identities", `SELECT count(*) FROM external_identities WHERE business_id = $1::uuid AND connection_id = $2::uuid AND external_user_id = 'provider-user-1'`, &identityCount},
		{"conversations", `SELECT count(*) FROM conversations WHERE business_id = $1::uuid`, &conversationCount},
		{"references", `SELECT count(*) FROM conversation_references WHERE business_id = $1::uuid AND system = 'provider' AND resource_id = 'provider-conversation-1'`, &referenceCount},
		{"messages", `SELECT count(*) FROM communication_messages WHERE business_id = $1::uuid AND provider_message_id = 'provider-message-1'`, &messageCount},
		{"processed ledger", `SELECT count(*) FROM inbound_event_ledger WHERE id = $1::uuid AND processing_state = 'processed' AND processing_result_code = 'socialapi_materialized'`, &processedCount},
	}
	for _, query := range queries {
		var queryErr error
		switch query.name {
		case "identities":
			queryErr = adapter.Pool().QueryRow(ctx, query.query, businessID, connectionID).Scan(query.out)
		case "processed ledger":
			queryErr = adapter.Pool().QueryRow(ctx, query.query, eventID).Scan(query.out)
		default:
			queryErr = adapter.Pool().QueryRow(ctx, query.query, businessID).Scan(query.out)
		}
		if queryErr != nil {
			t.Fatalf("count %s: %v", query.name, queryErr)
		}
	}
	if customerCount != 1 || identityCount != 1 || conversationCount != 1 || referenceCount != 1 || messageCount != 1 || processedCount != 1 {
		t.Fatalf("unexpected materialized counts customer=%d identity=%d conversation=%d reference=%d message=%d processed=%d", customerCount, identityCount, conversationCount, referenceCount, messageCount, processedCount)
	}

	var transport, direction, origin, content string
	if err := adapter.Pool().QueryRow(ctx, `SELECT transport, direction, origin, text_content FROM communication_messages WHERE id = $1::uuid`, first.CommunicationMessageID).Scan(&transport, &direction, &origin, &content); err != nil {
		t.Fatalf("read communication message: %v", err)
	}
	if transport != "provider" || direction != "inbound" || origin != "customer" || content != "مرحبا" {
		t.Fatalf("unexpected communication message projection transport=%s direction=%s origin=%s content=%s", transport, direction, origin, content)
	}

	wrongDraft := draft
	wrongDraft.BusinessID = otherBusinessID
	if _, err := materializer.Materialize(ctx, wrongDraft); !containsRepositoryKind(err, RepositoryConflict) {
		t.Fatalf("expected tenant conflict, got %v", err)
	}

	rollbackEventID := uuid.NewString()
	rollbackDraft := eventDraft
	rollbackDraft.ID = rollbackEventID
	rollbackDraft.ProviderEventID = "provider-event-rollback"
	rollbackDraft.ProviderMessageID = stringPointerForProviderInbound("provider-message-rollback")
	rollbackDraft.ProviderConversationID = stringPointerForProviderInbound("provider-conversation-rollback")
	rollbackDraft.ExternalUserID = stringPointerForProviderInbound("provider-user-rollback")
	if _, _, err := eventStore.RecordIfAbsent(ctx, rollbackDraft); err != nil {
		t.Fatalf("record rollback event: %v", err)
	}
	failedDraft := draft
	failedDraft.InboundEventID = rollbackEventID
	failedDraft.ProviderConversationID = "provider-conversation-rollback"
	failedDraft.ProviderMessageID = "provider-message-rollback"
	failedDraft.ExternalUserID = "provider-user-rollback"
	failedDraft.Channel = "unsupported-channel"
	if _, err := materializer.Materialize(ctx, failedDraft); err == nil {
		t.Fatal("expected unsupported channel failure")
	}
	var rollbackCount int
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM customers WHERE business_id = $1::uuid AND profile->>'external_user_id' = 'provider-user-rollback'`, businessID).Scan(&rollbackCount); err != nil {
		t.Fatal(err)
	}
	if rollbackCount != 0 {
		t.Fatalf("transaction did not roll back customer creation: %d", rollbackCount)
	}
}

func stringPointerForProviderInbound(value string) *string { return &value }
func providerInboundTimePtr(value time.Time) *time.Time    { return &value }

func TestSocialAPIWebhookRuntimeMaterializesAgainstPostgres(t *testing.T) {
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
	connectionID := uuid.NewString()
	base := time.Date(2026, 8, 25, 13, 0, 0, 0, time.UTC)
	_, err = adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, 'Runtime Provider Business', $1, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', $2, $2)`, businessID, base)
	if err != nil {
		t.Fatalf("insert business: %v", err)
	}
	defer func() {
		cleanupCtx := context.Background()
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM communication_messages WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM conversation_references WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM external_identities WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM conversations WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM customers WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM inbound_event_ledger WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM channel_connections WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM inbound_webhook_payloads WHERE provider_ref = 'socialapi' AND delivery_id = 'provider-runtime-1'`)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM businesses WHERE id = $1::uuid`, businessID)
	}()
	_, err = adapter.Pool().Exec(ctx, `INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'socialapi', 'whatsapp', 'account-runtime', 'connection-runtime', 'active', 'secret://runtime', $3, $3)`, connectionID, businessID, base)
	if err != nil {
		t.Fatalf("insert connection: %v", err)
	}

	body := []byte(`{"event":"dm.received","data":{"id":"runtime-event-1","type":"dm","platform":"whatsapp","account_id":"account-runtime","conversation_id":"runtime-conversation-1","author":{"id":"runtime-user-1"},"content":{"text":"أريد معرفة السعر"},"received_at":"2026-08-25T13:00:00Z"}}`)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte("runtime-secret"))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	command := commands.IngestWebhookCommand{RouteKey: "socialapi-runtime", DeliveryID: "provider-runtime-1", RequestID: "runtime-request-1", ProviderHeaders: map[string]string{"X-SocialAPI-Signature-V2": "sha256=" + hex.EncodeToString(mac.Sum(nil)), "X-SocialAPI-Timestamp": timestamp, "X-SocialAPI-Delivery": "provider-runtime-1"}, RawPayload: body}
	service := services.SocialAPIWebhookService{Receiver: socialapi.NewClient(socialapi.Config{WebhookSecret: "runtime-secret"}), RawPayloads: NewRawPayloadStore(adapter), Connections: NewChannelConnectionRepository(adapter), Events: NewInboundEventStore(adapter), Inbound: NewProviderInboundStore(adapter)}
	first, err := service.Handle(ctx, command)
	if err != nil || !first.Accepted || !first.Resolved || first.Duplicate {
		t.Fatalf("first runtime handle: result=%#v err=%v", first, err)
	}
	second, err := service.Handle(ctx, command)
	if err != nil || !second.Accepted || !second.Duplicate {
		t.Fatalf("duplicate runtime handle: result=%#v err=%v", second, err)
	}
	var customerCount, conversationCount, referenceCount, messageCount, processedCount int
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM customers WHERE business_id = $1::uuid`, businessID).Scan(&customerCount); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM conversations WHERE business_id = $1::uuid`, businessID).Scan(&conversationCount); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM conversation_references WHERE business_id = $1::uuid AND system = 'provider'`, businessID).Scan(&referenceCount); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM communication_messages WHERE business_id = $1::uuid AND provider_message_id = 'runtime-event-1'`, businessID).Scan(&messageCount); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM inbound_event_ledger WHERE business_id = $1::uuid AND processing_state = 'processed' AND processing_result_code = 'socialapi_materialized'`, businessID).Scan(&processedCount); err != nil {
		t.Fatal(err)
	}
	if customerCount != 1 || conversationCount != 1 || referenceCount != 1 || messageCount != 1 || processedCount != 1 {
		t.Fatalf("unexpected runtime counts customer=%d conversation=%d reference=%d message=%d processed=%d", customerCount, conversationCount, referenceCount, messageCount, processedCount)
	}
}

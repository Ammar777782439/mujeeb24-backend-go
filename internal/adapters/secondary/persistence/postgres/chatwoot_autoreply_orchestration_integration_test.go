//go:build integration

package postgres

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/workspaces/chatwoot"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
)

func TestChatwootInboundAutoReplyOrchestrationAgainstPostgres(t *testing.T) {
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
	customerID := uuid.NewString()
	conversationID := uuid.NewString()
	connectionID := uuid.NewString()
	chatwootReferenceID := uuid.NewString()
	providerReferenceID := uuid.NewString()
	bindingID := uuid.NewString()
	base := time.Date(2026, 8, 25, 19, 0, 0, 0, time.UTC)
	cleanup := func() {
		cleanupCtx := context.Background()
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM outbox_entries WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM outbound_messages WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM communication_messages WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM ai_decisions WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM inbound_event_ledger WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM conversation_references WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM conversations WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM chatwoot_contact_links WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM chatwoot_workspace_bindings WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM channel_connections WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM customers WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM businesses WHERE id = $1::uuid`, businessID)
	}
	defer cleanup()

	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO businesses (id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at) VALUES ($1::uuid, 'Chatwoot Auto Reply Business', $1, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', $2, $2)`, businessID, base); err != nil {
		t.Fatalf("insert business: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO customers (id, business_id, profile, contact_points, status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, '{}'::jsonb, '[]'::jsonb, 'active', $3, $3)`, customerID, businessID, base); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO conversations (id, business_id, customer_id, state, ownership, priority, last_activity_at, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'open', 'none', 'normal', $4, $4, $4)`, conversationID, businessID, customerID, base); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO channel_connections (id, business_id, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'socialapi', 'facebook', 'account-1', 'connection-1', 'active', 'local-secret-ref', $3, $3)`, connectionID, businessID, base); err != nil {
		t.Fatalf("insert provider connection: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO chatwoot_workspace_bindings (id, business_id, route_key, account_id, inbox_id, channel, active, created_at, updated_at) VALUES ($1::uuid, $2::uuid, 'chatwoot-test', '12', '34', 'facebook', true, $3, $3)`, bindingID, businessID, base); err != nil {
		t.Fatalf("insert Chatwoot binding: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `INSERT INTO conversation_references (id, business_id, conversation_id, system, provider_ref, resource_type, resource_id, connection_id, conversation_kind, is_current, mapping_status, created_at, updated_at) VALUES ($1::uuid, $2::uuid, $3::uuid, 'chatwoot', 'chatwoot', 'conversation', '78', NULL, 'dm', true, 'active', $4, $4), ($5::uuid, $2::uuid, $3::uuid, 'provider', 'socialapi', 'conversation', 'provider-conversation-1', $6::uuid, 'dm', true, 'active', $4, $4)`, chatwootReferenceID, businessID, conversationID, base, providerReferenceID, connectionID); err != nil {
		t.Fatalf("insert conversation references: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `UPDATE conversation_references SET chatwoot_account_id = '12', chatwoot_inbox_id = '34', chatwoot_conversation_id = '78' WHERE id = $1::uuid`, providerReferenceID); err != nil {
		t.Fatalf("bind provider reference fixture: %v", err)
	}

	receiver := chatwoot.NewClient(chatwoot.Config{WebhookSecret: "test-chatwoot-secret"})
	referenceRepository := NewConversationReferenceRepository(adapter)
	bridge := services.ChatwootAutoReplyBridge{
		Resolver: services.ChatwootProviderReferenceResolver{
			References:  referenceRepository,
			Connections: NewChannelConnectionRepository(adapter),
		},
		AutoReply: services.NewAutoReplyService(
			services.SafeAutoReplyRuntime{},
			NewAIDecisionRepository(adapter),
			referenceRepository,
			NewOutboundMessageRepository(adapter),
			NewPostgresOutboxStore(adapter),
			adapter,
		),
	}
	service := services.ChatwootWebhookService{Receiver: receiver, Inbound: NewChatwootInboundStore(adapter), AutoReply: &bridge}

	incomingBody := []byte(`{"event":"message_created","id":10001,"content":"مرحبا","message_type":"incoming","account":{"id":12},"inbox":{"id":34},"conversation":{"id":78},"sender":{"id":56,"type":"contact"}}`)
	incomingCommand := signedChatwootIntegrationCommand(t, incomingBody, "test-chatwoot-secret")
	incomingResult, err := service.Handle(ctx, incomingCommand)
	if err != nil || !incomingResult.Accepted || !incomingResult.Resolved || incomingResult.Duplicate {
		t.Fatalf("incoming callback result=%#v err=%v", incomingResult, err)
	}
	assertAutoReplyCounts(t, ctx, adapter, businessID, 1, 1, 1, 1, "inbound")

	duplicateResult, err := service.Handle(ctx, incomingCommand)
	if err != nil || !duplicateResult.Accepted || !duplicateResult.Duplicate {
		t.Fatalf("duplicate callback result=%#v err=%v", duplicateResult, err)
	}
	assertAutoReplyCounts(t, ctx, adapter, businessID, 1, 1, 1, 1, "inbound")

	outgoingBody := []byte(`{"event":"message_created","id":10002,"content":"رد النظام","message_type":"outgoing","account":{"id":12},"inbox":{"id":34},"conversation":{"id":78},"sender":{"id":0,"type":"agentbot"}}`)
	outgoingResult, err := service.Handle(ctx, signedChatwootIntegrationCommand(t, outgoingBody, "test-chatwoot-secret"))
	if err != nil || !outgoingResult.Accepted || outgoingResult.Duplicate {
		t.Fatalf("outgoing callback result=%#v err=%v", outgoingResult, err)
	}
	assertAutoReplyCounts(t, ctx, adapter, businessID, 1, 1, 1, 2, "outbound")
}

func signedChatwootIntegrationCommand(t *testing.T, body []byte, secret string) commands.IngestWebhookCommand {
	t.Helper()
	timestamp := time.Now().UTC().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(formatUnix(timestamp) + "." + string(body)))
	return commands.IngestWebhookCommand{RouteKey: "chatwoot-test", DeliveryID: "delivery-" + uuid.NewString(), RequestID: "request-" + uuid.NewString(), ProviderHeaders: map[string]string{"X-Chatwoot-Signature": "sha256=" + hex.EncodeToString(mac.Sum(nil)), "X-Chatwoot-Timestamp": formatUnix(timestamp)}, RawPayload: body}
}

func assertAutoReplyCounts(t *testing.T, ctx context.Context, adapter *Adapter, businessID string, decisions, outbound, outbox, communication int, lastDirection string) {
	t.Helper()
	var decisionCount, outboundCount, outboxCount, communicationCount int
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM ai_decisions WHERE business_id = $1::uuid AND requested_action = 'answer' AND lifecycle = 'proposed'`, businessID).Scan(&decisionCount); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM outbound_messages WHERE business_id = $1::uuid AND status = 'pending' AND origin = 'ai' AND transport = 'provider'`, businessID).Scan(&outboundCount); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM outbox_entries WHERE business_id = $1::uuid AND command_type = $2 AND status = 'pending'`, businessID, services.OutboundSendCommandType).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Pool().QueryRow(ctx, `SELECT count(*) FROM communication_messages WHERE business_id = $1::uuid`, businessID).Scan(&communicationCount); err != nil {
		t.Fatal(err)
	}
	var direction string
	if err := adapter.Pool().QueryRow(ctx, `SELECT direction FROM communication_messages WHERE business_id = $1::uuid ORDER BY occurred_at DESC, created_at DESC LIMIT 1`, businessID).Scan(&direction); err != nil {
		t.Fatal(err)
	}
	if decisionCount != decisions || outboundCount != outbound || outboxCount != outbox || communicationCount != communication || direction != lastDirection {
		t.Fatalf("counts decisions=%d outbound=%d outbox=%d communication=%d last_direction=%s; want %d/%d/%d/%d/%s", decisionCount, outboundCount, outboxCount, communicationCount, direction, decisions, outbound, outbox, communication, lastDirection)
	}
}

func formatUnix(value int64) string {
	return strconv.FormatInt(value, 10)
}

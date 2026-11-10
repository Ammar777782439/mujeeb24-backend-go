//go:build integration

package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
)

type outboundMediaIntegrationInbox struct {
	ports.ConversationInboxRepository
}

func (outboundMediaIntegrationInbox) GetInboxConversation(_ context.Context, businessID, conversationID string, access ports.ConversationAccess, _ bool) (ports.ConversationRecord, error) {
	assigned := access.PrincipalID
	return ports.ConversationRecord{
		ID: conversationID, BusinessID: businessID, State: "human_handling",
		Ownership: "human", AssignmentReference: &assigned,
	}, nil
}

type outboundMediaFailingOutbox struct {
	ports.OutboxStore
	cause error
}

func (f outboundMediaFailingOutbox) Enqueue(ctx context.Context, draft ports.OutboxEntryDraft) (ports.OutboxEntryRecord, error) {
	record, err := f.OutboxStore.Enqueue(ctx, draft)
	if err != nil {
		return record, err
	}
	return record, f.cause
}

func TestManualOutboundConversationImageIsAtomicInPostgres(t *testing.T) {
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
	customerID := uuid.NewString()
	conversationID := uuid.NewString()
	referenceID := uuid.NewString()
	principalID := uuid.NewString()
	mediaAssetID := uuid.NewString()
	now := time.Now().UTC()

	if _, err := adapter.Pool().Exec(ctx, `
		INSERT INTO businesses
			(id, name, slug, status, vertical_type, timezone, default_currency, locale, created_at, updated_at)
		VALUES ($1::uuid, 'Outbound Media Integration', $2, 'active', 'retail', 'Asia/Aden', 'YER', 'ar-YE', $3, $3)
	`, businessID, "outbound-media-"+businessID, now); err != nil {
		t.Fatalf("insert business: %v", err)
	}
	defer func() {
		cleanupCtx := context.Background()
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM outbox_entries WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM message_attachments WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM communication_messages WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM outbound_messages WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM media_assets WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM conversation_references WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM conversations WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM customers WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM channel_connection_capabilities WHERE connection_id = $1::uuid`, connectionID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM channel_connections WHERE business_id = $1::uuid`, businessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM principals WHERE id = $1::uuid`, principalID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM businesses WHERE id = $1::uuid`, businessID)
	}()

	if _, err := adapter.Pool().Exec(ctx, `
		INSERT INTO principals (id, email, display_name, password_hash, status, created_at, updated_at)
		VALUES ($1::uuid, $2, 'Media Test Agent', 'integration-test-hash', 'active', $3, $3)
	`, principalID, "outbound-media-"+principalID+"@example.test", now); err != nil {
		t.Fatalf("insert principal: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `
		INSERT INTO channel_connections
			(id, business_id, provider_ref, channel, provider_account_ref, provider_connection_ref, status, secret_reference, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, 'socialapi', 'whatsapp', 'media-test-account', 'media-test-connection', 'active', 'secret://media-test', $3, $3)
	`, connectionID, businessID, now); err != nil {
		t.Fatalf("insert channel connection: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `
		INSERT INTO channel_connection_capabilities (connection_id, capability, enabled, checked_at, evidence_source)
		VALUES ($1::uuid, 'media_outbound', true, $2, 'socialapi outbound image contract integration test')
	`, connectionID, now); err != nil {
		t.Fatalf("insert outbound media capability: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `
		INSERT INTO customers (id, business_id, profile, contact_points, status, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, '{}'::jsonb, '[]'::jsonb, 'active', $3, $3)
	`, customerID, businessID, now); err != nil {
		t.Fatalf("insert customer: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `
		INSERT INTO conversations
			(id, business_id, customer_id, state, ownership, priority, last_activity_at, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'human_handling', 'human', 'normal', $4, $4, $4)
	`, conversationID, businessID, customerID, now); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err := adapter.Pool().Exec(ctx, `
		INSERT INTO conversation_references
			(id, business_id, conversation_id, system, provider_ref, resource_type, resource_id,
			 connection_id, conversation_kind, is_current, mapping_status, created_at, updated_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'provider', 'socialapi', 'conversation',
		        'media-test-provider-conversation', $4::uuid, 'dm', true, 'active', $5, $5)
	`, referenceID, businessID, conversationID, connectionID, now); err != nil {
		t.Fatalf("insert conversation reference: %v", err)
	}

	mediaAssets := NewConversationMediaIngestionRepository(adapter)
	pending, err := mediaAssets.CreatePendingIngestion(ctx, ports.PendingConversationMediaAsset{
		ID: mediaAssetID, BusinessID: businessID, StorageProvider: "backblaze",
		StorageKey: "conversation/" + businessID + "/" + mediaAssetID + ".jpg", Kind: "image",
	})
	if err != nil || pending.Status != "pending_ingestion" {
		t.Fatalf("create pending media asset: asset=%#v err=%v", pending, err)
	}
	digest := sha256.Sum256([]byte("verified-image-fixture"))
	ready, err := mediaAssets.MarkImageReady(ctx, businessID, mediaAssetID, ports.VerifiedImage{
		MimeType: "image/jpeg", SizeBytes: 22, SHA256: digest[:], Width: 1, Height: 1,
	})
	if err != nil || ready.Status != "ready" || ready.Purpose != "conversation_attachment" {
		t.Fatalf("mark media asset ready: asset=%#v err=%v", ready, err)
	}

	outbound := NewOutboundMessageRepository(adapter)
	messages := NewMessageRepository(adapter)
	attachments := NewMessageAttachmentRepository(adapter)
	outbox := NewPostgresOutboxStore(adapter)
	service := services.ManualOutboundMessageService{
		References:              NewConversationReferenceRepository(adapter),
		Connections:             NewChannelConnectionRepository(adapter),
		MediaAttachmentsEnabled: true,
		Capabilities:            NewChannelCapabilityRepository(adapter),
		MediaAssets:             mediaAssets,
		Attachments:             attachments,
		Outbound:                outbound,
		Outbox:                  outboundMediaFailingOutbox{OutboxStore: outbox, cause: errors.New("forced outbox failure")},
		Messages:                messages,
		Inbox:                   outboundMediaIntegrationInbox{},
		Transactions:            adapter,
	}
	newCommand := func(key string) commands.CreateOutboundMessageCommand {
		return commands.CreateOutboundMessageCommand{
			Meta: commands.CommandMeta{
				Actor: commands.ActorContext{
					BusinessID: commands.BusinessID(businessID), PrincipalID: commands.PrincipalID(principalID), Role: "owner", Permissions: []string{"*"},
				},
				IdempotencyKey: key,
			},
			ConversationID:     commands.ConversationID(conversationID),
			Text:               "هذه صورة مرفقة",
			AttachmentMediaIDs: []string{mediaAssetID},
		}
	}

	// The fake returns an error only after the real PostgreSQL Outbox INSERT.
	// The enclosing transaction must roll back the outbound message, public
	// communication message, MessageAttachment relation, and Outbox row.
	if _, err := service.Handle(ctx, newCommand("rollback-outbound-media")); err == nil {
		t.Fatal("expected injected outbox failure")
	}
	for _, table := range []string{"outbound_messages", "communication_messages", "message_attachments", "outbox_entries"} {
		var count int
		query := "SELECT count(*) FROM " + table + " WHERE business_id = $1::uuid"
		if err := adapter.Pool().QueryRow(ctx, query, businessID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("transaction rollback left %s rows: count=%d err=%v", table, count, err)
		}
	}

	// A second request with a fresh idempotency key commits all four records.
	service.Outbox = outbox
	result, err := service.Handle(ctx, newCommand("commit-outbound-media"))
	if err != nil {
		t.Fatalf("create outbound image message: %v", err)
	}
	if result.Message.ID == "" || result.Message.ContentType != "mixed" || len(result.Message.Attachments) != 1 ||
		result.Message.Attachments[0].Type != "image" || result.Message.Attachments[0].MediaID != mediaAssetID ||
		result.Message.Attachments[0].Status != "ready" {
		t.Fatalf("unexpected committed message projection: %#v", result.Message)
	}

	var outboundMessageID string
	if err := adapter.Pool().QueryRow(ctx, `
		SELECT outbound_message_id::text FROM outbox_entries
		WHERE business_id = $1::uuid AND dedupe_key = 'manual-outbound:commit-outbound-media'
	`, businessID).Scan(&outboundMessageID); err != nil {
		t.Fatalf("read committed outbox obligation: %v", err)
	}
	resolved, err := attachments.ListForOutboundMessage(ctx, businessID, outboundMessageID)
	if err != nil || len(resolved) != 1 {
		t.Fatalf("resolve committed outbound attachment: rows=%#v err=%v", resolved, err)
	}
	item := resolved[0]
	if item.BusinessID != businessID || item.OutboundMessageID != outboundMessageID ||
		item.CommunicationMessageID != string(result.Message.ID) || item.MediaAssetID != mediaAssetID ||
		item.Status != "ready" || item.AssetPurpose != "conversation_attachment" ||
		item.AssetStatus != "ready" || item.AssetDeleted ||
		item.StorageProvider != "backblaze" || item.StorageKey != ready.StorageKey ||
		item.MimeType != "image/jpeg" {
		t.Fatalf("outbound attachment lost tenant or pinned storage metadata: %#v", item)
	}
}

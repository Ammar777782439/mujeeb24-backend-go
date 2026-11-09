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
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM message_attachments WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM communication_messages WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM conversation_references WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM external_identities WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM conversations WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM customers WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM inbound_event_ledger WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM channel_connections WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM inbound_webhook_payloads WHERE provider_ref = 'socialapi' AND delivery_id LIKE 'provider-test-%'`)
		_, _ = adapter.Pool().Exec(cleanupCtx, `DELETE FROM media_assets WHERE business_id IN ($1::uuid, $2::uuid)`, businessID, otherBusinessID)
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
	if _, err := rawStore.Put(ctx, "socialapi", "provider-test-1", []byte(`{"event":"different"}`)); !IsRepositoryKind(err, RepositoryConflict) {
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

	materializer := NewProviderInboundStore(adapter)
	draft := ports.ProviderInboundDraft{InboundEventID: eventRecord.ID, BusinessID: businessID, ConnectionID: connectionID, ProviderRef: "socialapi", ProviderEventID: "provider-event-1", Channel: "whatsapp", ProviderAccountRef: "account-provider-a", ProviderConversationID: "provider-conversation-1", ExternalUserID: "provider-user-1", ProviderMessageID: "provider-message-1", EventType: "interaction_received", InteractionKind: "dm", Text: "مرحبا", MediaStorageProvider: "r2", Attachments: []ports.ProviderInboundAttachmentDraft{{ProviderMediaReference: "socialapi:v1:cHJvdmlkZXItZXZlbnQtMQ:0", Kind: "image", SortOrder: 0}, {ProviderMediaReference: "socialapi:v1:cHJvdmlkZXItZXZlbnQtMQ:1", Kind: "file", SortOrder: 1}}, ExternalCreatedAt: providerInboundTimePtr(base), ReceivedAt: base, RawPayloadReference: raw.Reference, PayloadHash: raw.SHA256}
	first, err := materializer.Materialize(ctx, draft)
	if err != nil {
		t.Fatalf("first materialize: %v", err)
	}
	if first.Duplicate || first.Ignored || first.CustomerID == "" || first.ConversationID == "" || first.ConversationReferenceID == "" || first.CommunicationMessageID == "" || first.InboundEventID != eventID {
		t.Fatalf("unexpected first materialization: %#v", first)
	}
	// Conversation attachments are separate from catalog_media_links and
	// share only the tenant-owned MediaAsset storage metadata.
	mediaAssetID := uuid.NewString()
	mediaAssets := NewConversationMediaIngestionRepository(adapter)
	attachmentID := uuid.NewString()
	providerMediaRef := "provider-media-opaque-1"
	attachments := NewMessageAttachmentRepository(adapter)
	var pendingAsset ports.MediaAsset
	var createdAttachment ports.MessageAttachmentRecord
	err = adapter.Within(ctx, func(txCtx context.Context) error {
		var txErr error
		pendingAsset, txErr = mediaAssets.CreatePendingIngestion(txCtx, ports.PendingConversationMediaAsset{
			ID: mediaAssetID, BusinessID: businessID, StorageProvider: "r2",
			StorageKey: "conversation/" + businessID + "/" + mediaAssetID, Kind: "image",
		})
		if txErr != nil {
			return txErr
		}
		createdAttachment, txErr = attachments.Create(txCtx, ports.MessageAttachmentDraft{
			ID: attachmentID, BusinessID: businessID,
			CommunicationMessageID: first.CommunicationMessageID, MediaAssetID: mediaAssetID,
			Kind: "image", Caption: "صورة العميل", SortOrder: 2,
			SourceProviderMediaRef: &providerMediaRef, CreatedAt: base.Add(time.Second),
		})
		return txErr
	})
	if err != nil {
		t.Fatalf("create pending media asset and message attachment atomically: %v", err)
	}
	if pendingAsset.Purpose != "conversation_attachment" || pendingAsset.Kind != "image" || pendingAsset.Status != "pending_ingestion" {
		t.Fatalf("unexpected pending media asset: %#v", pendingAsset)
	}
	if createdAttachment.Status != "pending_ingestion" || createdAttachment.MediaAssetID != mediaAssetID {
		t.Fatalf("unexpected pending message attachment: %#v", createdAttachment)
	}
	listedAttachments, err := attachments.ListByMessage(ctx, businessID, first.CommunicationMessageID)
	if err != nil || len(listedAttachments) != 3 || listedAttachments[0].Kind != "image" ||
		listedAttachments[1].Kind != "file" || listedAttachments[2].ID != attachmentID {
		t.Fatalf("list materialized and manually-created message attachments: items=%#v err=%v", listedAttachments, err)
	}

	// Pending attachments are claimed with a bounded lease. A second worker
	// cannot claim the same rows until the lease expires or they are released.
	claimed, err := attachments.ClaimPendingIngestions(ctx, 10, time.Minute)
	// This worker slice intentionally claims only the first WhatsApp image
	// because SocialAPI history exposes one attachment per message. The file
	// placeholder and later attachment positions remain pending.
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim supported first-position WhatsApp image: items=%#v err=%v", claimed, err)
	}
	for _, item := range claimed {
		if item.BusinessID != businessID || item.ProviderConversationID != "provider-conversation-1" ||
			item.Channel != "whatsapp" || item.Kind != "image" ||
			item.ProviderMediaReference == "" || item.StorageProvider != "r2" ||
			!strings.HasPrefix(item.StorageKey, "conversation/"+businessID+"/") || item.AttemptCount != 1 {
			t.Fatalf("claimed ingestion leaked or lost scoped fields: %#v", item)
		}
	}
	if again, claimErr := attachments.ClaimPendingIngestions(ctx, 10, time.Minute); claimErr != nil || len(again) != 0 {
		t.Fatalf("leased attachments were claimed twice: items=%#v err=%v", again, claimErr)
	}
	if err := attachments.ReleaseForRetry(ctx, businessID, claimed[0].AttachmentID, time.Now().UTC().Add(time.Hour), "https://bad.example"); err == nil {
		t.Fatal("unsafe error code was accepted for ingestion retry")
	}
	for _, item := range claimed {
		if err := attachments.ReleaseForRetry(ctx, item.BusinessID, item.AttachmentID, time.Now().UTC().Add(time.Hour), "socialapi_media_not_ready"); err != nil {
			t.Fatalf("release attachment for retry: %v", err)
		}
	}
	if delayed, claimErr := attachments.ClaimPendingIngestions(ctx, 10, time.Minute); claimErr != nil || len(delayed) != 0 {
		t.Fatalf("future retry rows were claimed early: items=%#v err=%v", delayed, claimErr)
	}
	// Simulate a worker crash: an expired lease is reclaimable and increments
	// the attempt counter, while the attachment remains tenant-scoped.
	if _, err := adapter.Pool().Exec(ctx, `
		UPDATE message_attachments
		SET ingestion_next_attempt_at = now() - interval '1 second',
		    ingestion_lease_until = now() - interval '1 second'
		WHERE business_id = $1::uuid AND id = $2::uuid
	`, businessID, claimed[0].AttachmentID); err != nil {
		t.Fatalf("expire media ingestion lease: %v", err)
	}
	reclaimed, err := attachments.ClaimPendingIngestions(ctx, 10, time.Minute)
	if err != nil || len(reclaimed) != 1 || reclaimed[0].AttachmentID != claimed[0].AttachmentID || reclaimed[0].AttemptCount != 2 {
		t.Fatalf("reclaim expired media ingestion lease: items=%#v err=%v", reclaimed, err)
	}
	if _, err := attachments.MarkFailed(ctx, businessID, reclaimed[0].AttachmentID); err != nil {
		t.Fatalf("mark terminal ingestion failure: %v", err)
	}
	failedAsset, err := mediaAssets.GetAsset(ctx, businessID, reclaimed[0].MediaAssetID)
	if err != nil || failedAsset.Status != "rejected" {
		t.Fatalf("terminal attachment failure must reject its pending MediaAsset atomically: asset=%#v err=%v", failedAsset, err)
	}

	// A message and its media asset must belong to the same business.
	_, err = attachments.Create(ctx, ports.MessageAttachmentDraft{
		ID: uuid.NewString(), BusinessID: otherBusinessID,
		CommunicationMessageID: first.CommunicationMessageID, MediaAssetID: mediaAssetID,
		Kind: "image", SortOrder: 1, CreatedAt: base.Add(2 * time.Second),
	})
	if err == nil {
		t.Fatal("cross-business message/media attachment was accepted")
	}

	// The attachment cannot become ready before its bytes have been verified
	// and the corresponding MediaAsset has transitioned to ready.
	if _, err := attachments.MarkReady(ctx, businessID, attachmentID); err == nil {
		t.Fatal("pending MediaAsset was allowed to mark its message attachment ready")
	}
	digest := sha256.Sum256([]byte("verified image fixture"))
	var readyAttachment ports.MessageAttachmentRecord
	err = adapter.Within(ctx, func(txCtx context.Context) error {
		if _, txErr := mediaAssets.MarkImageReady(txCtx, businessID, mediaAssetID, ports.VerifiedImage{
			MimeType: "image/png", SizeBytes: 123, SHA256: digest[:], Width: 10, Height: 10,
		}); txErr != nil {
			return txErr
		}
		var txErr error
		readyAttachment, txErr = attachments.MarkReady(txCtx, businessID, attachmentID)
		return txErr
	})
	if err != nil || readyAttachment.Status != "ready" || readyAttachment.MimeType != "image/png" || readyAttachment.SizeBytes != 123 {
		t.Fatalf("mark MediaAsset and message attachment ready atomically: item=%#v err=%v", readyAttachment, err)
	}

	urlReference := "https://provider.example/media/temporary"
	if _, err := attachments.Create(ctx, ports.MessageAttachmentDraft{
		ID: uuid.NewString(), BusinessID: businessID,
		CommunicationMessageID: first.CommunicationMessageID, MediaAssetID: mediaAssetID,
		Kind: "image", SortOrder: 2, SourceProviderMediaRef: &urlReference, CreatedAt: base.Add(4 * time.Second),
	}); err == nil {
		t.Fatal("a provider URL was accepted as a durable opaque media reference")
	}

	// A failed relation insert must roll back the pending MediaAsset created
	// earlier in the same application transaction.
	orphanAssetID := uuid.NewString()
	err = adapter.Within(ctx, func(txCtx context.Context) error {
		if _, txErr := mediaAssets.CreatePendingIngestion(txCtx, ports.PendingConversationMediaAsset{
			ID: orphanAssetID, BusinessID: businessID, StorageProvider: "r2",
			StorageKey: "conversation/" + businessID + "/" + orphanAssetID, Kind: "image",
		}); txErr != nil {
			return txErr
		}
		_, txErr := attachments.Create(txCtx, ports.MessageAttachmentDraft{
			ID: uuid.NewString(), BusinessID: businessID,
			CommunicationMessageID: uuid.NewString(), MediaAssetID: orphanAssetID,
			Kind: "image", SourceProviderMediaRef: &providerMediaRef, CreatedAt: base.Add(5 * time.Second),
		})
		return txErr
	})
	if err == nil {
		t.Fatal("expected invalid message attachment transaction to fail")
	}
	if _, err := mediaAssets.GetAsset(ctx, businessID, orphanAssetID); err == nil {
		t.Fatal("pending MediaAsset survived a failed attachment transaction")
	}

	// Catalog assets are intentionally not accepted as conversation attachments.
	catalogAssetID := uuid.NewString()
	catalogMedia := NewCatalogMediaRepository(adapter)
	if _, err := catalogMedia.CreatePendingAsset(ctx, ports.PendingMediaAsset{
		ID: catalogAssetID, BusinessID: businessID,
		StorageKey:      "catalog/" + businessID + "/" + catalogAssetID,
		UploadExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create catalog asset: %v", err)
	}
	if _, err := catalogMedia.MarkAssetReady(ctx, businessID, catalogAssetID, ports.VerifiedImage{
		MimeType: "image/png", SizeBytes: 123, SHA256: digest[:], Width: 10, Height: 10,
	}); err != nil {
		t.Fatalf("mark catalog asset ready: %v", err)
	}
	if _, err := attachments.Create(ctx, ports.MessageAttachmentDraft{
		ID: uuid.NewString(), BusinessID: businessID,
		CommunicationMessageID: first.CommunicationMessageID, MediaAssetID: catalogAssetID,
		Kind: "image", SortOrder: 1, CreatedAt: base.Add(3 * time.Second),
	}); err == nil {
		t.Fatal("catalog-purpose MediaAsset was accepted as a conversation attachment")
	}
	second, err := materializer.Materialize(ctx, draft)
	if err != nil {
		t.Fatalf("duplicate materialize: %v", err)
	}
	if !second.Duplicate || second.CustomerID != first.CustomerID || second.ConversationID != first.ConversationID || second.ConversationReferenceID != first.ConversationReferenceID || second.CommunicationMessageID != first.CommunicationMessageID || second.InboundEventID != eventID {
		t.Fatalf("duplicate was not idempotent: first=%#v second=%#v", first, second)
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

	// Update conversation to waiting_customer and test reopening on new inbound message
	if _, err := adapter.Pool().Exec(ctx, `UPDATE conversations SET state = 'waiting_customer' WHERE id = $1::uuid`, first.ConversationID); err != nil {
		t.Fatalf("set waiting_customer: %v", err)
	}
	subsequentEventID := uuid.NewString()
	subsequentEventDraft := eventDraft
	subsequentEventDraft.ID = subsequentEventID
	subsequentEventDraft.ProviderEventID = "provider-event-subsequent"
	subsequentEventDraft.ProviderMessageID = stringPointerForProviderInbound("provider-message-subsequent")
	if _, _, err := eventStore.RecordIfAbsent(ctx, subsequentEventDraft); err != nil {
		t.Fatalf("record subsequent event: %v", err)
	}
	subsequentDraft := draft
	subsequentDraft.InboundEventID = subsequentEventID
	subsequentDraft.ProviderEventID = "provider-event-subsequent"
	subsequentDraft.ProviderMessageID = "provider-message-subsequent"
	subsequentDraft.Text = "هل تم الرد؟"
	subsequentResult, err := materializer.Materialize(ctx, subsequentDraft)
	if err != nil {
		t.Fatalf("subsequent materialize: %v", err)
	}
	if subsequentResult.ConversationID != first.ConversationID {
		t.Fatalf("expected same conversation ID %s, got %s", first.ConversationID, subsequentResult.ConversationID)
	}
	var reopenedState string
	if err := adapter.Pool().QueryRow(ctx, `SELECT state FROM conversations WHERE id = $1::uuid`, first.ConversationID).Scan(&reopenedState); err != nil {
		t.Fatalf("get reopened state: %v", err)
	}
	if reopenedState != "open" {
		t.Fatalf("expected reopened state 'open', got %q", reopenedState)
	}

	wrongDraft := draft
	wrongDraft.BusinessID = otherBusinessID
	if _, err := materializer.Materialize(ctx, wrongDraft); !IsRepositoryKind(err, RepositoryConflict) {
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

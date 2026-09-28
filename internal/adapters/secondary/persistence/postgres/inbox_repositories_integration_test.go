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

func TestInboxRepositoriesAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if _, err := database.RunMigrations(ctx, dsn, time.Now().UTC()); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	adapter, err := Open(ctx, dsn, DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer adapter.Close()

	pool := adapter.Pool()
	businessID, otherBusinessID := uuid.NewString(), uuid.NewString()
	principalID, customerID, connectionID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	conversationID, referenceID := uuid.NewString(), uuid.NewString()
	publicMessageID, privateMessageID := uuid.NewString(), uuid.NewString()
	cleanup := func() {
		cleanupCtx := context.Background()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM conversation_read_cursors WHERE business_id IN ($1::uuid,$2::uuid)`, businessID, otherBusinessID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM canned_replies WHERE business_id IN ($1::uuid,$2::uuid)`, businessID, otherBusinessID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM communication_messages WHERE business_id IN ($1::uuid,$2::uuid)`, businessID, otherBusinessID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM conversation_references WHERE business_id IN ($1::uuid,$2::uuid)`, businessID, otherBusinessID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM conversations WHERE business_id IN ($1::uuid,$2::uuid)`, businessID, otherBusinessID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM channel_connections WHERE business_id IN ($1::uuid,$2::uuid)`, businessID, otherBusinessID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM customers WHERE business_id IN ($1::uuid,$2::uuid)`, businessID, otherBusinessID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM business_memberships WHERE business_id IN ($1::uuid,$2::uuid)`, businessID, otherBusinessID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM principals WHERE id=$1::uuid`, principalID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM businesses WHERE id IN ($1::uuid,$2::uuid)`, businessID, otherBusinessID)
	}
	cleanup()
	defer cleanup()

	for _, row := range []struct{ id, slug string }{{businessID, "inbox-runtime-" + businessID}, {otherBusinessID, "inbox-runtime-" + otherBusinessID}} {
		if _, err := pool.Exec(ctx, `INSERT INTO businesses (id,name,slug,status,vertical_type,timezone,default_currency,locale,created_at,updated_at) VALUES ($1::uuid,'Inbox Runtime',$2,'active','retail','Asia/Aden','YER','ar-YE',now(),now())`, row.id, row.slug); err != nil {
			t.Fatalf("business %s: %v", row.id, err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO principals (id,email,display_name,password_hash,status,created_at,updated_at) VALUES ($1::uuid,'inbox-runtime-`+principalID+`@example.invalid','Inbox Agent','not-a-real-hash','active',now(),now())`, principalID); err != nil {
		t.Fatalf("principal: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO business_memberships (business_id,principal_id,role,permissions,status,created_at,updated_at) VALUES ($1::uuid,$2::uuid,'agent','[]','active',now(),now())`, businessID, principalID); err != nil {
		t.Fatalf("membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO customers (id,business_id,profile,contact_points,status,created_at,updated_at) VALUES ($1::uuid,$2::uuid,'{}','[]','active',now(),now())`, customerID, businessID); err != nil {
		t.Fatalf("customer: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO channel_connections (id,business_id,provider_ref,channel,provider_connection_ref,status,secret_reference,created_at,updated_at) VALUES ($1::uuid,$2::uuid,'socialapi','whatsapp','inbox-connection','active','test-secret-ref',now(),now())`, connectionID, businessID); err != nil {
		t.Fatalf("connection: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO conversations (id,business_id,customer_id,state,ownership,priority,last_activity_at,created_at,updated_at) VALUES ($1::uuid,$2::uuid,$3::uuid,'open','human','normal',now(),now(),now())`, conversationID, businessID, customerID); err != nil {
		t.Fatalf("conversation: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO conversation_references (id,business_id,conversation_id,system,provider_ref,resource_type,resource_id,connection_id,conversation_kind,is_current,mapping_status,created_at,updated_at) VALUES ($1::uuid,$2::uuid,$3::uuid,'provider','socialapi','conversation','inbox-provider-conversation',$4::uuid,'dm',true,'active',now(),now())`, referenceID, businessID, conversationID, connectionID); err != nil {
		t.Fatalf("reference: %v", err)
	}
	publicText, privateText := "public", "private"
	messages := NewMessageRepository(adapter)
	for _, draft := range []ports.CommunicationMessageDraft{
		{ID: publicMessageID, BusinessID: businessID, ConversationReferenceID: referenceID, Direction: "inbound", Origin: "customer", Transport: "provider", ProviderMessageID: inboxStringPointer("inbox-public-message"), ContentType: "text", TextContent: &publicText, ContentReference: "inbox-public-content", Visibility: "public", OccurredAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC), CreatedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)},
		{ID: privateMessageID, BusinessID: businessID, ConversationReferenceID: referenceID, Direction: "outbound", Origin: "human", Transport: "mujeeb", ContentType: "text", TextContent: &privateText, ContentReference: "inbox-private-content", Visibility: "private", OccurredAt: time.Date(2026, 8, 28, 12, 1, 0, 0, time.UTC), CreatedAt: time.Date(2026, 8, 28, 12, 1, 0, 0, time.UTC)},
	} {
		if _, err := messages.Record(ctx, draft); err != nil {
			t.Fatalf("message: %v", err)
		}
	}

	readCursors := NewConversationReadCursorRepository(adapter)
	cursor, err := readCursors.MarkRead(ctx, businessID, conversationID, principalID, time.Date(2026, 8, 28, 12, 2, 0, 0, time.UTC))
	if err != nil || cursor.LastReadMessageID == nil || *cursor.LastReadMessageID != publicMessageID {
		t.Fatalf("read cursor=%#v err=%v", cursor, err)
	}
	if _, err := readCursors.MarkRead(ctx, otherBusinessID, conversationID, principalID, time.Now().UTC()); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("cross-tenant read cursor error=%v", err)
	}

	replies := NewCannedReplyRepository(adapter)
	replyID := uuid.NewString()
	created, err := replies.Create(ctx, ports.CannedReplyCreate{ID: replyID, BusinessID: businessID, Title: "Welcome", Shortcut: "welcome", Body: "أهلًا بك", Status: "active", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
	if err != nil || created.ResourceVersion != 1 {
		t.Fatalf("create reply=%#v err=%v", created, err)
	}
	if _, err := replies.Create(ctx, ports.CannedReplyCreate{ID: uuid.NewString(), BusinessID: businessID, Title: "Duplicate", Shortcut: "welcome", Body: "مكرر", Status: "active", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}); !IsRepositoryKind(err, RepositoryConflict) {
		t.Fatalf("duplicate shortcut error=%v", err)
	}
	updatedTitle := "Welcome updated"
	updated, err := replies.Update(ctx, ports.CannedReplyUpdate{BusinessID: businessID, CannedReplyID: replyID, ExpectedVersion: 1, Title: &updatedTitle, UpdatedAt: time.Now().UTC()})
	if err != nil || updated.Title != updatedTitle || updated.ResourceVersion != 2 {
		t.Fatalf("update reply=%#v err=%v", updated, err)
	}
	if _, err := replies.Update(ctx, ports.CannedReplyUpdate{BusinessID: businessID, CannedReplyID: replyID, ExpectedVersion: 1, Title: &updatedTitle, UpdatedAt: time.Now().UTC()}); !IsRepositoryKind(err, RepositoryStale) {
		t.Fatalf("stale reply update error=%v", err)
	}
	page, err := replies.List(ctx, businessID, "active", 10, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != replyID {
		t.Fatalf("list replies=%#v err=%v", page, err)
	}
	if _, err := replies.GetByID(ctx, otherBusinessID, replyID); !IsRepositoryKind(err, RepositoryNotFound) {
		t.Fatalf("cross-tenant reply error=%v", err)
	}
}

func inboxStringPointer(value string) *string { return &value }

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

func TestConversationLabelsAndPrivateVisibilityAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	businessID := uuid.NewString()
	customerID := uuid.NewString()
	connectionID := uuid.NewString()
	conversationID := uuid.NewString()
	referenceID := uuid.NewString()
	messageID := uuid.NewString()
	slug := "labels-runtime-" + businessID
	_, _ = pool.Exec(ctx, `DELETE FROM businesses WHERE id=$1::uuid`, businessID)
	if _, err := pool.Exec(ctx, `INSERT INTO businesses (id,name,slug,status,vertical_type,timezone,default_currency,locale,created_at,updated_at) VALUES ($1::uuid,'Labels Runtime',$2,'active','retail','Asia/Aden','YER','ar-YE',now(),now())`, businessID, slug); err != nil {
		t.Fatalf("business: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM businesses WHERE id=$1::uuid`, businessID)
	if _, err := pool.Exec(ctx, `INSERT INTO customers (id,business_id,profile,contact_points,status,created_at,updated_at) VALUES ($1::uuid,$2::uuid,'{}','[]','active',now(),now())`, customerID, businessID); err != nil {
		t.Fatalf("customer: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO channel_connections (id,business_id,provider_ref,channel,provider_connection_ref,status,secret_reference,created_at,updated_at) VALUES ($1::uuid,$2::uuid,'socialapi','whatsapp','conn-private-note','active','test-secret-ref',now(),now())`, connectionID, businessID); err != nil {
		t.Fatalf("connection: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO conversations (id,business_id,customer_id,state,ownership,priority,last_activity_at,created_at,updated_at) VALUES ($1::uuid,$2::uuid,$3::uuid,'open','none','normal',now(),now(),now())`, conversationID, businessID, customerID); err != nil {
		t.Fatalf("conversation: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO conversation_references (id,business_id,conversation_id,system,provider_ref,resource_type,resource_id,connection_id,conversation_kind,is_current,mapping_status,created_at,updated_at) VALUES ($1::uuid,$2::uuid,$3::uuid,'provider','socialapi','conversation','provider-conversation-private',$4::uuid,'dm',true,'active',now(),now())`, referenceID, businessID, conversationID, connectionID); err != nil {
		t.Fatalf("reference: %v", err)
	}
	labels := NewConversationLabelRepository(adapter)
	if err := labels.Apply(ctx, businessID, conversationID, []string{" VIP ", "vip", "Follow-Up"}, []string{}); err != nil {
		t.Fatalf("add labels: %v", err)
	}
	values, err := labels.List(ctx, businessID, conversationID)
	if err != nil || len(values) != 2 || values[0] != "follow-up" || values[1] != "vip" {
		t.Fatalf("labels: %#v err=%v", values, err)
	}
	if err := labels.Apply(ctx, businessID, conversationID, []string{}, []string{"VIP"}); err != nil {
		t.Fatalf("remove label: %v", err)
	}
	values, err = labels.List(ctx, businessID, conversationID)
	if err != nil || len(values) != 1 || values[0] != "follow-up" {
		t.Fatalf("labels after remove: %#v err=%v", values, err)
	}
	note := "internal only"
	messages := NewMessageRepository(adapter)
	record, err := messages.Record(ctx, ports.CommunicationMessageDraft{ID: messageID, BusinessID: businessID, ConversationReferenceID: referenceID, Direction: "outbound", Origin: "human", Transport: "mujeeb", ContentType: "text", TextContent: &note, ContentReference: "private-note:" + messageID, Visibility: "private", OccurredAt: time.Now().UTC(), CreatedAt: time.Now().UTC()})
	if err != nil || record.Visibility != "private" {
		t.Fatalf("record private note: %#v err=%v", record, err)
	}
	page, err := messages.ListByConversation(ctx, businessID, conversationID, 50, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].Visibility != "private" {
		t.Fatalf("list private timeline: %#v err=%v", page, err)
	}
}

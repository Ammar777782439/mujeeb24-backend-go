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

// Real PostgreSQL test, not a mocked service: both messages and sales data
// must remain inaccessible when the same ID is requested from another tenant.
func TestMerchantAssistSourcesTenantIsolationPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
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
	businessA, businessB := uuid.NewString(), uuid.NewString()
	customerA, customerB := uuid.NewString(), uuid.NewString()
	conversationID, channelID, referenceID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	leadID, otherLeadID := uuid.NewString(), uuid.NewString()
	for _, id := range []string{businessA, businessB} {
		_, err = pool.Exec(ctx, `INSERT INTO businesses (id,name,slug,status,vertical_type,timezone,default_currency,locale,created_at,updated_at)
			VALUES ($1::uuid,'AI Isolation Test',$2,'active','retail','Asia/Aden','YER','ar-YE',now(),now())`, id, "assist-tenant-"+id)
		if err != nil {
			t.Fatalf("insert business: %v", err)
		}
		defer pool.Exec(context.Background(), `DELETE FROM businesses WHERE id=$1::uuid`, id)
	}
	for _, ids := range [][2]string{{customerA, businessA}, {customerB, businessB}} {
		if _, err = pool.Exec(ctx, `INSERT INTO customers (id,business_id,profile,contact_points,status,created_at,updated_at)
			VALUES ($1::uuid,$2::uuid,'{}'::jsonb,'[]'::jsonb,'active',now(),now())`, ids[0], ids[1]); err != nil {
			t.Fatalf("insert customer: %v", err)
		}
	}
	if _, err = pool.Exec(ctx, `INSERT INTO channel_connections
		(id,business_id,provider_ref,channel,provider_connection_ref,status,secret_reference,created_at,updated_at)
		VALUES ($1::uuid,$2::uuid,'socialapi','whatsapp',$3,'active','integration-only-secret-ref',now(),now())`, channelID, businessA, "assist-"+channelID); err != nil {
		t.Fatalf("insert connection: %v", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO conversations
		(id,business_id,customer_id,state,ownership,priority,last_activity_at,created_at,updated_at)
		VALUES ($1::uuid,$2::uuid,$3::uuid,'open','none','normal',now(),now(),now())`, conversationID, businessA, customerA); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO conversation_references
		(id,business_id,conversation_id,system,provider_ref,resource_type,resource_id,connection_id,conversation_kind,is_current,mapping_status,created_at,updated_at)
		VALUES ($1::uuid,$2::uuid,$3::uuid,'provider','socialapi','conversation',$4,$5::uuid,'dm',true,'active',now(),now())`, referenceID, businessA, conversationID, "remote-"+referenceID, channelID); err != nil {
		t.Fatalf("insert reference: %v", err)
	}
	messages := NewMessageRepository(adapter)
	publicText := "Public merchant inquiry"
	privateText := "PRIVATE MUST NOT BE PROMPTED"
	for _, m := range []struct {
		text       string
		visibility string
	}{{publicText, "public"}, {privateText, "private"}} {
		_, err = messages.Record(ctx, ports.CommunicationMessageDraft{
			ID: uuid.NewString(), BusinessID: businessA, ConversationReferenceID: referenceID,
			Direction: "outbound", Origin: "human", Transport: "mujeeb", ContentType: "text",
			TextContent: &m.text, Visibility: m.visibility, OccurredAt: time.Now().UTC(), CreatedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("record message: %v", err)
		}
	}
	page, err := messages.ListByConversation(ctx, businessA, conversationID, 25, "")
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("own tenant cannot read its messages: %#v err=%v", page, err)
	}
	privateSeen := false
	for _, message := range page.Items {
		if message.Visibility == "private" {
			privateSeen = true
		}
	}
	if !privateSeen {
		t.Fatal("fixture lost private note")
	}
	alienMessages, err := messages.ListByConversation(ctx, businessB, conversationID, 25, "")
	if err == nil && len(alienMessages.Items) != 0 {
		t.Fatal("cross-tenant messages returned")
	}
	if _, err = pool.Exec(ctx, `INSERT INTO leads(id,business_id,customer_id,status,qualification_state,qualification_context,created_by,created_at,updated_at)
		VALUES ($1::uuid,$2::uuid,$3::uuid,'new','new','{}'::jsonb,'human',now(),now()),
		($4::uuid,$5::uuid,$6::uuid,'new','new','{}'::jsonb,'human',now(),now())`,
		leadID, businessA, customerA, otherLeadID, businessB, customerB); err != nil {
		t.Fatalf("insert leads: %v", err)
	}
	leads := NewLeadRepository(adapter)
	if _, err = leads.Get(ctx, businessA, leadID); err != nil {
		t.Fatalf("own lead missing: %v", err)
	}
	if _, err = leads.Get(ctx, businessB, leadID); err == nil {
		t.Fatal("lead belonging to business A returned under business B")
	}
	if _, err = leads.Get(ctx, businessA, otherLeadID); err == nil {
		t.Fatal("lead belonging to business B returned under business A")
	}
	attr, err := leads.ListAttributions(ctx, businessB, leadID, 5, "")
	if err == nil && len(attr.Items) > 0 {
		t.Fatal("cross-tenant attributions leaked")
	}
	score, err := leads.ListScores(ctx, businessB, leadID, 5, "")
	if err == nil && len(score.Items) > 0 {
		t.Fatal("cross-tenant scores leaked")
	}
}

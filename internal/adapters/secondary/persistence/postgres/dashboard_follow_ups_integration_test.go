package postgres

import (
	"context"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
	"github.com/google/uuid"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDashboardFollowUpsAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := database.RunMigrations(ctx, dsn, now); err != nil {
		t.Fatal(err)
	}
	config := DefaultPoolConfig()
	config.MaxConns, config.MinConns = 1, 0
	adapter, err := Open(ctx, dsn, config)
	if err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()
	pool := adapter.Pool()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("query %s: %v", sql, err)
		}
	}
	business, foreign, agent, other := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, b := range []string{business, foreign} {
		exec(`INSERT INTO businesses(id,name,slug,status,vertical_type,timezone,default_currency,locale,created_at,updated_at) VALUES($1::uuid,'Followups',$1::text,'active','retail','Asia/Aden','YER','ar-YE',now(),now())`, b)
	}
	for _, p := range []string{agent, other} {
		exec(`INSERT INTO principals(id,email,display_name,password_hash,status,created_at,updated_at) VALUES($1::uuid,$1::text||'@example.invalid','موظف المتجر','test-only','active',now(),now())`, p)
		exec(`INSERT INTO business_memberships(business_id,principal_id,role,permissions,status,created_at,updated_at) VALUES($1,$2,'agent','[]','active',now(),now())`, business, p)
	}
	connection := uuid.NewString()
	exec(`INSERT INTO channel_connections(id,business_id,provider_ref,channel,provider_connection_ref,status,secret_reference,created_at,updated_at) VALUES($1::uuid,$2::uuid,'socialapi','facebook',$1::text,'active','test-only',now(),now())`, connection, business)
	foreignConnection := uuid.NewString()
	exec(`INSERT INTO channel_connections(id,business_id,provider_ref,channel,provider_connection_ref,status,secret_reference,created_at,updated_at) VALUES($1::uuid,$2::uuid,'socialapi','facebook',$1::text,'active','test-only',now(),now())`, foreignConnection, foreign)
	type seeded struct{ lead, customer, conv, ref string }
	seed := func(b, name, state, ownership, assignee, status string) seeded {
		t.Helper()
		x := seeded{uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()}
		exec(`INSERT INTO customers(id,business_id,profile,contact_points,status,created_at,updated_at) VALUES($1,$2,jsonb_build_object('display_name',$3::text),'[]','active',now(),now())`, x.customer, b, name)
		exec(`INSERT INTO conversations(id,business_id,customer_id,state,ownership,assignment_reference,priority,last_activity_at,created_at,updated_at) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),'normal',now(),now(),now())`, x.conv, b, x.customer, state, ownership, assignee)
		conn := connection
		if b != business {
			conn = foreignConnection
		}
		exec(`INSERT INTO conversation_references(id,business_id,conversation_id,system,provider_ref,resource_type,resource_id,connection_id,conversation_kind,is_current,mapping_status,created_at,updated_at) VALUES($1::uuid,$2::uuid,$3::uuid,'provider','socialapi','conversation',$1::text,$4::uuid,'dm',true,'active',now(),now())`, x.ref, b, x.conv, conn)
		exec(`INSERT INTO leads(id,business_id,customer_id,status,created_by,source_conversation_reference_id,lost_reason,created_at,updated_at) VALUES($1,$2,$3,$4,'human',$5,CASE WHEN $4='lost' THEN 'test reason' ELSE NULL END,now(),now())`, x.lead, b, x.customer, status, x.ref)
		return x
	}
	mine := seed(business, "أحمد علي", "waiting_human", "human", agent, "interested")
	hidden := seed(business, "عميل موظف آخر", "waiting_human", "human", other, "qualified")
	seed(foreign, "متجر آخر", "waiting_human", "none", "", "new")
	seed(business, "فرصة مكتملة", "open", "human", agent, "won")
	seed(business, "فرصة خسارة", "open", "human", agent, "lost")
	seed(business, "يتابعه مجيب", "open", "ai", "", "new")
	seed(business, "ينتظر العميل", "waiting_customer", "human", agent, "new")
	// Duplicate leads must not repeat the same customer in the five-row dashboard.
	exec(`INSERT INTO leads(id,business_id,customer_id,status,created_by,source_conversation_reference_id,created_at,updated_at) VALUES($1,$2,$3,'new','human',$4,now(),now())`, uuid.NewString(), business, mine.customer, mine.ref)
	public, private := "متوفر اللون الأسود؟", "PRIVATE INTERNAL NOTE"
	messages := NewMessageRepository(adapter)
	for i, text := range []string{public, private} {
		visibility, origin, direction := "public", "customer", "inbound"
		if i == 1 {
			visibility, origin, direction = "private", "human", "outbound"
		}
		_, err := messages.Record(ctx, ports.CommunicationMessageDraft{ID: uuid.NewString(), BusinessID: business, ConversationReferenceID: mine.ref, Direction: direction, Origin: origin, Transport: "mujeeb", ContentType: "text", TextContent: &text, ContentReference: uuid.NewString(), Visibility: visibility, OccurredAt: now.Add(time.Duration(i) * time.Minute), CreatedAt: now.Add(time.Duration(i) * time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
	}
	repo := NewDashboardRepository(adapter)
	rows, err := repo.ListFollowUps(ctx, business, ports.ConversationAccess{PrincipalID: agent}, now, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].CustomerID != mine.customer {
		t.Fatalf("restricted, deduplicated view: %#v", rows)
	}
	if rows[0].LastCustomerMessage == nil || *rows[0].LastCustomerMessage != public || rows[0].AssigneeName == nil || rows[0].Attention != "waiting_human" {
		t.Fatalf("grounded view: %#v", rows[0])
	}
	rows, err = repo.ListFollowUps(ctx, business, ports.ConversationAccess{PrincipalID: agent, Supervisor: true}, now, 6)
	if err != nil || len(rows) != 2 {
		t.Fatalf("supervisor view=%#v err=%v", rows, err)
	}
	// Reassignment removes the row from the original agent on the next read.
	exec(`UPDATE conversations SET assignment_reference=$1 WHERE id=$2`, other, mine.conv)
	rows, err = repo.ListFollowUps(ctx, business, ports.ConversationAccess{PrincipalID: agent}, now, 6)
	if err != nil || len(rows) != 0 {
		t.Fatalf("stale assignment access: %#v %v", rows, err)
	}
	// Only an actual review state or due date interrupts automated handling.
	review := seed(business, "طلب للاعتماد", "open", "human", agent, "new")
	exec(`INSERT INTO sales_workflows(id,business_id,customer_id,conversation_id,lead_id,state,policy_version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'awaiting_review',1,now(),now())`, uuid.NewString(), business, review.customer, review.conv, review.lead)
	due := seed(business, "متابعة مستحقة", "open", "ai", "", "new")
	exec(`UPDATE leads SET next_action_at=$1 WHERE id=$2`, now.Add(-time.Hour), due.lead)
	rows, err = repo.ListFollowUps(ctx, business, ports.ConversationAccess{PrincipalID: agent, Supervisor: true}, now, 6)
	if err != nil {
		t.Fatal(err)
	}
	reviewIndex, dueIndex := -1, -1
	for i, x := range rows {
		if x.LeadID == review.lead {
			reviewIndex = i
			if x.WorkflowID == nil || x.Attention != "awaiting_review" {
				t.Fatal("review must be backed by a stored workflow")
			}
		}
		if x.LeadID == due.lead {
			dueIndex = i
			if x.Attention != "follow_up_due" {
				t.Fatal("stored due date missing")
			}
		}
	}
	if reviewIndex < 0 || dueIndex <= reviewIndex {
		t.Fatalf("review and due ordering: %#v", rows)
	}
	// A same-tenant attribution pointing at a different customer cannot supply context.
	mismatched := seed(business, "مصدر غير مطابق", "open", "human", agent, "new")
	exec(`UPDATE leads SET source_conversation_reference_id=$1 WHERE id=$2`, hidden.ref, mismatched.lead)
	rows, err = repo.ListFollowUps(ctx, business, ports.ConversationAccess{PrincipalID: agent, Supervisor: true}, now, 6)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range rows {
		if x.LeadID == mismatched.lead && (x.ConversationID != nil || x.LastCustomerMessage != nil) {
			t.Fatal("another customer conversation supplied context")
		}
		if x.LeadID == mine.lead && x.LastCustomerMessage != nil && strings.Contains(*x.LastCustomerMessage, private) {
			t.Fatal("private message leaked")
		}
	}
}

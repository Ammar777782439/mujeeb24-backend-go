package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/database"
)

func TestConversationRuntimeRepositoryAgainstPostgres(t *testing.T) {
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
	const businessA = "00000000-0000-0000-0000-000000000140"
	const businessB = "00000000-0000-0000-0000-000000000141"
	const customerA = "00000000-0000-0000-0000-000000000142"
	const conversationA = "00000000-0000-0000-0000-000000000143"
	const conversationB = "00000000-0000-0000-0000-000000000144"
	for _, id := range []string{businessA, businessB} {
		_, _ = pool.Exec(ctx, `DELETE FROM businesses WHERE id=$1::uuid`, id)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO businesses (id,name,slug,status,vertical_type,timezone,default_currency,locale,created_at,updated_at) VALUES ($1::uuid,'Conversation A','conversation-a','active','retail','Asia/Aden','YER','ar-YE',now(),now()),($2::uuid,'Conversation B','conversation-b','active','retail','Asia/Aden','YER','ar-YE',now(),now())`, businessA, businessB); err != nil {
		t.Fatalf("businesses: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO customers (id,business_id,profile,contact_points,status,created_at,updated_at) VALUES ($1::uuid,$2::uuid,'{}','[]','active',now(),now())`, customerA, businessA); err != nil {
		t.Fatalf("customer: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO conversations (id,business_id,customer_id,state,ownership,priority,last_activity_at,created_at,updated_at) VALUES ($1::uuid,$2::uuid,$3::uuid,'open','none','normal',now()-interval '1 minute',now(),now()),($4::uuid,$2::uuid,$3::uuid,'waiting_human','none','high',now(),now(),now())`, conversationA, businessA, customerA, conversationB); err != nil {
		t.Fatalf("conversations: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM businesses WHERE id IN ($1::uuid,$2::uuid)`, businessA, businessB)
	repo := NewConversationRepository(adapter)
	page, err := repo.List(ctx, businessA, "", "", "", nil, 1, "")
	if err != nil || len(page.Items) != 1 || !page.HasMore {
		t.Fatalf("list page: %#v err=%v", page, err)
	}
	next, err := repo.List(ctx, businessA, "", "", "", nil, 1, page.NextCursor)
	if err != nil || len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
		t.Fatalf("keyset next: %#v err=%v", next, err)
	}
	if _, err := repo.List(ctx, businessB, "", "", "", nil, 50, ""); err != nil {
		t.Fatalf("other tenant empty list: %v", err)
	}
	record, err := repo.GetByID(ctx, businessA, conversationA)
	if err != nil || record.ResourceVersion != 1 {
		t.Fatalf("get: %#v err=%v", record, err)
	}
	state := "human_handling"
	updated, err := repo.Update(ctx, ports.ConversationUpdate{BusinessID: businessA, ConversationID: conversationA, ExpectedVersion: record.ResourceVersion, State: &state})
	if err != nil || updated.State != state || updated.ResourceVersion != 2 {
		t.Fatalf("update: %#v err=%v", updated, err)
	}
	if _, err := repo.Update(ctx, ports.ConversationUpdate{BusinessID: businessA, ConversationID: conversationA, ExpectedVersion: 1, State: &state}); !IsRepositoryKind(err, RepositoryStale) {
		t.Fatalf("stale: %v", err)
	}
	ownership := "human"
	assignee := "agent-17"
	assigned, err := repo.Update(ctx, ports.ConversationUpdate{BusinessID: businessA, ConversationID: conversationA, ExpectedVersion: updated.ResourceVersion, Ownership: &ownership, AssignmentReference: &assignee})
	if err != nil || assigned.AssignmentReference == nil || *assigned.AssignmentReference != assignee {
		t.Fatalf("assign: %#v err=%v", assigned, err)
	}
	rollbackErr := errors.New("force rollback")
	err = adapter.Within(ctx, func(txCtx context.Context) error {
		priority := "urgent"
		if _, updateErr := repo.Update(txCtx, ports.ConversationUpdate{BusinessID: businessA, ConversationID: conversationA, ExpectedVersion: assigned.ResourceVersion, Priority: &priority}); updateErr != nil {
			return updateErr
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("rollback: %v", err)
	}
	rolled, err := repo.GetByID(ctx, businessA, conversationA)
	if err != nil || rolled.ResourceVersion != assigned.ResourceVersion {
		t.Fatalf("rollback persisted: %#v err=%v", rolled, err)
	}
}

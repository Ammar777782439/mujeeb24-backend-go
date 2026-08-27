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

func TestChannelConnectionRuntimeAgainstPostgres(t *testing.T) {
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
	businessA := uuid.NewString()
	businessB := uuid.NewString()
	connectionID := uuid.NewString()
	for _, id := range []string{businessA, businessB} {
		_, _ = pool.Exec(ctx, `DELETE FROM businesses WHERE id=$1::uuid`, id)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO businesses (id,name,slug,status,vertical_type,timezone,default_currency,locale,created_at,updated_at) VALUES ($1::uuid,'Channel A','channel-a','active','retail','Asia/Aden','YER','ar-YE',now(),now()),($2::uuid,'Channel B','channel-b','active','retail','Asia/Aden','YER','ar-YE',now(),now())`, businessA, businessB); err != nil {
		t.Fatalf("businesses: %v", err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM businesses WHERE id IN ($1::uuid,$2::uuid)`, businessA, businessB)
	if _, err := pool.Exec(ctx, `INSERT INTO channel_connections (id,business_id,provider_ref,channel,provider_connection_ref,status,secret_reference,created_at,updated_at) VALUES ($1::uuid,$2::uuid,'socialapi','whatsapp','runtime-channel-connection','active','secret-ref',now(),now())`, connectionID, businessA); err != nil {
		t.Fatalf("connection: %v", err)
	}
	repo := NewChannelConnectionRepository(adapter)
	page, err := repo.List(ctx, businessA, "active", "whatsapp", 1, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != connectionID {
		t.Fatalf("list: %#v err=%v", page, err)
	}
	if page, err := repo.List(ctx, businessB, "", "", 50, ""); err != nil || len(page.Items) != 0 {
		t.Fatalf("tenant list: %#v err=%v", page, err)
	}
	reconnected, err := repo.Transition(ctx, ports.ChannelConnectionTransition{BusinessID: businessA, ConnectionID: connectionID, ExpectedVersion: 1, TargetStatus: "reconnect_required", Action: "reconnect_requested", Reason: "credentials rotated", ActorReference: "principal-test"})
	if err != nil || reconnected.Status != "reconnect_required" || reconnected.ResourceVersion != 2 {
		t.Fatalf("reconnect: %#v err=%v", reconnected, err)
	}
	if _, err := repo.Transition(ctx, ports.ChannelConnectionTransition{BusinessID: businessA, ConnectionID: connectionID, ExpectedVersion: 1, TargetStatus: "disconnected", Action: "disconnect_requested", Reason: "stale", ActorReference: "principal-test"}); !IsRepositoryKind(err, RepositoryStale) {
		t.Fatalf("expected stale: %v", err)
	}
	disconnected, err := repo.Transition(ctx, ports.ChannelConnectionTransition{BusinessID: businessA, ConnectionID: connectionID, ExpectedVersion: 2, TargetStatus: "disconnected", Action: "disconnect_requested", Reason: "merchant request", ActorReference: "principal-test"})
	if err != nil || disconnected.Status != "disconnected" || disconnected.ResourceVersion != 3 {
		t.Fatalf("disconnect: %#v err=%v", disconnected, err)
	}
	var events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM channel_connection_state_events WHERE business_id=$1::uuid AND connection_id=$2::uuid`, businessA, connectionID).Scan(&events); err != nil || events != 2 {
		t.Fatalf("events=%d err=%v", events, err)
	}
}

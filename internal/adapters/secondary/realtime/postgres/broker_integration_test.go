package postgres

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBrokerDeliversLargeMessageInvalidationAcrossInstances(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	sender, receiver := NewBroker(pool, nil), NewBroker(pool, nil)
	defer sender.Close()
	defer receiver.Close()
	sender.StartListener(ctx)
	receiver.StartListener(ctx)
	if err := sender.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err := receiver.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	businessID := uuid.NewString()
	connection := NewClientConnection(businessID, nil, "", 8)
	receiver.Hub.Register(connection)
	foreign := NewClientConnection(uuid.NewString(), nil, "", 8)
	receiver.Hub.Register(foreign)
	data, _ := json.Marshal(map[string]string{"text": strings.Repeat("private message ", 1000)})
	event := ports.RealtimeEvent{EventID: uuid.NewString(), EventType: "conversation.note_added", BusinessID: businessID, ResourceType: "conversation", ResourceID: uuid.NewString(), Data: data}
	if err := sender.Publish(ctx, event); err != nil {
		t.Fatal(err)
	}
	select {
	case received := <-connection.Events:
		if received.EventID != event.EventID || len(received.Data) > 0 {
			t.Fatal("cross-instance invalidation leaked or lost the event")
		}
	case <-ctx.Done():
		t.Fatal("LISTEN/NOTIFY did not deliver to a separate broker")
	}
	select {
	case <-foreign.Events:
		t.Fatal("notification crossed tenant scope")
	default:
	}
}

func TestBrokerReestablishesListenAfterDatabaseConnectionTermination(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	applicationName := "listener-recovery-" + uuid.NewString()
	config.ConnConfig.RuntimeParams["application_name"] = applicationName
	config.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	broker := NewBroker(pool, nil)
	defer broker.Close()
	broker.StartListener(ctx)
	if err := broker.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	connection := NewClientConnection(uuid.NewString(), nil, "", 8)
	broker.Hub.Register(connection)
	var terminated bool
	// Only this test's own LISTEN session can match the unique application name.
	if err := pool.QueryRow(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name=$1 AND query=$2 AND pid<>pg_backend_pid()`, applicationName, "LISTEN "+RealtimeNotificationChannel).Scan(&terminated); err != nil || !terminated {
		t.Fatalf("isolated LISTEN termination failed: %v", err)
	}
	select {
	case event := <-connection.Events:
		if event.EventType != "realtime.resync_required" || event.BusinessID != connection.BusinessID {
			t.Fatal("LISTEN recovery did not request scoped HTTP resync")
		}
	case <-ctx.Done():
		t.Fatal("LISTEN did not recover after database disconnect")
	}
	event := ports.RealtimeEvent{EventID: uuid.NewString(), EventType: "conversation.assigned", BusinessID: connection.BusinessID, ResourceType: "conversation", ResourceID: uuid.NewString()}
	if err := broker.Publish(ctx, event); err != nil {
		t.Fatal(err)
	}
	select {
	case received := <-connection.Events:
		if received.EventID != event.EventID {
			t.Fatal("reconnected listener lost the next event")
		}
	case <-ctx.Done():
		t.Fatal("no delivery after LISTEN recovery")
	}
	broker.Close()
	// pgxpool destroys a closed connection asynchronously after Release.
	// Observe the eventual count, not the in-flight destruction callback.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for pool.Stat().AcquiredConns() != 0 {
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("LISTEN retained a pool connection after shutdown")
		}
	}
}

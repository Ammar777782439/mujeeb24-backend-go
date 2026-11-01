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

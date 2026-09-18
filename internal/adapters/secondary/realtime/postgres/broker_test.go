package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestBroker_PublishValidation(t *testing.T) {
	broker := NewBroker(nil, nil)
	defer broker.Close()

	ctx := context.Background()

	// Missing EventID
	err := broker.Publish(ctx, ports.RealtimeEvent{
		EventType:  "conversation.message_received",
		BusinessID: "biz-1",
	})
	if err == nil {
		t.Fatal("expected error when event_id is missing")
	}

	// Missing EventType
	err = broker.Publish(ctx, ports.RealtimeEvent{
		EventID:    "evt-1",
		BusinessID: "biz-1",
	})
	if err == nil {
		t.Fatal("expected error when event_type is missing")
	}

	// Missing BusinessID
	err = broker.Publish(ctx, ports.RealtimeEvent{
		EventID:   "evt-1",
		EventType: "conversation.message_received",
	})
	if err == nil {
		t.Fatal("expected error when business_id is missing")
	}
}

func TestBroker_PublishFallbackToHubWhenPoolNil(t *testing.T) {
	hub := NewLocalHub()
	broker := NewBroker(nil, hub)
	defer broker.Close()

	bizID := "biz-fallback"
	conn := NewClientConnection(bizID, nil, "", 10)
	hub.Register(conn)

	ctx := context.Background()
	event := ports.RealtimeEvent{
		EventID:      "evt-fb",
		EventType:    "conversation.message_sent",
		BusinessID:   bizID,
		ResourceType: "conversation",
		ResourceID:   "conv-fb",
		OccurredAt:   time.Now().UTC(),
		Data:         json.RawMessage(`{"text":"fallback msg"}`),
	}

	err := broker.Publish(ctx, event)
	if err != nil {
		t.Fatalf("expected nil error on fallback publish, got: %v", err)
	}

	select {
	case evt := <-conn.Events:
		if evt.EventID != "evt-fb" {
			t.Fatalf("expected evt-fb, got %s", evt.EventID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for fallback published event")
	}
}

func TestBroker_GracefulShutdownWithoutPanic(t *testing.T) {
	broker := NewBroker(nil, nil)
	// Start listener with pool == nil should be safe no-op
	broker.StartListener(context.Background())
	// Closing multiple times should be idempotent
	broker.Close()
	broker.Close()
}

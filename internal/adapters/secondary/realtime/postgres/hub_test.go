package postgres

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestLocalHub_RegisterAndUnregister(t *testing.T) {
	hub := NewLocalHub()
	defer hub.Close()

	bizA := "biz-111"
	conn1 := NewClientConnection(bizA, nil, "", 10)
	conn2 := NewClientConnection(bizA, nil, "", 10)

	hub.Register(conn1)
	hub.Register(conn2)

	if count := hub.ActiveConnectionCount(bizA); count != 2 {
		t.Fatalf("expected 2 active connections for %s, got %d", bizA, count)
	}

	hub.Unregister(conn1)
	if count := hub.ActiveConnectionCount(bizA); count != 1 {
		t.Fatalf("expected 1 active connection for %s after unregister, got %d", bizA, count)
	}

	hub.Unregister(conn2)
	if count := hub.ActiveConnectionCount(bizA); count != 0 {
		t.Fatalf("expected 0 active connections for %s after unregistering all, got %d", bizA, count)
	}
}

func TestLocalHub_BusinessIsolation(t *testing.T) {
	hub := NewLocalHub()
	defer hub.Close()

	bizA := "biz-aaa"
	bizB := "biz-bbb"

	connA := NewClientConnection(bizA, nil, "", 10)
	connB := NewClientConnection(bizB, nil, "", 10)

	hub.Register(connA)
	hub.Register(connB)

	eventForA := ports.RealtimeEvent{
		EventID:      "evt-1",
		EventType:    "conversation.message_received",
		BusinessID:   bizA,
		ResourceType: "conversation",
		ResourceID:   "conv-1",
		OccurredAt:   time.Now().UTC(),
		Data:         json.RawMessage(`{"text":"hello A"}`),
	}

	hub.Broadcast(eventForA)

	// connA should receive the event
	select {
	case evt := <-connA.Events:
		if evt.EventID != "evt-1" {
			t.Fatalf("connA expected evt-1, got %s", evt.EventID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("connA timed out waiting for event")
	}

	// connB must NOT receive the event
	select {
	case evt := <-connB.Events:
		t.Fatalf("connB unexpectedly received event: %+v", evt)
	case <-time.After(100 * time.Millisecond):
		// Expected: no event received
	}
}

func TestLocalHub_TopicFiltering(t *testing.T) {
	hub := NewLocalHub()
	defer hub.Close()

	bizID := "biz-topics"

	// connConv is only subscribed to conversations
	connConv := NewClientConnection(bizID, []string{"conversations"}, "", 10)
	// connLead is only subscribed to leads
	connLead := NewClientConnection(bizID, []string{"leads"}, "", 10)
	// connAll is subscribed to all topics (empty topic slice)
	connAll := NewClientConnection(bizID, nil, "", 10)

	hub.Register(connConv)
	hub.Register(connLead)
	hub.Register(connAll)

	convEvent := ports.RealtimeEvent{
		EventID:      "evt-conv",
		EventType:    "conversation.state_changed",
		BusinessID:   bizID,
		ResourceType: "conversation",
		ResourceID:   "conv-10",
		OccurredAt:   time.Now().UTC(),
		Data:         json.RawMessage(`{}`),
	}

	hub.Broadcast(convEvent)

	// connConv should receive it
	select {
	case evt := <-connConv.Events:
		if evt.EventID != "evt-conv" {
			t.Fatalf("connConv expected evt-conv, got %s", evt.EventID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("connConv timed out")
	}

	// connAll should receive it
	select {
	case evt := <-connAll.Events:
		if evt.EventID != "evt-conv" {
			t.Fatalf("connAll expected evt-conv, got %s", evt.EventID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("connAll timed out")
	}

	// connLead must NOT receive conversation event
	select {
	case evt := <-connLead.Events:
		t.Fatalf("connLead unexpectedly received conversation event: %+v", evt)
	case <-time.After(100 * time.Millisecond):
		// Expected
	}
}

func TestLocalHub_ResourceFiltering(t *testing.T) {
	hub := NewLocalHub()
	defer hub.Close()

	bizID := "biz-resource"

	// connSpecific is subscribed only to conversation "conv-target"
	connSpecific := NewClientConnection(bizID, nil, "conv-target", 10)
	hub.Register(connSpecific)

	eventOther := ports.RealtimeEvent{
		EventID:      "evt-other",
		EventType:    "conversation.message_received",
		BusinessID:   bizID,
		ResourceType: "conversation",
		ResourceID:   "conv-other",
		OccurredAt:   time.Now().UTC(),
		Data:         json.RawMessage(`{}`),
	}

	hub.Broadcast(eventOther)

	// connSpecific should not receive event for other conversation
	select {
	case evt := <-connSpecific.Events:
		t.Fatalf("connSpecific unexpectedly received event for other conversation: %+v", evt)
	case <-time.After(100 * time.Millisecond):
		// Expected
	}

	eventTarget := ports.RealtimeEvent{
		EventID:      "evt-target",
		EventType:    "conversation.message_received",
		BusinessID:   bizID,
		ResourceType: "conversation",
		ResourceID:   "conv-target",
		OccurredAt:   time.Now().UTC(),
		Data:         json.RawMessage(`{}`),
	}

	hub.Broadcast(eventTarget)

	select {
	case evt := <-connSpecific.Events:
		if evt.EventID != "evt-target" {
			t.Fatalf("connSpecific expected evt-target, got %s", evt.EventID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("connSpecific timed out waiting for target conversation event")
	}
}

func TestLocalHub_SlowConsumerDisconnect(t *testing.T) {
	hub := NewLocalHub()
	defer hub.Close()

	bizID := "biz-slow"

	// Connection with buffer size 1
	slowConn := NewClientConnection(bizID, nil, "", 1)
	// Fast connection with buffer size 10
	fastConn := NewClientConnection(bizID, nil, "", 10)

	hub.Register(slowConn)
	hub.Register(fastConn)

	// Broadcast 3 events without draining slowConn
	for i := 1; i <= 3; i++ {
		hub.Broadcast(ports.RealtimeEvent{
			EventID:      "evt",
			EventType:    "conversation.message_received",
			BusinessID:   bizID,
			ResourceType: "conversation",
			ResourceID:   "conv-1",
			OccurredAt:   time.Now().UTC(),
			Data:         json.RawMessage(`{}`),
		})
	}

	// Give a moment for asynchronous slow-consumer unregistration
	time.Sleep(50 * time.Millisecond)

	// Fast connection should have received all 3 events without issue
	for i := 1; i <= 3; i++ {
		select {
		case <-fastConn.Events:
			// OK
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("fastConn timed out on event %d", i)
		}
	}

	// Slow connection should have been closed/unregistered
	select {
	case <-slowConn.Done:
		// OK: slowConn.Done closed
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected slowConn.Done to be closed due to backpressure overflow")
	}
}

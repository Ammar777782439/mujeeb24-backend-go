package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	realtimePostgres "github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/realtime/postgres"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type mockScopeProvider struct {
	resolveFn func(ctx context.Context, businessID commands.BusinessID) (commands.ActorContext, error)
}

func (m mockScopeProvider) Resolve(ctx context.Context, businessID commands.BusinessID) (commands.ActorContext, error) {
	if m.resolveFn != nil {
		return m.resolveFn(ctx, businessID)
	}
	return commands.ActorContext{
		BusinessID: businessID,
		Role:       "owner",
	}, nil
}

func TestRealtimeSSEHandler_Unauthenticated(t *testing.T) {
	hub := realtimePostgres.NewLocalHub()
	defer hub.Close()

	scope := mockScopeProvider{
		resolveFn: func(ctx context.Context, businessID commands.BusinessID) (commands.ActorContext, error) {
			return commands.ActorContext{}, appErrors.New(appErrors.CodeUnauthenticated, "authenticated principal is required")
		},
	}

	handler := NewRealtimeSSEHandler(hub, scope)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/businesses/biz-test/realtime/events", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rec.Code)
	}
}

func TestRealtimeSSEHandler_Forbidden(t *testing.T) {
	hub := realtimePostgres.NewLocalHub()
	defer hub.Close()

	scope := mockScopeProvider{
		resolveFn: func(ctx context.Context, businessID commands.BusinessID) (commands.ActorContext, error) {
			return commands.ActorContext{}, appErrors.New(appErrors.CodeForbidden, "principal does not have access to this business")
		},
	}

	handler := NewRealtimeSSEHandler(hub, scope)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/businesses/biz-test/realtime/events", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", rec.Code)
	}
}

func TestRealtimeSSEHandler_MethodNotAllowed(t *testing.T) {
	hub := realtimePostgres.NewLocalHub()
	defer hub.Close()

	handler := NewRealtimeSSEHandler(hub, mockScopeProvider{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/businesses/biz-test/realtime/events", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", rec.Code)
	}
}

func TestRealtimeSSEHandler_StreamingAndEventDelivery(t *testing.T) {
	hub := realtimePostgres.NewLocalHub()
	defer hub.Close()

	scope := mockScopeProvider{}
	handler := NewRealtimeSSEHandler(hub, scope)
	handler.HeartbeatInterval = 50 * time.Millisecond // fast heartbeat for test

	bizID := "biz-stream"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/businesses/"+bizID+"/realtime/events", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(rec, req)
	}()

	// Wait for handler to register connection
	time.Sleep(50 * time.Millisecond)

	if count := hub.ActiveConnectionCount(bizID); count != 1 {
		t.Fatalf("expected 1 active connection in hub, got %d", count)
	}

	// Broadcast an event to the business
	hub.Broadcast(ports.RealtimeEvent{
		EventID:      "evt-sse-1",
		EventType:    "conversation.message_received",
		BusinessID:   bizID,
		ResourceType: "conversation",
		ResourceID:   "conv-sse-1",
		OccurredAt:   time.Now().UTC(),
		Data:         json.RawMessage(`{"text":"hello sse"}`),
	})

	// Wait a moment for delivery and heartbeat
	time.Sleep(120 * time.Millisecond)

	// Cancel context to end streaming
	cancel()

	select {
	case <-done:
		// Clean exit
	case <-time.After(1 * time.Second):
		t.Fatal("handler did not terminate on context cancellation")
	}

	// Verify headers
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected Content-Type text/event-stream, got %s", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("expected Cache-Control no-cache, got %s", cc)
	}
	if cn := rec.Header().Get("Connection"); cn != "keep-alive" {
		t.Fatalf("expected Connection keep-alive, got %s", cn)
	}

	body := rec.Body.String()

	// Verify event framing
	if !strings.Contains(body, "id: evt-sse-1\n") {
		t.Fatalf("expected body to contain id: evt-sse-1, got:\n%s", body)
	}
	if !strings.Contains(body, "event: conversation.message_received\n") {
		t.Fatalf("expected body to contain event: conversation.message_received, got:\n%s", body)
	}
	if !strings.Contains(body, "data: ") {
		t.Fatalf("expected body to contain data: ..., got:\n%s", body)
	}
	if !strings.Contains(body, "hello sse") {
		t.Fatalf("expected body to contain payload 'hello sse', got:\n%s", body)
	}

	// Verify heartbeat ping frame
	if !strings.Contains(body, ": ping\n\n") {
		t.Fatalf("expected body to contain heartbeat ': ping\\n\\n', got:\n%s", body)
	}
}

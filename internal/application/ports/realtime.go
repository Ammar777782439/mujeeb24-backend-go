package ports

import (
	"context"
	"encoding/json"
	"time"
)

// RealtimeEvent is the unified, platform-wide event envelope delivered over SSE.
type RealtimeEvent struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	BusinessID    string          `json:"business_id"`
	ResourceType  string          `json:"resource_type"`
	ResourceID    string          `json:"resource_id"`
	OccurredAt    time.Time       `json:"occurred_at"`
	CorrelationID *string         `json:"correlation_id,omitempty"`
	Data          json.RawMessage `json:"data"`
}

// RealtimePublisher is the application port for emitting committed domain/platform events.
// It must only be called after the underlying database transaction has successfully committed.
type RealtimePublisher interface {
	Publish(ctx context.Context, event RealtimeEvent) error
}

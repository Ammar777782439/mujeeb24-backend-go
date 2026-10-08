package ports

import (
	"context"
	"time"
)

// ChannelHistoryStore manages a confirmed purge of one merchant connection's
// local inbox history. Provider accounts and billing are not affected.
type ChannelHistoryStore interface {
	Preview(ctx context.Context, businessID, connectionID string) (ChannelHistorySummary, error)
	Purge(ctx context.Context, businessID, connectionID, idempotencyKey string, expectedVersion int64) (ChannelHistorySummary, error)
}

// ChannelHistoryGuard prevents replay of events that preceded a channel purge.
type ChannelHistoryGuard interface {
	ShouldIgnore(ctx context.Context, businessID, connectionID string, occurredAt *time.Time) (bool, error)
}

type ChannelHistorySummary struct {
	BusinessID    string
	ConnectionID  string
	Conversations int64
	Messages      int64
	PurgedAt      *time.Time
}

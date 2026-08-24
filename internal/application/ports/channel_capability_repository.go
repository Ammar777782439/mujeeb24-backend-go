package ports

import (
	"context"
	"time"
)

type ChannelCapabilityRecord struct {
	ConnectionID   string
	Name           string
	Enabled        bool
	CheckedAt      time.Time
	EvidenceSource *string
}

type ChannelCapabilityRepository interface {
	ListByConnection(ctx context.Context, businessID, connectionID string) ([]ChannelCapabilityRecord, error)
}

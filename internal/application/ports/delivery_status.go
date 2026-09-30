package ports

import (
	"context"
	"time"
)

type DeliveryStatusStore interface {
	Apply(ctx context.Context, draft DeliveryStatusDraft) (DeliveryStatusResult, error)
}

type DeliveryStatusDraft struct {
	InboundEventID     string
	BusinessID         string
	ConnectionID       string
	ProviderRef        string
	ProviderAccountRef string
	ProviderMessageID  string
	Status             string
	OccurredAt         time.Time
}

type DeliveryStatusResult struct {
	InboundEventID    string
	OutboundMessageID string
	Applied           bool
	Ignored           bool
	Duplicate         bool
	Status            string
}

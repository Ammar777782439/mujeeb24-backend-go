package ports

import (
	"context"
	"time"
)

// ProviderInboundStore materializes a verified provider interaction into
// Mujeeb-owned customer, conversation, provider-reference, and message records.
// It performs no provider or workspace network calls.
type ProviderInboundStore interface {
	Materialize(ctx context.Context, draft ProviderInboundDraft) (ProviderInboundResult, error)
}

type ProviderInboundDraft struct {
	InboundEventID         string
	BusinessID             string
	ConnectionID           string
	ProviderRef            string
	ProviderEventID        string
	Channel                string
	ProviderAccountRef     string
	ProviderConversationID string
	ExternalUserID         string
	ProviderMessageID      string
	EventType              string
	InteractionKind        string
	Text                   string
	ExternalCreatedAt      *time.Time
	ReceivedAt             time.Time
	RawPayloadReference    string
	PayloadHash            string
}

type ProviderInboundResult struct {
	BusinessID              string
	Duplicate               bool
	Ignored                 bool
	CustomerID              string
	ConversationID          string
	ConversationReferenceID string
	CommunicationMessageID  string
	InboundEventID          string
}

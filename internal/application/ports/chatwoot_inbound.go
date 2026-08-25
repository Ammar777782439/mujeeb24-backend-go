package ports

import (
	"context"
	"time"
)

// ChatwootInboundStore atomically materializes a verified Chatwoot callback into
// Mujeeb-owned customer, conversation, reference, and communication-message
// records. It never performs a network call.
type ChatwootInboundStore interface {
	Materialize(ctx context.Context, draft ChatwootInboundDraft) (ChatwootInboundResult, error)
}

type ChatwootInboundDraft struct {
	EventID             string
	EventType           string
	RouteKey            string
	AccountID           string
	InboxID             string
	ConversationID      string
	ExternalUserID      string
	ProviderMessageID   string
	Content             string
	OccurredAt          time.Time
	ReceivedAt          time.Time
	RawPayloadReference string
	PayloadHash         string
}

type ChatwootInboundResult struct {
	Duplicate               bool
	CustomerID              string
	ConversationID          string
	ConversationReferenceID string
	CommunicationMessageID  string
	InboundEventID          string
}

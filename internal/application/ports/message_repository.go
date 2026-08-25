package ports

import (
	"context"
	"time"
)

type CommunicationMessageRecord struct {
	ID                      string
	BusinessID              string
	ConversationID          string
	ConversationReferenceID string
	InboundEventID          *string
	OutboundMessageID       *string
	Direction               string
	Origin                  string
	Transport               string
	ProviderMessageID       *string
	ChatwootMessageID       *string
	ContentType             string
	TextContent             *string
	ContentReference        string
	Visibility              string
	OccurredAt              time.Time
	CreatedAt               time.Time
	Status                  string
}

type CommunicationMessageDraft struct {
	ID                      string
	BusinessID              string
	ConversationReferenceID string
	InboundEventID          *string
	OutboundMessageID       *string
	Direction               string
	Origin                  string
	Transport               string
	ProviderMessageID       *string
	ChatwootMessageID       *string
	ContentType             string
	TextContent             *string
	ContentReference        string
	Visibility              string
	OccurredAt              time.Time
	CreatedAt               time.Time
}

type MessagePage struct {
	Items      []CommunicationMessageRecord
	NextCursor string
	HasMore    bool
}

type MessageRepository interface {
	Record(ctx context.Context, draft CommunicationMessageDraft) (CommunicationMessageRecord, error)
	ListByConversation(ctx context.Context, businessID, conversationID string, limit int, cursor string) (MessagePage, error)
}

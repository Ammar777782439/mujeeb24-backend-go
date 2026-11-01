package ports

import (
	"context"
	"time"
)

type CommunicationMessageRecord struct {
	AuthorPrincipalID       *string
	AuthorDisplayName       *string
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
	ContentType             string
	TextContent             *string
	ContentReference        string
	Visibility              string
	OccurredAt              time.Time
	CreatedAt               time.Time
	Status                  string
}

type CommunicationMessageDraft struct {
	AuthorPrincipalID       *string
	ID                      string
	BusinessID              string
	ConversationReferenceID string
	InboundEventID          *string
	OutboundMessageID       *string
	Direction               string
	Origin                  string
	Transport               string
	ProviderMessageID       *string
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

// PrivateNoteTargetReader only resolves public inbound messages in the requested tenant and conversation.
type PrivateNoteTargetReader interface {
	GetPrivateNoteTarget(ctx context.Context, businessID, conversationID, messageID string) (CommunicationMessageRecord, error)
}

type MessageRepository interface {
	Record(ctx context.Context, draft CommunicationMessageDraft) (CommunicationMessageRecord, error)
	ListByConversation(ctx context.Context, businessID, conversationID string, limit int, cursor string) (MessagePage, error)
}

// PrivateNoteRepository must atomically return an existing identical note or
// insert it once within the caller's transaction. A changed payload conflicts.
type PrivateNoteRepository interface {
	RecordPrivateNoteOnce(ctx context.Context, conversationID string, draft CommunicationMessageDraft) (record CommunicationMessageRecord, created bool, err error)
}

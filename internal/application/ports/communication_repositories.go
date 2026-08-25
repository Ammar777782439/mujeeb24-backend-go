package ports

import "context"

type ConversationReferenceRecord struct {
	ID               string
	BusinessID       string
	ConversationID   string
	System           string
	ProviderRef      string
	ResourceType     string
	ResourceID       string
	ConnectionID     *string
	ConversationKind *string
	IsCurrent        bool
	MappingStatus    string
}

type ConversationReferenceRepository interface {
	GetByID(ctx context.Context, businessID, referenceID string) (ConversationReferenceRecord, error)
	GetCurrentByConversation(ctx context.Context, businessID, conversationID, system string) (ConversationReferenceRecord, error)
}

type OutboundMessageDraft struct {
	ID                      string
	BusinessID              string
	ConversationID          string
	ConversationReferenceID string
	ConnectionID            string
	ProviderRef             string
	Channel                 string
	Origin                  string
	Transport               string
	ContentReference        string
	ProviderIdempotencyKey  string
	CorrelationID           *string
	CausationID             *string
}

type OutboundMessageRecord struct {
	ID                      string
	BusinessID              string
	ConversationID          string
	ConversationReferenceID string
	ConnectionID            string
	ProviderRef             string
	Channel                 string
	Origin                  string
	Direction               string
	Transport               string
	ContentReference        string
	ProviderIdempotencyKey  string
	Status                  string
	ProviderMessageID       *string
	ChatwootMessageID       *string
	FailureCode             *string
	AttemptCount            int
	CorrelationID           *string
	CausationID             *string
}

type OutboundMessageRepository interface {
	CreatePending(ctx context.Context, draft OutboundMessageDraft) (OutboundMessageRecord, error)
	GetByID(ctx context.Context, businessID, messageID string) (OutboundMessageRecord, error)
}

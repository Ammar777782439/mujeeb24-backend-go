package ports

import (
	"context"
	"time"
)

type EventStore interface {
	RecordIfAbsent(ctx context.Context, draft InboundEventDraft) (created bool, record InboundEventRecord, err error)
	Get(ctx context.Context, businessID, eventID string) (InboundEventRecord, error)
	List(ctx context.Context, filter InboundEventFilter) (InboundEventPage, error)
	Claim(ctx context.Context, eventID string, lease InboundEventLease) (InboundEventClaimResult, error)
	MarkProcessed(ctx context.Context, eventID string, completion InboundEventCompletion) (InboundEventRecord, error)
	MarkRetryableFailure(ctx context.Context, eventID string, failure InboundEventFailure) (InboundEventRecord, error)
	MoveToDeadLetter(ctx context.Context, eventID string, failure InboundEventFailure) (InboundEventRecord, error)
}

type InboundEventFilter struct {
	BusinessID      string
	ConnectionID    string
	ProviderRef     string
	ProcessingState string
	Limit           int
	Cursor          string
}

type InboundEventPage struct {
	Items      []InboundEventRecord
	NextCursor string
	HasMore    bool
}

type InboundEventRecord struct {
	ID                     string
	ProviderRef            string
	ProviderConnectionRef  string
	ProviderEventID        string
	DedupeStrategy         string
	BusinessID             *string
	ConnectionID           *string
	EventType              string
	InteractionKind        *string
	ProviderMessageID      *string
	ProviderConversationID *string
	ExternalUserID         *string
	ContentReference       *string
	ExternalCreatedAt      *time.Time
	ReceivedAt             time.Time
	RawPayloadReference    string
	PayloadHash            string
	SignatureVerified      bool
	ProcessingState        string
	ProcessingOwner        *string
	ProcessingLeaseToken   *string
	LeaseExpiresAt         *time.Time
	AttemptCount           int
	LastErrorCode          *string
	NextAttemptAt          *time.Time
	ProcessingResultCode   *string
	ProcessedAt            *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type InboundEventDraft struct {
	ID                     string
	ProviderRef            string
	ProviderConnectionRef  string
	ProviderEventID        string
	DedupeStrategy         string
	BusinessID             *string
	ConnectionID           *string
	EventType              string
	InteractionKind        *string
	ProviderMessageID      *string
	ProviderConversationID *string
	ExternalUserID         *string
	ContentReference       *string
	ExternalCreatedAt      *time.Time
	ReceivedAt             time.Time
	RawPayloadReference    string
	PayloadHash            string
	SignatureVerified      bool
	ProcessingState        string
	NextAttemptAt          *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type InboundEventLease struct {
	Owner     string
	Token     string
	ExpiresAt time.Time
}

type InboundEventClaimResult struct {
	Claimed bool
	Record  InboundEventRecord
}

type InboundEventCompletion struct {
	Owner       string
	Token       string
	ResultCode  string
	ProcessedAt time.Time
	UpdatedAt   time.Time
}

type InboundEventFailure struct {
	Owner       string
	Token       string
	ErrorCode   string
	NextAttempt *time.Time
	UpdatedAt   time.Time
}

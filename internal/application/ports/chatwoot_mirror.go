package ports

import (
	"context"
	"time"
)

// ChatwootMirrorStore persists the Mujeeb-owned obligation to mirror one
// normalized CommunicationMessage to the internal Chatwoot workspace.
// Network calls are deliberately not part of this port.
type ChatwootMirrorStore interface {
	Enqueue(ctx context.Context, draft ChatwootMirrorDraft) (ChatwootMirrorJob, error)
	ListClaimable(ctx context.Context, limit int) ([]ChatwootMirrorJob, error)
	Claim(ctx context.Context, jobID string, lease MirrorLease) (MirrorClaimResult, error)
	Resolve(ctx context.Context, businessID, jobID string) (ChatwootMirrorDelivery, error)
	MarkCompleted(ctx context.Context, completion ChatwootMirrorCompletion) (ChatwootMirrorJob, error)
	MoveToDeadLetter(ctx context.Context, failure ChatwootMirrorFailure) (ChatwootMirrorJob, error)
}

type ChatwootMirrorDraft struct {
	ID                     string
	BusinessID             string
	CommunicationMessageID string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type ChatwootMirrorJob struct {
	ID                     string
	BusinessID             string
	CommunicationMessageID string
	Status                 string
	AttemptCount           int
	LeaseOwner             *string
	LeaseToken             *string
	LeaseExpiresAt         *time.Time
	FailureCode            *string
	ResultCode             *string
	ChatwootContactID      *string
	ChatwootConversationID *string
	ChatwootMessageID      *string
}

type MirrorLease struct {
	Owner     string
	Token     string
	ExpiresAt time.Time
}

type MirrorClaimResult struct {
	Claimed bool
	Record  ChatwootMirrorJob
}

// ChatwootMirrorDelivery is resolved from Mujeeb state only. The processor
// calls the workspace after this resolution and outside a database transaction.
type ChatwootMirrorDelivery struct {
	BusinessID              string
	JobID                   string
	CommunicationMessageID  string
	ConversationReferenceID string
	CustomerID              string
	CustomerName            string
	CustomerIdentifier      string
	ProviderConversationID  string
	Text                    string
	AccountID               int64
	InboxID                 int64
}

type ChatwootMirrorCompletion struct {
	JobID                  string
	Owner                  string
	Token                  string
	AccountID              int64
	InboxID                int64
	ChatwootContactID      string
	ChatwootConversationID string
	ChatwootMessageID      string
	CompletedAt            time.Time
	UpdatedAt              time.Time
}

type ChatwootMirrorFailure struct {
	JobID       string
	Owner       string
	Token       string
	FailureCode string
	UpdatedAt   time.Time
}

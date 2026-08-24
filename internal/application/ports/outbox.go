package ports

import (
	"context"
	"time"
)

type OutboxStore interface {
	Enqueue(ctx context.Context, draft OutboxEntryDraft) (OutboxEntryRecord, error)
	Get(ctx context.Context, businessID, entryID string) (OutboxEntryRecord, error)
	List(ctx context.Context, filter OutboxFilter) (OutboxPage, error)
	Claim(ctx context.Context, entryID string, lease OutboxLease) (OutboxClaimResult, error)
	MarkCompleted(ctx context.Context, entryID string, completion OutboxCompletion) (OutboxEntryRecord, error)
	MarkRetryableFailure(ctx context.Context, entryID string, failure OutboxFailure) (OutboxEntryRecord, error)
	MoveToDeadLetter(ctx context.Context, entryID string, failure OutboxFailure) (OutboxEntryRecord, error)
	Requeue(ctx context.Context, entryID string, nextAttemptAt, updatedAt time.Time) (OutboxEntryRecord, error)
}

type OutboxFilter struct {
	BusinessID string
	Status     string
	Limit      int
	Cursor     string
}

type OutboxPage struct {
	Items      []OutboxEntryRecord
	NextCursor string
	HasMore    bool
}

type OutboxEntryDraft struct {
	ID                string
	BusinessID        string
	OutboundMessageID string
	CommandType       string
	DedupeKey         string
	AvailableAt       time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type OutboxEntryRecord struct {
	ID                string
	BusinessID        string
	OutboundMessageID string
	CommandType       string
	DedupeKey         string
	Status            string
	AttemptCount      int
	AvailableAt       time.Time
	LeaseOwner        *string
	LeaseToken        *string
	LeaseExpiresAt    *time.Time
	LastErrorCode     *string
	ResultCode        *string
	CompletedAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type OutboxLease struct {
	Owner     string
	Token     string
	ExpiresAt time.Time
}

type OutboxClaimResult struct {
	Claimed bool
	Record  OutboxEntryRecord
}

type OutboxCompletion struct {
	Owner       string
	Token       string
	ResultCode  string
	CompletedAt time.Time
	UpdatedAt   time.Time
}

type OutboxFailure struct {
	Owner       string
	Token       string
	ErrorCode   string
	NextAttempt *time.Time
	UpdatedAt   time.Time
}

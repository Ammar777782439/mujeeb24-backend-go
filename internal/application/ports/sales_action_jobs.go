package ports

import (
	"context"
	"time"
)

// The button job is also the durable continuation carrier; no second queue is created.
type SalesActionJob struct {
	ID, LeaseToken, Phase string
	Action                SalesButtonAction
}

// SalesStartContinuation is trusted DB state, never synthesized as a customer message.
type SalesStartContinuation struct {
	RequestID, WorkflowID, BusinessID, CustomerID, ConversationID string
	SourceMessageID, Text, Channel, ProviderRef                   string
	Completed, AwaitingHumanReview                                bool
}

type SalesActionJobRepository interface {
	Claim(context.Context, int, time.Time) ([]SalesActionJob, error)
	Finish(context.Context, string, string, string, time.Time) error
	LoadContinuation(context.Context, SalesActionJob) (SalesStartContinuation, error)
	CompleteContinuation(context.Context, SalesActionJob, string) error
}

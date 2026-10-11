package services

import (
	"context"
	"fmt"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// Authorization and execution transitions must succeed before business writes.
// Passing the transaction context keeps them atomic with the decision/outbox.
func (s AutoReplyService) transitionRun(ctx context.Context, run ports.AIRunRecord, status string) error {
	if s.RunRepository == nil || run.ID == "" {
		return nil
	}
	lifecycle := NewAIRunLifecycle(s.RunRepository)
	var err error
	switch status {
	case ports.AIRunStatusValidating:
		_, err = lifecycle.MarkValidating(ctx, run.BusinessID, run.ID)
	case ports.AIRunStatusAuthorized:
		_, err = lifecycle.MarkAuthorized(ctx, run.BusinessID, run.ID)
	case ports.AIRunStatusExecuting:
		_, err = lifecycle.MarkExecuting(ctx, run.BusinessID, run.ID)
	default:
		return fmt.Errorf("unsupported auto reply transition %s", status)
	}
	if err != nil {
		return fmt.Errorf("auto reply lifecycle %s: %w", status, err)
	}
	return nil
}

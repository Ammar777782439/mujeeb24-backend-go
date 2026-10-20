package services

import (
	"context"
	"errors"
	"log"
	"net"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/aierrors"
)

type SalesActionProcessor struct {
	Jobs      ports.SalesActionJobRepository
	Sales     CustomerSalesWorkflowService
	AutoReply commands.AutoReplyHandler
}

func (p SalesActionProcessor) Process(ctx context.Context, limit int) (int, error) {
	if p.Jobs == nil {
		return 0, errors.New("sales action jobs are not configured")
	}
	jobs, err := p.Jobs.Claim(ctx, limit, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	for i, j := range jobs {
		code := ""
		var execErr error
		if j.Phase == "continue_ai" {
			execErr = p.continueSales(ctx, j)
			if execErr != nil {
				code = "sales_continuation_permanent"
				if retryableSalesContinuation(execErr) {
					code = "sales_continuation_transient"
				}
				log.Printf("[SalesAction] CONTINUATION_FAILED job=%s business=%s", j.ID, j.Action.BusinessID)
			}
		} else if strings.HasPrefix(j.Action.Payload, "m24.sales.start:") {
			var result ports.SalesWorkflowResult
			result, execErr = p.Sales.HandleStartButton(ctx, j.Action)
			if execErr == nil && result.Workflow.ID != "" {
				code = "sales_continuation_queued"
			}
		} else {
			_, execErr = p.Sales.HandleButton(ctx, j.Action)
		}
		if execErr != nil && j.Phase != "continue_ai" {
			code = "sales_action_failed"
			log.Printf("[SalesAction] FAILED job=%s business=%s", j.ID, j.Action.BusinessID)
		}
		if err := p.Jobs.Finish(ctx, j.ID, j.LeaseToken, code, time.Now().UTC()); err != nil {
			return i, err
		}
	}
	return len(jobs), nil
}

func (p SalesActionProcessor) continueSales(ctx context.Context, j ports.SalesActionJob) error {
	continuation, err := p.Jobs.LoadContinuation(ctx, j)
	if err != nil {
		return err
	}
	if continuation.Completed {
		return nil
	}
	// Employee review is a real durable workflow state, not an invented handoff.
	if continuation.AwaitingHumanReview {
		if p.Sales.Transactions == nil {
			return errors.New("sales transaction manager missing")
		}
		return p.Sales.Transactions.Within(ctx, func(tx context.Context) error {
			return p.Jobs.CompleteContinuation(tx, j, continuation.RequestID)
		})
	}
	if p.AutoReply == nil {
		return errors.New("customer sales AI continuation is not wired")
	}
	if strings.TrimSpace(continuation.Text) == "" {
		return errors.New("original customer message has no reusable text")
	}
	_, err = p.AutoReply.Handle(ctx, commands.AutoReplyCommand{
		Meta:                   commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(continuation.BusinessID)}},
		ConversationID:         commands.ConversationID(continuation.ConversationID),
		SourceMessageReference: continuation.SourceMessageID,
		Text:                   continuation.Text, Channel: continuation.Channel, ProviderRef: continuation.ProviderRef,
		SalesStartContinuationID: continuation.RequestID, SalesStartJobID: j.ID,
		SalesStartWorkflowID: continuation.WorkflowID, SalesStartLeaseToken: j.LeaseToken,
	})
	// Handle may commit an Outbox response before failing on usage accounting.
	// Read durable state instead of causing another model call.
	if err != nil {
		if current, lookupErr := p.Jobs.LoadContinuation(ctx, j); lookupErr == nil && current.Completed {
			return nil
		}
		return err
	}
	current, err := p.Jobs.LoadContinuation(ctx, j)
	if err != nil {
		return err
	}
	if !current.Completed {
		return errors.New("AI continuation completed without durable message or review")
	}
	return nil
}

func retryableSalesContinuation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		return true
	}
	var aiErr *aierrors.Failure
	if errors.As(err, &aiErr) {
		return aiErr.Kind != aierrors.ModelOutput && aiErr.Kind != aierrors.CatalogData && aiErr.Kind != aierrors.CatalogBudget
	}
	// Infrastructure failures are retryable; persistent contract/data failures
	// must be made explicit by their typed error before reaching this boundary.
	return true
}

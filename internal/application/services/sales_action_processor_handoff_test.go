package services

import (
	"context"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type persistedHumanHandoffJobs struct {
	completions int
	finishCode  string
}

func (f *persistedHumanHandoffJobs) Claim(context.Context, int, time.Time) ([]ports.SalesActionJob, error) {
	return []ports.SalesActionJob{{
		ID: "job", LeaseToken: "lease", Phase: "continue_ai",
		Action: ports.SalesButtonAction{BusinessID: "business", CustomerID: "customer", ConversationID: "conversation"},
	}}, nil
}

func (f *persistedHumanHandoffJobs) Finish(_ context.Context, _, _, code string, _ time.Time) error {
	f.finishCode = code
	return nil
}

func (f *persistedHumanHandoffJobs) LoadContinuation(context.Context, ports.SalesActionJob) (ports.SalesStartContinuation, error) {
	return ports.SalesStartContinuation{
		RequestID: "start", WorkflowID: "workflow",
		BusinessID: "business", CustomerID: "customer", ConversationID: "conversation",
		AwaitingHumanReview: true,
	}, nil
}

func (f *persistedHumanHandoffJobs) CompleteContinuation(context.Context, ports.SalesActionJob, string) error {
	f.completions++
	return nil
}

type forbiddenHandoffAI struct{ calls int }

func (f *forbiddenHandoffAI) Handle(context.Context, commands.AutoReplyCommand) (commands.AutoReplyResult, error) {
	f.calls++
	return commands.AutoReplyResult{}, nil
}

// The PostgreSQL test verifies LoadContinuation sees waiting_human; this test
// verifies that the processor responds without another call to the AI provider.
func TestSalesActionProcessorDoesNotCallAIForPersistedHumanHandoff(t *testing.T) {
	jobs := &persistedHumanHandoffJobs{}
	ai := &forbiddenHandoffAI{}
	processor := SalesActionProcessor{
		Jobs:      jobs,
		Sales:     CustomerSalesWorkflowService{Transactions: fakeTransactionManager{}},
		AutoReply: ai,
	}
	count, err := processor.Process(context.Background(), 1)
	if err != nil || count != 1 || ai.calls != 0 || jobs.completions != 1 || jobs.finishCode != "" {
		t.Fatalf("handoff must finish without AI: count=%d ai_calls=%d completions=%d code=%q err=%v",
			count, ai.calls, jobs.completions, jobs.finishCode, err)
	}
}

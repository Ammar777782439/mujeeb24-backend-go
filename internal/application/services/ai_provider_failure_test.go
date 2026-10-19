package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/native"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/aierrors"
)

type workerContextInspection struct {
	deadline chan bool
}

func (h workerContextInspection) Handle(ctx context.Context, _ commands.AutoReplyCommand) (commands.AutoReplyResult, error) {
	_, hasDeadline := ctx.Deadline()
	h.deadline <- hasDeadline
	return commands.AutoReplyResult{}, nil
}

// The worker must not cancel the provider at 120 seconds (or any independent
// shorter duration); NATIVE_AI_HTTP_TIMEOUT is enforced by the HTTP client.
func TestWorkerDoesNotImposeDeadlineOnNative(t *testing.T) {
	pool := NewAutoReplyWorkerPool(1, 1)
	probe := workerContextInspection{deadline: make(chan bool, 1)}
	if !pool.Submit(autoReplyTask{handler: probe}) {
		t.Fatal("worker task was rejected")
	}
	select {
	case hasDeadline := <-probe.deadline:
		if hasDeadline {
			t.Fatal("worker must not set an AI execution deadline")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not start")
	}
	pool.Stop()
}

func TestAIProviderErrorClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"deadline", fmt.Errorf("gateway call: %w", context.DeadlineExceeded), string(ports.AIRunFailureCategoryTimeout)},
		{"canceled", context.Canceled, string(ports.AIRunFailureCategoryCancelled)},
		{"gateway 503", &native.StatusError{Provider: "gemini", Code: 503}, string(ports.AIRunFailureCategoryProviderTemporary)},
		{"gateway 429", &native.StatusError{Provider: "gemini", Code: 429}, string(ports.AIRunFailureCategoryRateLimit)},
		{"gateway 401", &native.StatusError{Provider: "gemini", Code: 401}, string(ports.AIRunFailureCategoryProviderPermanent)},
		{"network timeout", &net.DNSError{IsTimeout: true}, string(ports.AIRunFailureCategoryTimeout)},
		{"other", errors.New("invalid function arguments"), string(ports.AIRunFailureCategoryUnclassified)},
		{"catalog budget", fmt.Errorf("wrapper: %w", aierrors.Wrap(aierrors.CatalogBudget, errors.New("no progress"))), string(ports.AIRunFailureCategoryCatalogBudget)},
		{"model output", aierrors.Wrap(aierrors.ModelOutput, errors.New("schema")), string(ports.AIRunFailureCategoryInvalidAIOutput)}
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyAIProviderError(tt.err); got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

type canceledContextRunRepo struct {
	ports.AIRunRepository
	persisted bool
	category  string
}

func (r *canceledContextRunRepo) GetRun(ctx context.Context, _, _ string) (ports.AIRunRecord, error) {
	if err := ctx.Err(); err != nil {
		return ports.AIRunRecord{}, err
	}
	return ports.AIRunRecord{ID: "run-1", BusinessID: "business-1", Status: ports.AIRunStatusRunning}, nil
}
func (r *canceledContextRunRepo) UpdateRunStatus(ctx context.Context, _, _ string, patch ports.AIRunStatusPatch) (ports.AIRunRecord, error) {
	if err := ctx.Err(); err != nil {
		return ports.AIRunRecord{}, err
	}
	if patch.Status != ports.AIRunStatusFailed || patch.FailureCategory == nil {
		return ports.AIRunRecord{}, errors.New("unexpected run transition")
	}
	r.persisted = true
	r.category = *patch.FailureCategory
	return ports.AIRunRecord{ID: "run-1", Status: ports.AIRunStatusFailed}, nil
}

func TestMarkFailedPersistsWhenProviderContextExpired(t *testing.T) {
	repo := &canceledContextRunRepo{}
	svc := AutoReplyService{RunRepository: repo}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.markFailedSafe(ctx,
		ports.AIRunRecord{ID: "run-1", BusinessID: "business-1"},
		ports.AIRunFailureStageProviderRequest, string(ports.AIRunFailureCategoryTimeout), "context deadline exceeded",
	)
	if !repo.persisted || repo.category != string(ports.AIRunFailureCategoryTimeout) {
		t.Fatalf("provider timeout not recorded in terminal state: %+v", repo)
	}
}

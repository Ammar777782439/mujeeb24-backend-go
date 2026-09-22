// Package services — AI Runtime Retry / Backoff / Idempotency
//
// Implements contract ⑨ §5-15 (Retry Principle, Retryable vs Non-Retryable,
// Backoff, Provider Rate Limit, Timeout, Idempotency, Duplicate handling,
// Partial Progress, Catalog Batch State, Validation/Policy/Authorization/
// Execution Failure, Cancellation, State Transition Rule, Attempt vs AI Run,
// Runtime Does Not Mutate Business State, Provider Failure Does Not Force
// Human Handoff, Failure Visibility, Final Result).
//
// Per contract ⑨ §8, no fixed retry count is in the Domain. This service reads
// retry configuration from runtime config and decides whether to retry an
// Attempt, spawn a new Attempt on the same Run, or fail the Run.
//
// Per contract ⑨ §17, Gemini is never used as a retry mechanism. The Runtime
// is the sole owner of retry; Gemini only sees fresh Interactions.
//
// Per contract ⑨ §28, Retry is an Infrastructure concern and does NOT mutate
// Lead status, Order, or Conversation ownership. Business state changes only
// when a correct result reaches the application/domain layer.

package services

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// RetryConfig is the runtime configuration for retry behavior. Per contract ⑨
// §8, no fixed number lives in the Domain; this config comes from the
// deployment environment (env vars, config files).
type RetryConfig struct {
	// MaxAttempts is the maximum number of operational Attempts per AI Run.
	// Per contract ⑨ §8, this is Runtime config, NOT Domain contract. A
	// sensible production default is 3-5.
	MaxAttempts int

	// InitialBackoff is the first retry delay. Per contract ⑨ §9, Backoff
	// is Runtime-configured.
	InitialBackoff time.Duration

	// MaxBackoff caps the exponential backoff to avoid very long waits.
	MaxBackoff time.Duration

	// BackoffMultiplier applies to the previous backoff on each retry.
	// 2.0 means exponential; 1.5 means gentler.
	BackoffMultiplier float64

	// JitterFraction adds randomness to avoid thundering-herd on rate limits.
	// 0.2 means ±20% of the computed backoff.
	JitterFraction float64

	// ProviderTimeout is the per-request timeout for Gemini API calls.
	// Per contract ⑨ §11-12, every external operation must have a Timeout.
	ProviderTimeout time.Duration

	// ToolTimeout is the per-tool-call timeout. Per contract ⑨ §13.
	ToolTimeout time.Duration

	// ExecutionTimeout is the per-execution-action timeout. Per contract ⑨ §14.
	ExecutionTimeout time.Duration
}

// DefaultRetryConfig is a sensible production starting point. Per contract ⑨
// §8, this is NOT a Domain contract — operators may override it.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    500 * time.Millisecond,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
		JitterFraction:    0.2,
		ProviderTimeout:   45 * time.Second,
		ToolTimeout:       15 * time.Second,
		ExecutionTimeout:  30 * time.Second,
	}
}

// RetryDecision is what the RetryPolicy produces for a failed Attempt.
type RetryDecision struct {
	// ShouldRetry is true when a new Attempt on the same Run is warranted.
	ShouldRetry bool

	// NextAttemptNumber is the next Attempt's number (1-indexed).
	NextAttemptNumber int

	// BackoffBeforeNextAttempt is the delay before the next Attempt.
	BackoffBeforeNextAttempt time.Duration

	// FailureCategory echoes the input for logging.
	FailureCategory ports.AIRunFailureCategory

	// Reason explains the decision in plain language.
	Reason string
}

// RetryPolicy decides whether a failed Attempt should be retried. Per contract
// ⑨ §6-7, the Retryable/Non-Retryable distinction is the core decision.
type RetryPolicy struct {
	Config RetryConfig
	Rand   *rand.Rand
}

func NewRetryPolicy(cfg RetryConfig) *RetryPolicy {
	return &RetryPolicy{Config: cfg, Rand: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

// DecideForFailure examines a failed Attempt and returns a RetryDecision.
//
// Per contract ⑨ §6, Retryable categories are: provider_temporary, network,
// timeout, rate_limit, infrastructure.
//
// Per contract ⑨ §7, Non-Retryable categories are: provider_permanent,
// invalid_ai_output, invalid_tool_arguments, tenant_violation,
// invalid_reference, policy_denial, authorization_denial, unsupported_action,
// execution_failure (depending on context — execution_failure may be
// retryable if the external action supports safe retry, but we err on the
// side of caution here).
//
// Per contract ⑨ §8, if MaxAttempts is exhausted, ShouldRetry is false even
// for Retryable categories.
func (p *RetryPolicy) DecideForFailure(currentAttemptNumber int, category ports.AIRunFailureCategory) RetryDecision {
	if !category.IsRetryable() {
		return RetryDecision{
			ShouldRetry:              false,
			NextAttemptNumber:        currentAttemptNumber,
			BackoffBeforeNextAttempt: 0,
			FailureCategory:          category,
			Reason:                   "failure category is Non-Retryable per contract ⑨ §7",
		}
	}
	if currentAttemptNumber >= p.Config.MaxAttempts {
		return RetryDecision{
			ShouldRetry:              false,
			NextAttemptNumber:        currentAttemptNumber,
			BackoffBeforeNextAttempt: 0,
			FailureCategory:          category,
			Reason:                   "MaxAttempts exhausted per contract ⑨ §8",
		}
	}
	backoff := p.computeBackoff(currentAttemptNumber)
	return RetryDecision{
		ShouldRetry:              true,
		NextAttemptNumber:        currentAttemptNumber + 1,
		BackoffBeforeNextAttempt: backoff,
		FailureCategory:          category,
		Reason:                   "failure category is Retryable; spawning next Attempt per contract ⑨ §6",
	}
}

// computeBackoff returns the delay before the next Attempt.
//
// Per contract ⑨ §9, Backoff is exponential with jitter to avoid thundering-herd.
// The formula is: backoff = min(MaxBackoff, InitialBackoff * multiplier^attempt)
//   - (1 ± JitterFraction)
func (p *RetryPolicy) computeBackoff(currentAttemptNumber int) time.Duration {
	backoff := float64(p.Config.InitialBackoff)
	for i := 1; i < currentAttemptNumber; i++ {
		backoff *= p.Config.BackoffMultiplier
	}
	if backoff > float64(p.Config.MaxBackoff) {
		backoff = float64(p.Config.MaxBackoff)
	}
	if p.Config.JitterFraction > 0 {
		jitter := 1.0 + (p.Rand.Float64()*2-1)*p.Config.JitterFraction
		backoff *= jitter
	}
	return time.Duration(backoff)
}

// SleepForBackoff blocks for the computed backoff duration. Used between
// Attempts. Per contract ⑨ §9, this avoids tight retry loops.
//
// The function returns ctx.Err() if the context is cancelled during sleep,
// allowing upstream cancellation to abort the retry loop.
func (p *RetryPolicy) SleepForBackoff(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// PartialProgressPolicy decides, for a multi-batch Catalog Evaluation that
// partially failed, whether to retry only the failed batch or fail the whole
// Run.
//
// Per contract ⑨ §21, when Batch 1 ✅, Batch 2 ✅, Batch 3 ❌, the Runtime
// retries Batch 3 only — it does NOT re-run Batches 1 and 2 from scratch.
//
// Per contract ⑨ §22, each Batch has state PENDING/RUNNING/COMPLETED/FAILED,
// and the controller can resume from where it left off.
type PartialProgressPolicy struct{}

func NewPartialProgressPolicy() *PartialProgressPolicy { return &PartialProgressPolicy{} }

// BatchesToRetry returns the IDs of batches in FAILED state that should be
// retried. COMPLETED batches are NOT included per contract ⑨ §21.
//
// If all batches are COMPLETED, returns an empty list (final evaluation may proceed).
// If any batch is in PENDING state, those should also be attempted (they may have
// never started due to a prior failure).
func (p *PartialProgressPolicy) BatchesToRetry(batches []ports.AICatalogBatchRecord) []string {
	var retryIDs []string
	for _, b := range batches {
		if b.Status == "failed" || b.Status == "pending" {
			retryIDs = append(retryIDs, b.ID)
		}
	}
	return retryIDs
}

// CoverageComplete returns true when every batch in the evaluation is COMPLETED.
//
// Per contract ② §3 and ⑨ §23, no Final Gemini Evaluation may run before
// Coverage is complete for the entire catalog scope.
func (p *PartialProgressPolicy) CoverageComplete(batches []ports.AICatalogBatchRecord) bool {
	if len(batches) == 0 {
		return true
	}
	for _, b := range batches {
		if b.Status != "completed" {
			return false
		}
	}
	return true
}

// ErrNoRetryableAttempts is returned when a Run has exhausted its retry budget
// and MustFail. Per contract ⑨ §6, this is the FAILED terminal transition.
var ErrNoRetryableAttempts = errors.New("no retryable attempts remaining per contract ⑨ §6")

// ErrNonRetryableFailure is returned when the failure category is Non-Retryable.
// Per contract ⑨ §7, no retry is attempted; the Run fails immediately.
var ErrNonRetryableFailure = errors.New("non-retryable failure per contract ⑨ §7")

// ErrIdempotencyViolation is returned when an attempt is made to create a
// duplicate AI Run for the same source event. Per contract ⑨ §16, this is
// prevented at the DB uniqueness level too.
var ErrIdempotencyViolation = errors.New("idempotency violation: AI Run already exists for this source event per contract ⑨ §16")

// ClassifyError maps a raw error from a Gemini call, tool call, or execution
// to one of the closed AIRunFailureCategory values per contract ⑨ §6-7.
//
// This is intentionally simple: error string matching. A production deployment
// may want richer classification (e.g., HTTP status codes), but the closed
// category set is the contract.
func ClassifyError(err error) ports.AIRunFailureCategory {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, "timeout"):
		return ports.AIRunFailureCategoryTimeout
	case strings.Contains(msg, "rate limit") || strings.Contains(msg, "429") || strings.Contains(msg, "quota"):
		return ports.AIRunFailureCategoryRateLimit
	case strings.Contains(msg, "network") || strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") || strings.Contains(msg, "dns"):
		return ports.AIRunFailureCategoryNetwork
	case strings.Contains(msg, "invalid reference") || strings.Contains(msg, "reference not found"):
		return ports.AIRunFailureCategoryInvalidReference
	case strings.Contains(msg, "tenant") || strings.Contains(msg, "business scope"):
		return ports.AIRunFailureCategoryTenantViolation
	case strings.Contains(msg, "policy denied") || strings.Contains(msg, "policy_decision=denied"):
		return ports.AIRunFailureCategoryPolicyDenial
	case strings.Contains(msg, "authorization denied") || strings.Contains(msg, "unauthorized"):
		return ports.AIRunFailureCategoryAuthorizationDenial
	case strings.Contains(msg, "invalid ai output") || strings.Contains(msg, "proposal schema"):
		return ports.AIRunFailureCategoryInvalidAIOutput
	case strings.Contains(msg, "tool argument"):
		return ports.AIRunFailureCategoryInvalidToolArguments
	case strings.Contains(msg, "unsupported action"):
		return ports.AIRunFailureCategoryUnsupportedAction
	case strings.Contains(msg, "execution failed"):
		return ports.AIRunFailureCategoryExecutionFailure
	case strings.Contains(msg, "5xx") || strings.Contains(msg, "502") || strings.Contains(msg, "503") || strings.Contains(msg, "504"):
		return ports.AIRunFailureCategoryProviderTemporary
	case strings.Contains(msg, "400") || strings.Contains(msg, "401") || strings.Contains(msg, "403") || strings.Contains(msg, "404"):
		return ports.AIRunFailureCategoryProviderPermanent
	}
	return ports.AIRunFailureCategoryInfrastructure
}

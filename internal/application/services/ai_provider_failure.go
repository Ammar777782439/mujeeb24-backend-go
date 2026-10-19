package services

import (
	"context"
	"errors"
	"net"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/platform/aierrors"
)

// classifyAIProviderError keeps transport timeouts and transient gateway
// failures retryable in AI trace, not permanently attributed to the model.
// Native provider failover remains isolated from commercial execution.
func classifyAIProviderError(err error) string {
	if errors.Is(err, context.Canceled) {
		return string(ports.AIRunFailureCategoryCancelled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return string(ports.AIRunFailureCategoryTimeout)
	}
	var known *aierrors.Failure
	if errors.As(err, &known) {
		switch known.Kind {
		case aierrors.Application:
			return string(ports.AIRunFailureCategoryApplicationInternal)
		case aierrors.CatalogBudget:
			return string(ports.AIRunFailureCategoryCatalogBudget)
		case aierrors.CatalogData:
			return string(ports.AIRunFailureCategoryCatalogData)
		case aierrors.ModelOutput:
			return string(ports.AIRunFailureCategoryInvalidAIOutput)
		}
	}
	// Do not manufacture provider HTTP errors from ordinary Go errors.
	var status interface { ProviderHTTPStatus() int }
	if errors.As(err, &status) {
		switch code := status.ProviderHTTPStatus(); {
		case code == 429:
			return string(ports.AIRunFailureCategoryRateLimit)
		case code == 503 || code == 502 || code == 504:
			return string(ports.AIRunFailureCategoryProviderTemporary)
		default:
			return string(ports.AIRunFailureCategoryProviderPermanent)
		}
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		if networkError.Timeout() {
			return string(ports.AIRunFailureCategoryTimeout)
		}
		return string(ports.AIRunFailureCategoryNetwork)
	}
	return string(ports.AIRunFailureCategoryUnclassified)
}

// Stage is derived only from the originating failure type, never its message.
func classifyAIErrorStage(err error) string {
	var known *aierrors.Failure
	if errors.As(err, &known) {
		switch known.Kind {
		case aierrors.Application, aierrors.CatalogBudget:
			return ports.AIRunFailureStageContextBuild
		case aierrors.CatalogData, aierrors.ModelOutput:
			return ports.AIRunFailureStageValidation
		}
	}
	var status interface { ProviderHTTPStatus() int }
	var networkError net.Error
	if errors.As(err, &status) || errors.As(err, &networkError) || errors.Is(err, context.DeadlineExceeded) {
		return ports.AIRunFailureStageProviderRequest
	}
	return ports.AIRunFailureStageUnclassified
}

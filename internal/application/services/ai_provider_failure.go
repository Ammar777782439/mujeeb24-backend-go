package services

import (
	"context"
	"errors"
	"net"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// classifyAIProviderError keeps transport timeouts and transient gateway
// failures retryable in AI trace, not permanently attributed to the model.
// Native provider failover remains isolated from commercial execution.
func classifyAIProviderError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return string(ports.AIRunFailureCategoryTimeout)
	}
	if errors.Is(err, context.Canceled) {
		return string(ports.AIRunFailureCategoryInfrastructure)
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		if networkError.Timeout() {
			return string(ports.AIRunFailureCategoryTimeout)
		}
		return string(ports.AIRunFailureCategoryNetwork)
	}
	var temporary interface{ Retryable() bool }
	if errors.As(err, &temporary) && temporary.Retryable() {
		return string(ports.AIRunFailureCategoryProviderTemporary)
	}
	return string(ports.AIRunFailureCategoryProviderPermanent)
}

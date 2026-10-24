package ports

import (
	"context"
	"errors"
)

// ErrBusinessAIDisabled signals the merchant's explicit AI opt-out.
var ErrBusinessAIDisabled = errors.New("business AI is disabled")

// ErrBusinessAIPolicyUnavailable never authorizes a model call on read failure.
var ErrBusinessAIPolicyUnavailable = errors.New("business AI policy is unavailable")

// BusinessAIExecutionGuard checks whether model generation is permitted for a
// specific business. It does not authorize individual customer sales actions.
type BusinessAIExecutionGuard interface {
	Check(context.Context, string) error
}

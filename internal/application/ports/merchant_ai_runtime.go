package ports

import (
	"context"
)

// MerchantAIChatInput represents the context passed to the Merchant AI Copilot.
type MerchantAIChatInput struct {
	BusinessID     string
	PrincipalID    string
	SessionID      string
	CurrentMessage string
	History        []MerchantAIMessageRecord
}

// MerchantAIChatOutput represents the outcome of the Merchant AI Copilot's response.
type MerchantAIChatOutput struct {
	ResponseText string
	Action       string
	ToolCalls    []string
}

// MerchantAIRuntime defines the contract for communicating with LLM for the Merchant Copilot.
type MerchantAIRuntime interface {
	Chat(ctx context.Context, input MerchantAIChatInput) (MerchantAIChatOutput, error)
}

// Package services — Test fake for the customer-sales decision port.
//
// This fake implements ports.CustomerSalesDecisionPort so application/postgres tests can
// exercise the contract-aligned AutoReplyService flow without an HTTP Gemini
// call. It returns a fixed CapabilityDecision per contract ④ §4.
//
// Tests can override the returned proposal by setting FakeCustomerSalesDecisionPort.Decision.

package services

import (
	"context"
	"errors"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// FakeCustomerSalesDecisionPort is a test fake implementing ports.CustomerSalesDecisionPort.
//
// It returns the configured Proposal on every call, recording the input
// arguments for assertions. Tests may set Error to simulate a provider failure.
type FakeCustomerSalesDecisionPort struct {
	Decision            ports.CapabilityDecision
	GeminiInteractionID string // returned as ResultingInteractionID
	Error               error
	LastInput           ports.CustomerSalesDecisionInput
}

// NewFakeCustomerSalesDecisionPort returns a fake that produces a contract-aligned
// resolved/answer proposal with the given response text.
func NewFakeCustomerSalesDecisionPort(responseText string) *FakeCustomerSalesDecisionPort {
	return &FakeCustomerSalesDecisionPort{
		Decision: ports.CapabilityDecision{
			Capability:   ports.CustomerSalesCapabilityRespondWithoutAction,
			ResponseText: responseText,
		},
		GeminiInteractionID: "fake-interaction-id",
	}
}

// Decide implements ports.CustomerSalesDecisionPort.
func (f *FakeCustomerSalesDecisionPort) Decide(ctx context.Context, input ports.CustomerSalesDecisionInput) (ports.CustomerSalesDecisionOutput, error) {
	f.LastInput = input
	if f.Error != nil {
		return ports.CustomerSalesDecisionOutput{}, f.Error
	}
	if strings.TrimSpace(input.Request.Text) == "" {
		return ports.CustomerSalesDecisionOutput{}, errors.New("text is required")
	}
	return ports.CustomerSalesDecisionOutput{
		Decision: f.Decision,
		GeminiInteraction: ports.GeminiInteractionContext{
			PreviousInteractionID:  input.GeminiInteraction.PreviousInteractionID,
			ResultingInteractionID: f.GeminiInteractionID,
			Store:                  input.GeminiInteraction.Store,
		},
		Usage: ports.CustomerSalesUsageTelemetry{
			InputTokens:  10,
			OutputTokens: 5,
			Model:        "fake-model",
		},
		LatencyMs: 1,
	}, nil
}

var _ ports.CustomerSalesDecisionPort = (*FakeCustomerSalesDecisionPort)(nil)

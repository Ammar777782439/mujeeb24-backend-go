// Package services — Fake ContractRuntime for tests.
//
// This fake implements ports.ContractRuntime so application/postgres tests can
// exercise the contract-aligned AutoReplyService flow without an HTTP Gemini
// call. It returns a fixed AIGeminiProposal per contract ④ §4.
//
// Tests can override the returned proposal by setting FakeContractRuntime.Proposal.

package services

import (
	"context"
	"errors"
	"strings"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// FakeContractRuntime is a test fake implementing ports.ContractRuntime.
//
// It returns the configured Proposal on every call, recording the input
// arguments for assertions. Tests may set Error to simulate a provider failure.
type FakeContractRuntime struct {
	Proposal            ports.AIGeminiProposal
	GeminiInteractionID string // returned as ResultingInteractionID
	Error               error
	LastInput           ports.ContractRuntimeInput
}

// NewFakeContractRuntime returns a fake that produces a contract-aligned
// resolved/answer proposal with the given response text.
func NewFakeContractRuntime(responseText string) *FakeContractRuntime {
	return &FakeContractRuntime{
		Proposal: ports.AIGeminiProposal{
			Status:       ports.AIProposalStatusResolved,
			Action:       ports.AIProposalActionAnswer,
			ResponseText: responseText,
		},
		GeminiInteractionID: "fake-interaction-id",
	}
}

// DecideContract implements ports.ContractRuntime.
func (f *FakeContractRuntime) DecideContract(ctx context.Context, input ports.ContractRuntimeInput) (ports.ContractRuntimeOutput, error) {
	f.LastInput = input
	if f.Error != nil {
		return ports.ContractRuntimeOutput{}, f.Error
	}
	if strings.TrimSpace(input.DecisionInput.Text) == "" {
		return ports.ContractRuntimeOutput{}, errors.New("text is required")
	}
	return ports.ContractRuntimeOutput{
		Proposal: f.Proposal,
		GeminiInteraction: ports.GeminiInteractionContext{
			PreviousInteractionID:  input.GeminiInteraction.PreviousInteractionID,
			ResultingInteractionID: f.GeminiInteractionID,
			Store:                  input.GeminiInteraction.Store,
		},
		Usage: ports.ContractUsageTelemetry{
			InputTokens:  10,
			OutputTokens: 5,
			Model:        "fake-model",
		},
		LatencyMs: 1,
	}, nil
}

var _ ports.ContractRuntime = (*FakeContractRuntime)(nil)

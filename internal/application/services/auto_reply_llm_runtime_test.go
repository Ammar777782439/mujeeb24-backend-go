package services

import (
	"context"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// TestAutoReplyUsesStructuredLLMProposalBeforeEnqueue was the legacy test that
// used openaicompatible.Client directly. Per contract ④ §8, the AutoReplyService
// now uses ports.ContractRuntime (implemented by gemini.ContractClient). The
// OpenAI-compatible adapter does not yet implement ContractRuntime.
//
// The contract-aligned test that replaces this one is TestAutoReplyServicePersistsDecisionAndEnqueuesAnswerAtomically
// in auto_reply_test.go, which uses FakeContractRuntime.

func TestAutoReplyUsesContractRuntimeProposalBeforeEnqueue(t *testing.T) {
	decisions := &fakeDecisionRepository{}
	outbound := &fakeOutboundRepository{}
	outbox := &fakeOutboxStore{}
	// Use FakeContractRuntime which returns a contract-aligned AIGeminiProposal
	// per contract ④ §4 (status + action + response_text + selected[]).
	runtime := NewFakeContractRuntime("تم استلام رسالتك")
	service := NewAutoReplyService(
		runtime,
		decisions,
		fakeReferenceRepository{record: ports.ConversationReferenceRecord{ID: "reference-1", BusinessID: "business-1", ConversationID: "conversation-1", System: "provider", ProviderRef: "socialapi", ResourceID: "provider-conversation-1", ConnectionID: stringPtr("connection-1"), IsCurrent: true, MappingStatus: "active"}},
		outbound,
		outbox,
		fakeTransactionManager{},
	)

	result, err := service.Handle(context.Background(), commands.AutoReplyCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}}, ConversationID: "conversation-1", SourceMessageReference: "inbound-llm-1", Text: "مرحبا", Channel: "whatsapp", ProviderRef: "socialapi"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !result.Enqueued || result.Action != string(ports.AIProposalActionAnswer) || result.OutboundMessageID == "" || result.OutboxEntryID == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if outbox.calls != 1 {
		t.Fatalf("expected one outbox entry, got %d", outbox.calls)
	}
}

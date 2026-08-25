package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/ai/openaicompatible"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestAutoReplyUsesStructuredLLMProposalBeforeEnqueue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"intent_base\":\"information_request\",\"domain_context\":\"support\",\"entities\":{\"catalog_item_id\":\"\",\"variant_id\":\"\",\"quantity\":\"\",\"origin\":\"\",\"destination\":\"\",\"departure_date\":\"\",\"doctor_specialty\":\"\",\"preferred_time\":\"\",\"location\":\"\",\"phone\":\"\"},\"evidence_references\":[],\"requested_action\":\"answer\",\"response_text\":\"تم استلام رسالتك\",\"confidence_value\":\"0.9\",\"confidence_band\":\"high\",\"requires_human\":false,\"missing_information\":[],\"reason_codes\":[\"greeting\"],\"policy_decision\":\"allowed\",\"policy_version\":\"auto-reply-v1\",\"knowledge_version\":\"none\",\"schema_version\":1}"}}]}`))
	}))
	defer server.Close()

	runtime, err := openaicompatible.NewClient(openaicompatible.Config{BaseURL: server.URL, APIKey: "test-only-key", Model: "test-model"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	decisions := &fakeDecisionRepository{}
	outbound := &fakeOutboundRepository{}
	outbox := &fakeOutboxStore{}
	service := NewAutoReplyService(runtime, decisions, fakeReferenceRepository{record: ports.ConversationReferenceRecord{ID: "reference-1", BusinessID: "business-1", ConversationID: "conversation-1", System: "provider", ProviderRef: "socialapi", ResourceID: "provider-conversation-1", ConnectionID: stringPtr("connection-1"), IsCurrent: true, MappingStatus: "active"}}, outbound, outbox, fakeTransactionManager{})

	result, err := service.Handle(context.Background(), commands.AutoReplyCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}}, ConversationID: "conversation-1", SourceMessageReference: "inbound-llm-1", Text: "مرحبا", Channel: "whatsapp", ProviderRef: "socialapi"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !result.Enqueued || result.Action != AutoReplyActionAnswer || result.OutboundMessageID == "" || result.OutboxEntryID == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if decisions.record.ModelReference == nil || *decisions.record.ModelReference != "openai-compatible/test-model" {
		t.Fatalf("unexpected model reference: %#v", decisions.record.ModelReference)
	}
	if outbox.calls != 1 {
		t.Fatalf("expected one outbox entry, got %d", outbox.calls)
	}
}

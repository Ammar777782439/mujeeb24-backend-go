package services

import (
	"context"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type fakeMessageRepository struct {
	recorded ports.CommunicationMessageDraft
}

func (f *fakeMessageRepository) Record(_ context.Context, draft ports.CommunicationMessageDraft) (ports.CommunicationMessageRecord, error) {
	f.recorded = draft
	return ports.CommunicationMessageRecord{
		ID:                      draft.ID,
		BusinessID:              draft.BusinessID,
		ConversationReferenceID: draft.ConversationReferenceID,
		OutboundMessageID:       draft.OutboundMessageID,
		Direction:               draft.Direction,
		Origin:                  draft.Origin,
		Transport:               draft.Transport,
		ContentType:             draft.ContentType,
		TextContent:             draft.TextContent,
		ContentReference:        draft.ContentReference,
		Visibility:              draft.Visibility,
		OccurredAt:              draft.OccurredAt,
		CreatedAt:               draft.CreatedAt,
		Status:                  "pending",
	}, nil
}

func (f *fakeMessageRepository) ListByConversation(context.Context, string, string, int, string) (ports.MessagePage, error) {
	return ports.MessagePage{}, nil
}

type fakeChannelConnectionRepository struct {
	record ports.ChannelConnectionRecord
}

func (f *fakeChannelConnectionRepository) GetByID(_ context.Context, _, _ string) (ports.ChannelConnectionRecord, error) {
	return f.record, nil
}

func (f *fakeChannelConnectionRepository) GetByProviderReferences(_ context.Context, _, _, _ string) (ports.ChannelConnectionRecord, error) {
	return f.record, nil
}

func TestManualOutboundRecordsCommunicationMessageAndEnqueuesOutbox(t *testing.T) {
	connID := "conn-1"
	refRepo := fakeReferenceRepository{
		record: ports.ConversationReferenceRecord{
			ID:             "ref-1",
			BusinessID:     "biz-1",
			ConversationID: "conv-1",
			System:         "provider",
			ProviderRef:    "socialapi",
			ResourceID:     "res-1",
			ConnectionID:   &connID,
			IsCurrent:      true,
			MappingStatus:  "active",
		},
	}
	connRepo := &fakeChannelConnectionRepository{
		record: ports.ChannelConnectionRecord{
			ID:                "conn-1",
			BusinessID:        "biz-1",
			ProviderReference: "socialapi",
			Channel:           "whatsapp",
			Status:            "active",
		},
	}
	outboundRepo := &fakeOutboundRepository{}
	outboxStore := &fakeOutboxStore{}
	msgRepo := &fakeMessageRepository{}
	convRepo := &fakeConversationRuntimeRepository{}

	svc := ManualOutboundMessageService{
		References:    refRepo,
		Connections:   connRepo,
		Outbound:      outboundRepo,
		Outbox:        outboxStore,
		Messages:      msgRepo,
		Conversations: convRepo,
		Transactions:  fakeTransactionManager{},
	}

	cmd := commands.CreateOutboundMessageCommand{
		Meta: commands.CommandMeta{
			Actor:          commands.ActorContext{BusinessID: "biz-1"},
			IdempotencyKey: "idem-manual-1",
		},
		ConversationID: "conv-1",
		Text:           "مرحباً بك، كيف يمكنني مساعدتك؟",
	}

	result, err := svc.Handle(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Message.Text != cmd.Text {
		t.Errorf("expected result text %q, got %q", cmd.Text, result.Message.Text)
	}
	if outboundRepo.record.ID == "" || outboundRepo.record.Origin != "human" {
		t.Errorf("expected outbound created with origin 'human', got %#v", outboundRepo.record)
	}
	if outboxStore.record.OutboundMessageID != outboundRepo.record.ID {
		t.Errorf("expected outbox entry for outbound ID %s, got %s", outboundRepo.record.ID, outboxStore.record.OutboundMessageID)
	}
	if msgRepo.recorded.ID == "" || msgRepo.recorded.Origin != "human" || msgRepo.recorded.Direction != "outbound" {
		t.Errorf("expected communication message recorded with origin 'human' and direction 'outbound', got %#v", msgRepo.recorded)
	}
	if msgRepo.recorded.OutboundMessageID == nil || *msgRepo.recorded.OutboundMessageID != outboundRepo.record.ID {
		t.Errorf("expected communication message outbound_message_id %s, got %v", outboundRepo.record.ID, msgRepo.recorded.OutboundMessageID)
	}
	if len(convRepo.transitions) != 1 {
		t.Fatalf("expected 1 conversation transition, got %d", len(convRepo.transitions))
	}
	tr := convRepo.transitions[0]
	if tr.BusinessID != "biz-1" || tr.ConversationID != "conv-1" || tr.State == nil || *tr.State != "waiting_customer" || tr.Ownership == nil || *tr.Ownership != "human" {
		t.Errorf("unexpected conversation transition: %#v", tr)
	}
}

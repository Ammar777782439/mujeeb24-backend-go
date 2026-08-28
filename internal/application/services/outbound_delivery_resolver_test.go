package services

import (
	"context"

	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type resolverOutboundRepository struct{ record ports.OutboundMessageRecord }

func (r resolverOutboundRepository) CreatePending(context.Context, ports.OutboundMessageDraft) (ports.OutboundMessageRecord, error) {
	return ports.OutboundMessageRecord{}, nil
}
func (r resolverOutboundRepository) GetByID(context.Context, string, string) (ports.OutboundMessageRecord, error) {
	return r.record, nil
}

type resolverReferenceRepository struct {
	record ports.ConversationReferenceRecord
}

func (r resolverReferenceRepository) GetByID(context.Context, string, string) (ports.ConversationReferenceRecord, error) {
	return r.record, nil
}
func (r resolverReferenceRepository) GetCurrentByConversation(context.Context, string, string, string) (ports.ConversationReferenceRecord, error) {
	return r.record, nil
}

type resolverConnectionRepository struct{ record ports.ChannelConnectionRecord }

func (r resolverConnectionRepository) GetByID(context.Context, string, string) (ports.ChannelConnectionRecord, error) {
	return r.record, nil
}
func (r resolverConnectionRepository) GetByProviderReferences(context.Context, string, string, string) (ports.ChannelConnectionRecord, error) {
	return r.record, nil
}

func TestMujeebOutboundDeliveryResolverResolvesInlineText(t *testing.T) {
	account := "account-1"
	connection := "connection-1"
	resolver := MujeebOutboundDeliveryResolver{
		OutboundRepository:   resolverOutboundRepository{record: ports.OutboundMessageRecord{ID: "message-1", BusinessID: "business-1", ConversationID: "conversation-1", ConversationReferenceID: "reference-1", ConnectionID: connection, ProviderRef: "socialapi", Channel: "facebook", ContentReference: EncodeInlineTextContentReference("reply"), ProviderIdempotencyKey: "auto-reply:inbound-1", Status: "pending"}},
		ReferenceRepository:  resolverReferenceRepository{record: ports.ConversationReferenceRecord{ID: "reference-1", BusinessID: "business-1", ConversationID: "conversation-1", System: "provider", ProviderRef: "socialapi", ResourceType: "conversation", ResourceID: "provider-conversation-1", ConnectionID: &connection, IsCurrent: true, MappingStatus: "active"}},
		ConnectionRepository: resolverConnectionRepository{record: ports.ChannelConnectionRecord{ID: connection, BusinessID: "business-1", ProviderReference: "socialapi", Channel: "facebook", ProviderAccountReference: &account, Status: "active"}},
	}
	delivery, err := resolver.Resolve(context.Background(), "business-1", "message-1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if delivery.ConnectionID != connection || delivery.ProviderAccountID != account || delivery.ProviderConversationID != "provider-conversation-1" || delivery.Text != "reply" || delivery.IdempotencyKey != "auto-reply:inbound-1" {
		t.Fatalf("unexpected delivery: %#v", delivery)
	}
}

func TestMujeebOutboundDeliveryResolverRejectsInactiveConnection(t *testing.T) {
	account := "account-1"
	connection := "connection-1"
	resolver := MujeebOutboundDeliveryResolver{
		OutboundRepository:   resolverOutboundRepository{record: ports.OutboundMessageRecord{ID: "message-1", BusinessID: "business-1", ConversationReferenceID: "reference-1", ConnectionID: connection, ProviderRef: "socialapi", Channel: "facebook", ContentReference: EncodeInlineTextContentReference("reply"), ProviderIdempotencyKey: "idem-1", Status: "pending"}},
		ReferenceRepository:  resolverReferenceRepository{record: ports.ConversationReferenceRecord{ID: "reference-1", System: "provider", ProviderRef: "socialapi", ResourceID: "provider-conversation-1", ConnectionID: &connection, IsCurrent: true}},
		ConnectionRepository: resolverConnectionRepository{record: ports.ChannelConnectionRecord{ID: connection, ProviderReference: "socialapi", Channel: "facebook", ProviderAccountReference: &account, Status: "disconnected"}},
	}
	if _, err := resolver.Resolve(context.Background(), "business-1", "message-1"); err == nil {
		t.Fatal("expected inactive connection rejection")
	}
}

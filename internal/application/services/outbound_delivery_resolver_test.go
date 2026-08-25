package services

import (
	"context"
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
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
func (r resolverReferenceRepository) GetCurrentProviderByChatwoot(context.Context, string, string, string, string) (ports.ConversationReferenceRecord, error) {
	return r.record, nil
}
func (r resolverReferenceRepository) BindProviderToChatwoot(context.Context, ports.ProviderChatwootBindingDraft) (ports.ConversationReferenceRecord, error) {
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

type resolverErrorReferenceRepository struct{ err error }

func (r resolverErrorReferenceRepository) GetByID(context.Context, string, string) (ports.ConversationReferenceRecord, error) {
	return ports.ConversationReferenceRecord{}, r.err
}
func (r resolverErrorReferenceRepository) GetCurrentProviderByChatwoot(context.Context, string, string, string, string) (ports.ConversationReferenceRecord, error) {
	return ports.ConversationReferenceRecord{}, r.err
}
func (r resolverErrorReferenceRepository) BindProviderToChatwoot(context.Context, ports.ProviderChatwootBindingDraft) (ports.ConversationReferenceRecord, error) {
	return ports.ConversationReferenceRecord{}, r.err
}
func (r resolverErrorReferenceRepository) GetCurrentByConversation(context.Context, string, string, string) (ports.ConversationReferenceRecord, error) {
	return ports.ConversationReferenceRecord{}, r.err
}

func providerReferenceBindingFixtures() (ports.ConversationReferenceRecord, ports.ChannelConnectionRecord) {
	connectionID := "connection-1"
	accountID := "account-1"
	return ports.ConversationReferenceRecord{ID: "reference-1", BusinessID: "business-1", ConversationID: "conversation-1", System: "provider", ProviderRef: "socialapi", ResourceType: "conversation", ResourceID: "provider-conversation-1", ConnectionID: &connectionID, IsCurrent: true, MappingStatus: "active", ChatwootAccountID: &accountID, ChatwootInboxID: stringPtr("inbox-1"), ChatwootConversationID: stringPtr("chatwoot-conversation-1")}, ports.ChannelConnectionRecord{ID: connectionID, BusinessID: "business-1", ProviderReference: "socialapi", Channel: "facebook", ProviderConnectionRef: "provider-connection-1", ProviderAccountReference: &accountID, Status: "active"}
}

func TestChatwootProviderReferenceResolverResolvesValidBinding(t *testing.T) {
	reference, connection := providerReferenceBindingFixtures()
	resolver := ChatwootProviderReferenceResolver{References: resolverReferenceRepository{record: reference}, Connections: resolverConnectionRepository{record: connection}}
	binding, err := resolver.Resolve(context.Background(), ChatwootProviderReferenceInput{BusinessID: "business-1", AccountID: "account-1", InboxID: "inbox-1", ChatwootConversationID: "chatwoot-conversation-1"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if binding.MujeebConversationID != "conversation-1" || binding.ProviderRef != "socialapi" || binding.Channel != "facebook" || binding.ConnectionID != "connection-1" || binding.ProviderAccountID != "account-1" || binding.ProviderConversationID != "provider-conversation-1" {
		t.Fatalf("unexpected binding: %#v", binding)
	}
}

func TestChatwootProviderReferenceResolverRejectsMissingProviderReference(t *testing.T) {
	resolver := ChatwootProviderReferenceResolver{References: resolverErrorReferenceRepository{err: &repositoryError{kind: "not_found"}}, Connections: resolverConnectionRepository{}}
	_, err := resolver.Resolve(context.Background(), ChatwootProviderReferenceInput{BusinessID: "business-1", AccountID: "account-1", InboxID: "inbox-1", ChatwootConversationID: "chatwoot-conversation-1"})
	if err == nil || bindingBlockReason(err) != "missing_provider_conversation_reference" {
		t.Fatalf("expected missing provider reference, err=%v", err)
	}
}

func TestChatwootProviderReferenceResolverRejectsStaleAndMismatchedBinding(t *testing.T) {
	reference, connection := providerReferenceBindingFixtures()
	reference.IsCurrent = false
	resolver := ChatwootProviderReferenceResolver{References: resolverReferenceRepository{record: reference}, Connections: resolverConnectionRepository{record: connection}}
	if _, err := resolver.Resolve(context.Background(), ChatwootProviderReferenceInput{BusinessID: "business-1", AccountID: "account-1", InboxID: "inbox-1", ChatwootConversationID: "chatwoot-conversation-1"}); err == nil {
		t.Fatal("expected stale reference rejection")
	}

	reference, connection = providerReferenceBindingFixtures()
	connection.ProviderReference = "chatwoot"
	resolver = ChatwootProviderReferenceResolver{References: resolverReferenceRepository{record: reference}, Connections: resolverConnectionRepository{record: connection}}
	if _, err := resolver.Resolve(context.Background(), ChatwootProviderReferenceInput{BusinessID: "business-1", AccountID: "account-1", InboxID: "inbox-1", ChatwootConversationID: "chatwoot-conversation-1"}); err == nil {
		t.Fatal("expected connection mismatch rejection")
	}

	reference, connection = providerReferenceBindingFixtures()
	resolver = ChatwootProviderReferenceResolver{References: resolverReferenceRepository{record: reference}, Connections: resolverConnectionRepository{record: connection}}
	if _, err := resolver.Resolve(context.Background(), ChatwootProviderReferenceInput{BusinessID: "business-2", AccountID: "account-1", InboxID: "inbox-1", ChatwootConversationID: "chatwoot-conversation-1"}); err == nil {
		t.Fatal("expected tenant mismatch rejection or no match")
	}
}

func TestChatwootAutoReplyBridgeAllowsOutboxOnlyForValidBinding(t *testing.T) {
	reference, connection := providerReferenceBindingFixtures()
	references := fakeReferenceRepository{record: reference}
	outbox := &fakeOutboxStore{}
	autoReply := NewAutoReplyService(SafeAutoReplyRuntime{}, &fakeDecisionRepository{}, references, &fakeOutboundRepository{}, outbox, fakeTransactionManager{})
	bridge := ChatwootAutoReplyBridge{Resolver: ChatwootProviderReferenceResolver{References: references, Connections: resolverConnectionRepository{record: connection}}, AutoReply: autoReply}
	result, err := bridge.Handle(context.Background(), commands.ChatwootAutoReplyCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}}, AccountID: "account-1", InboxID: "inbox-1", ChatwootConversationID: "chatwoot-conversation-1", MujeebConversationID: "conversation-1", SourceMessageReference: "chatwoot-message-1", Text: "مرحبا"})
	if err != nil || !result.Executed || result.Blocked || !result.AutoReply.Enqueued || outbox.calls != 1 {
		t.Fatalf("unexpected bridge result=%#v err=%v outbox_calls=%d", result, err, outbox.calls)
	}

	blockedBridge := ChatwootAutoReplyBridge{Resolver: ChatwootProviderReferenceResolver{References: resolverErrorReferenceRepository{err: &repositoryError{kind: "not_found"}}, Connections: resolverConnectionRepository{record: connection}}, AutoReply: autoReply}
	blocked, err := blockedBridge.Handle(context.Background(), commands.ChatwootAutoReplyCommand{Meta: commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}}, AccountID: "account-1", InboxID: "inbox-1", ChatwootConversationID: "chatwoot-conversation-1", MujeebConversationID: "conversation-1", SourceMessageReference: "chatwoot-message-2", Text: "مرحبا"})
	if err != nil || !blocked.Blocked || blocked.Executed || blocked.BlockedReason != "missing_provider_conversation_reference" {
		t.Fatalf("unexpected blocked bridge result=%#v err=%v", blocked, err)
	}
}

type repositoryError struct{ kind string }

func (e *repositoryError) Error() string     { return e.kind }
func (e *repositoryError) ErrorKind() string { return e.kind }

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type outboundMediaCapabilityFake struct {
	records []ports.ChannelCapabilityRecord
	err     error
}

func (f outboundMediaCapabilityFake) ListByConnection(context.Context, string, string) ([]ports.ChannelCapabilityRecord, error) {
	return f.records, f.err
}

type outboundMediaAssetFake struct {
	asset ports.MediaAsset
	err   error
}

func (f outboundMediaAssetFake) GetAsset(context.Context, string, string) (ports.MediaAsset, error) {
	return f.asset, f.err
}

type outboundMessageAttachmentsFake struct {
	created []ports.MessageAttachmentDraft
	items   []ports.OutboundMessageAttachmentRecord
	err     error
}

func (f *outboundMessageAttachmentsFake) CreateReadyOutbound(_ context.Context, draft ports.MessageAttachmentDraft) (ports.MessageAttachmentRecord, error) {
	if f.err != nil {
		return ports.MessageAttachmentRecord{}, f.err
	}
	f.created = append(f.created, draft)
	return ports.MessageAttachmentRecord{
		ID: draft.ID, BusinessID: draft.BusinessID,
		CommunicationMessageID: draft.CommunicationMessageID,
		MediaAssetID:           draft.MediaAssetID, Kind: draft.Kind,
		Status: "ready", SortOrder: draft.SortOrder, CreatedAt: draft.CreatedAt,
	}, nil
}

func (f *outboundMessageAttachmentsFake) ListForOutboundMessage(context.Context, string, string) ([]ports.OutboundMessageAttachmentRecord, error) {
	return f.items, f.err
}

type outboundMediaPresignerFake struct {
	provider string
	key      string
	ttl      time.Duration
	url      string
	err      error
}

func (f *outboundMediaPresignerFake) PresignGetFromProvider(_ context.Context, provider, key string, ttl time.Duration) (string, error) {
	f.provider, f.key, f.ttl = provider, key, ttl
	return f.url, f.err
}

func verifiedOutboundMediaCapability() []ports.ChannelCapabilityRecord {
	evidence := "verified-provider-capability"
	return []ports.ChannelCapabilityRecord{{
		ConnectionID: "connection-1", Name: "media_outbound", Enabled: true,
		CheckedAt:      time.Date(2026, 11, 10, 10, 0, 0, 0, time.UTC),
		EvidenceSource: &evidence,
	}}
}

func TestHasVerifiedChannelCapabilityFailsClosed(t *testing.T) {
	valid := verifiedOutboundMediaCapability()
	if !hasVerifiedChannelCapability(valid, "media_outbound") {
		t.Fatal("expected enabled, checked and evidenced capability")
	}
	if hasVerifiedChannelCapability(nil, "media_outbound") {
		t.Fatal("missing capability must fail closed")
	}
	disabled := append([]ports.ChannelCapabilityRecord(nil), valid...)
	disabled[0].Enabled = false
	if hasVerifiedChannelCapability(disabled, "media_outbound") {
		t.Fatal("disabled capability must fail closed")
	}
	unchecked := append([]ports.ChannelCapabilityRecord(nil), valid...)
	unchecked[0].CheckedAt = time.Time{}
	if hasVerifiedChannelCapability(unchecked, "media_outbound") {
		t.Fatal("unchecked capability must fail closed")
	}
	unevidenced := append([]ports.ChannelCapabilityRecord(nil), valid...)
	unevidenced[0].EvidenceSource = nil
	if hasVerifiedChannelCapability(unevidenced, "media_outbound") {
		t.Fatal("capability without evidence must fail closed")
	}
}

func outboundMediaResolver(attachments []ports.OutboundMessageAttachmentRecord, capabilities []ports.ChannelCapabilityRecord, presigner *outboundMediaPresignerFake) MujeebOutboundDeliveryResolver {
	account := "account-1"
	connection := "connection-1"
	_ = attachments
	return MujeebOutboundDeliveryResolver{
		OutboundRepository: resolverOutboundRepository{record: ports.OutboundMessageRecord{
			ID: "message-1", BusinessID: "business-1", ConversationID: "conversation-1",
			ConversationReferenceID: "reference-1", ConnectionID: connection,
			ProviderRef: "socialapi", Channel: "whatsapp",
			ContentReference:       EncodeInlineTextContentReference("reply"),
			ProviderIdempotencyKey: "idem-1", Status: "pending",
		}},
		ReferenceRepository: resolverReferenceRepository{record: ports.ConversationReferenceRecord{
			ID: "reference-1", BusinessID: "business-1", ConversationID: "conversation-1",
			System: "provider", ProviderRef: "socialapi", ResourceType: "conversation",
			ResourceID: "provider-conversation-1", ConnectionID: &connection,
			IsCurrent: true, MappingStatus: "active",
		}},
		ConnectionRepository: resolverConnectionRepository{record: ports.ChannelConnectionRecord{
			ID: connection, BusinessID: "business-1", ProviderReference: "socialapi",
			Channel: "whatsapp", ProviderAccountReference: &account, Status: "active",
		}},
		ConversationAttachments:  &outboundMessageAttachmentsFake{items: attachments},
		ConversationMediaObjects: presigner,
		Capabilities:             outboundMediaCapabilityFake{records: capabilities},
	}
}

func TestOutboundResolverPresignsConversationImageFromRecordedProvider(t *testing.T) {
	presigner := &outboundMediaPresignerFake{url: "https://private.example/signed/image"}
	resolver := outboundMediaResolver([]ports.OutboundMessageAttachmentRecord{{
		AttachmentID: "attachment-1", BusinessID: "business-1",
		CommunicationMessageID: "communication-1", OutboundMessageID: "message-1",
		MediaAssetID: "media-1", Kind: "image", MimeType: "image/png",
		Status: "ready", StorageProvider: "backblaze",
		StorageKey:   "conversation/business-1/media-1.png",
		AssetPurpose: "conversation_attachment", AssetStatus: "ready",
	}}, verifiedOutboundMediaCapability(), presigner)

	delivery, err := resolver.Resolve(context.Background(), "business-1", "message-1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(delivery.Attachments) != 1 || delivery.Attachments[0].Type != "image" ||
		delivery.Attachments[0].URL != presigner.url {
		t.Fatalf("unexpected delivery attachments: %#v", delivery.Attachments)
	}
	if presigner.provider != "backblaze" || presigner.key != "conversation/business-1/media-1.png" ||
		presigner.ttl != 45*time.Minute {
		t.Fatalf("read did not use recorded storage provider/key/TTL: %#v", presigner)
	}
}

func TestOutboundResolverRejectsConversationImageWithoutVerifiedCapability(t *testing.T) {
	presigner := &outboundMediaPresignerFake{url: "https://private.example/signed/image"}
	resolver := outboundMediaResolver([]ports.OutboundMessageAttachmentRecord{{
		AttachmentID: "attachment-1", BusinessID: "business-1", OutboundMessageID: "message-1",
		MediaAssetID: "media-1", Kind: "image", MimeType: "image/jpeg", Status: "ready",
		StorageProvider: "r2", StorageKey: "conversation/business-1/media-1.jpg",
		AssetPurpose: "conversation_attachment", AssetStatus: "ready",
	}}, nil, presigner)
	if _, err := resolver.Resolve(context.Background(), "business-1", "message-1"); !errors.Is(err, ErrRequiredConversationMediaUnavailable) {
		t.Fatalf("expected fail-closed media capability error, got %v", err)
	}
	if presigner.provider != "" {
		t.Fatal("storage URL must not be generated when channel capability is unverified")
	}
}

func TestManualOutboundLinksTenantOwnedReadyMediaInsideOutboxTransaction(t *testing.T) {
	const mediaID = "11111111-1111-4111-8111-111111111111"
	connectionID := "conn-1"
	references := fakeReferenceRepository{record: ports.ConversationReferenceRecord{
		ID: "ref-1", BusinessID: "biz-1", ConversationID: "conv-1",
		System: "provider", ProviderRef: "socialapi", ResourceID: "provider-conv-1",
		ConnectionID: &connectionID, IsCurrent: true, MappingStatus: "active",
	}}
	connections := &fakeChannelConnectionRepository{record: ports.ChannelConnectionRecord{
		ID: connectionID, BusinessID: "biz-1", ProviderReference: "socialapi",
		Channel: "whatsapp", Status: "active",
	}}
	outbound := &fakeOutboundRepository{}
	outbox := &fakeOutboxStore{}
	messages := &fakeMessageRepository{}
	attachments := &outboundMessageAttachmentsFake{}
	service := ManualOutboundMessageService{
		Inbox: unitOwnedInbox{}, References: references, Connections: connections,
		Capabilities: outboundMediaCapabilityFake{records: verifiedOutboundMediaCapability()},
		MediaAssets: outboundMediaAssetFake{asset: ports.MediaAsset{
			ID: mediaID, BusinessID: "biz-1", Purpose: "conversation_attachment",
			Kind: "image", MimeType: "image/jpeg", Status: "ready",
			StorageProvider: "backblaze", StorageKey: "conversation/biz-1/photo.jpg",
		}},
		Attachments: attachments, Outbound: outbound, Outbox: outbox, Messages: messages,
		Conversations: &fakeConversationRuntimeRepository{}, Transactions: fakeTransactionManager{},
	}
	result, err := service.Handle(context.Background(), commands.CreateOutboundMessageCommand{
		Meta: commands.CommandMeta{
			Actor:          commands.ActorContext{BusinessID: "biz-1", PrincipalID: "agent-1", Role: "owner", Permissions: []string{"*"}},
			IdempotencyKey: "reply-with-photo-1",
		},
		ConversationID: "conv-1", Text: "هذه صورة المنتج",
		AttachmentMediaIDs: []string{mediaID},
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if outbound.record.ID == "" || messages.recorded.OutboundMessageID == nil ||
		*messages.recorded.OutboundMessageID != outbound.record.ID ||
		outbox.record.OutboundMessageID != outbound.record.ID || outbox.calls != 1 {
		t.Fatalf("outbound, message and outbox were not linked: outbound=%#v message=%#v outbox=%#v", outbound.record, messages.recorded, outbox.record)
	}
	if messages.recorded.ContentType != "mixed" || len(attachments.created) != 1 ||
		attachments.created[0].CommunicationMessageID != messages.recorded.ID ||
		attachments.created[0].MediaAssetID != mediaID || len(result.Message.Attachments) != 1 ||
		result.Message.Attachments[0].Status != "ready" {
		t.Fatalf("conversation image was not attached to the public message: msg=%#v attachments=%#v result=%#v",
			messages.recorded, attachments.created, result.Message)
	}
}

func TestManualOutboundRejectsImageWhenMediaOutboundCapabilityIsNotVerified(t *testing.T) {
	const mediaID = "11111111-1111-4111-8111-111111111111"
	connectionID := "conn-1"
	outbound := &fakeOutboundRepository{}
	outbox := &fakeOutboxStore{}
	service := ManualOutboundMessageService{
		Inbox: unitOwnedInbox{},
		References: fakeReferenceRepository{record: ports.ConversationReferenceRecord{
			ID: "ref-1", BusinessID: "biz-1", ConversationID: "conv-1",
			System: "provider", ProviderRef: "socialapi", ResourceID: "provider-conv-1",
			ConnectionID: &connectionID, IsCurrent: true, MappingStatus: "active",
		}},
		Connections: &fakeChannelConnectionRepository{record: ports.ChannelConnectionRecord{
			ID: connectionID, BusinessID: "biz-1", ProviderReference: "socialapi",
			Channel: "whatsapp", Status: "active",
		}},
		Capabilities: outboundMediaCapabilityFake{},
		MediaAssets: outboundMediaAssetFake{asset: ports.MediaAsset{
			ID: mediaID, BusinessID: "biz-1", Purpose: "conversation_attachment",
			Kind: "image", MimeType: "image/png", Status: "ready",
			StorageProvider: "r2", StorageKey: "conversation/biz-1/photo.png",
		}},
		Attachments: &outboundMessageAttachmentsFake{}, Outbound: outbound,
		Outbox: outbox, Messages: &fakeMessageRepository{}, Transactions: fakeTransactionManager{},
	}
	_, err := service.Handle(context.Background(), commands.CreateOutboundMessageCommand{
		Meta: commands.CommandMeta{
			Actor:          commands.ActorContext{BusinessID: "biz-1", PrincipalID: "agent-1", Role: "owner", Permissions: []string{"*"}},
			IdempotencyKey: "reply-with-unverified-media-1",
		},
		ConversationID: "conv-1", Text: "صورة",
		AttachmentMediaIDs: []string{mediaID},
	})
	if err == nil || outbound.record.ID != "" || outbox.calls != 0 {
		t.Fatalf("unverified outbound media must not create an outbound obligation: err=%v outbound=%#v outbox=%#v", err, outbound.record, outbox)
	}
}

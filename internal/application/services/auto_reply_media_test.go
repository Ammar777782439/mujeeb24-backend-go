package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type galleryMediaRepo struct {
	ports.CatalogMediaRepository
	links []ports.CatalogMediaLink
}

func (r galleryMediaRepo) ListGallery(_ context.Context, businessID, itemID string, variantID *string) ([]ports.CatalogMediaLink, error) {
	if businessID != "business-1" || itemID != "phone" || !mediaVariantMatches(variantID, stringPtr("blue")) {
		return nil, fmt.Errorf("incorrect gallery scope")
	}
	return r.links, nil
}
func (r galleryMediaRepo) FindApprovedMediaLink(_ context.Context, businessID, id string) (*ports.CatalogMediaLink, error) {
	if businessID != "business-1" {
		return nil, fmt.Errorf("wrong tenant")
	}
	for _, link := range r.links {
		if link.ID == id {
			photo := link
			return &photo, nil
		}
	}
	return nil, nil
}

type photoSelectionFake struct {
	ids []string
}

func (m photoSelectionFake) SelectMedia(_ context.Context, _ ports.CustomerMediaSelectionInput) (ports.CustomerMediaSelectionOutput, error) {
	return ports.CustomerMediaSelectionOutput{MediaLinkIDs: m.ids}, nil
}

type photoRecordingOutbound struct {
	fakeOutboundRepository
	drafts []ports.OutboundMessageDraft
}

func (r *photoRecordingOutbound) CreatePending(ctx context.Context, draft ports.OutboundMessageDraft) (ports.OutboundMessageRecord, error) {
	r.drafts = append(r.drafts, draft)
	return r.fakeOutboundRepository.CreatePending(ctx, draft)
}

func photoTestService(ids []string) (AutoReplyService, *photoRecordingOutbound, *fakeOutboxStore) {
	images := make([]ports.CatalogMediaLink, 0, 4)
	now := time.Now().UTC()
	for i := 1; i <= 4; i++ {
		images = append(images, ports.CatalogMediaLink{
			ID: fmt.Sprintf("photo-%d", i), BusinessID: "business-1",
			CatalogItemID: "phone", VariantID: stringPtr("blue"), MediaID: fmt.Sprintf("asset-%d", i),
			Status: "active", ApprovedAt: &now, IsPrimary: i == 1,
			AltText: fmt.Sprintf("blue phone view %d", i),
		})
	}
	outbound := &photoRecordingOutbound{}
	outbox := &fakeOutboxStore{}
	service := NewAutoReplyService(
		&FakeCustomerSalesDecisionPort{Decision: ports.CapabilityDecision{
			Capability:   ports.CustomerSalesCapabilityRespondWithoutAction,
			ResponseText: "هذه صور الهاتف الأزرق",
			Selected:     []ports.SelectedReference{{ItemID: "phone", VariantID: stringPtr("blue")}},
		}},
		&fakeDecisionRepository{},
		fakeReferenceRepository{record: ports.ConversationReferenceRecord{
			ID: "reference", BusinessID: "business-1", ConversationID: "conversation-1",
			ProviderRef: "socialapi", ResourceID: "external-conversation",
			ConnectionID: stringPtr("connection"), IsCurrent: true,
		}},
		outbound, outbox, fakeTransactionManager{},
	)
	service.CatalogMedia = galleryMediaRepo{links: images}
	service.MediaSelection = photoSelectionFake{ids: ids}
	service.Validation = allowAllValidationPipeline()
	service.AIUsage = &stubAIUsageRepo{}
	service.Subscriptions = &stubSubscriptionsRepo{items: []ports.SubscriptionRecord{{ID: "sub", BusinessID: "business-1", Status: "ACTIVE"}}}
	service.AIPricing = &stubPricingRepoAlwaysFail{}
	return service, outbound, outbox
}

func TestAutoReplyOneTurnCreatesFourIndividuallyTrackedPhotos(t *testing.T) {
	service, outbound, outbox := photoTestService([]string{"photo-1", "photo-2", "photo-3", "photo-4"})
	result, err := service.Handle(context.Background(), commands.AutoReplyCommand{
		Meta:           commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}},
		ConversationID: "conversation-1", SourceMessageReference: "inbound-photos",
		Text: "ارسل لي أربع صور من الهاتف الأزرق", Channel: "whatsapp", ProviderRef: "socialapi",
	})
	if err != nil {
		t.Fatalf("handle four photos: %v", err)
	}
	if !result.Enqueued || outbox.calls != 4 || len(outbound.drafts) != 4 {
		t.Fatalf("expected four outbounds for one AI turn: result=%+v outbox=%d drafts=%d", result, outbox.calls, len(outbound.drafts))
	}
	seen := map[string]bool{}
	for i, draft := range outbound.drafts {
		if seen[draft.ProviderIdempotencyKey] {
			t.Fatalf("duplicate provider key at image %d", i)
		}
		seen[draft.ProviderIdempotencyKey] = true
		msg, err := DecodeInteractiveContentReference(draft.ContentReference)
		if err != nil || msg.MediaID != fmt.Sprintf("asset-%d", i+1) ||
			msg.MediaLinkID != fmt.Sprintf("photo-%d", i+1) ||
			msg.MediaRequired != (i > 0) {
			t.Fatalf("image %d has invalid envelope: %+v err=%v", i+1, msg, err)
		}
	}
}

func TestAutoReplyMediaSelectionInvalidIDFallsBackToTextOnly(t *testing.T) {
	for _, ids := range [][]string{nil, {"wrong-merchant-photo"}, {"photo-1", "photo-1"}} {
		service, outbound, outbox := photoTestService(ids)
		result, err := service.Handle(context.Background(), commands.AutoReplyCommand{
			Meta:           commands.CommandMeta{Actor: commands.ActorContext{BusinessID: "business-1"}},
			ConversationID: "conversation-1", SourceMessageReference: "inbound-one",
			Text: "صور الهاتف الأزرق", Channel: "whatsapp", ProviderRef: "socialapi",
		})
		if err != nil || !result.Enqueued || outbox.calls != 1 || len(outbound.drafts) != 1 {
			t.Fatalf("invalid media selection leaked to outbox: ids=%v result=%+v err=%v", ids, result, err)
		}
		msg, err := DecodeInteractiveContentReference(outbound.drafts[0].ContentReference)
		if err != nil || msg.MediaID != "" {
			t.Fatalf("invalid media selection sent image: ids=%v msg=%+v err=%v", ids, msg, err)
		}
	}
}

func TestMediaVariantMatcherNeverMixesPhoneColours(t *testing.T) {
	if mediaVariantMatches(stringPtr("blue"), stringPtr("red")) ||
		mediaVariantMatches(nil, stringPtr("blue")) ||
		!mediaVariantMatches(stringPtr("blue"), stringPtr("blue")) {
		t.Fatal("variant gallery must only match the exact selected variant")
	}
}

func TestResolverDoesNotSendCaptionAloneWhenRequiredImageUnavailable(t *testing.T) {
	account, connection := "account", "connection"
	ref, err := EncodeInteractiveContentReference(ports.InteractiveMessage{
		Text: "صورة 2 من 4", MediaID: "asset-2",
		MediaLinkID: "photo-2", MediaRequired: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	resolver := MujeebOutboundDeliveryResolver{
		OutboundRepository: resolverOutboundRepository{record: ports.OutboundMessageRecord{
			ID: "outbound", BusinessID: "business-1", ConversationReferenceID: "ref",
			ConnectionID: connection, ProviderRef: "socialapi", Channel: "whatsapp",
			ContentReference: ref, ProviderIdempotencyKey: "photo-send", Status: "pending",
		}},
		ReferenceRepository: resolverReferenceRepository{record: ports.ConversationReferenceRecord{
			ID: "ref", System: "provider", ProviderRef: "socialapi",
			ResourceID: "provider-conversation", ConnectionID: &connection, IsCurrent: true,
		}},
		ConnectionRepository: resolverConnectionRepository{record: ports.ChannelConnectionRecord{
			ID: connection, ProviderReference: "socialapi", Channel: "whatsapp",
			ProviderAccountReference: &account, Status: "active",
		}},
	}
	if _, err := resolver.Resolve(context.Background(), "business-1", "outbound"); err == nil {
		t.Fatal("missing media storage must not send a caption-only image message")
	}
}

package services

import (
	"context"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type webhookHistoryGuardFake struct {
	ignore       bool
	calls        int
	businessID   string
	connectionID string
	occurredAt   *time.Time
}

func (f *webhookHistoryGuardFake) ShouldIgnore(_ context.Context, businessID, connectionID string, occurredAt *time.Time) (bool, error) {
	f.calls++
	f.businessID, f.connectionID, f.occurredAt = businessID, connectionID, occurredAt
	return f.ignore, nil
}

var _ ports.ChannelHistoryGuard = (*webhookHistoryGuardFake)(nil)

func TestWebhookHistoryPurgeDoesNotPersistOldMessageOrRawBody(t *testing.T) {
	raw := &webhookRawPayloadStore{}
	events := &webhookEventStore{created: true}
	inbound := &providerInboundStore{}
	guard := &webhookHistoryGuardFake{ignore: true}
	service := resolvedSocialWebhookService(inbound)
	service.RawPayloads = raw
	service.Events = events
	service.History = guard
	body := []byte(`{"event":"dm.received","data":{"id":"event-old","type":"dm","platform":"instagram","account_id":"account-1","conversation_id":"conversation-1","author":{"id":"customer-1"},"content":{"text":"secret deleted conversation"}}}`)
	result, err := service.Handle(context.Background(), signedSocialCommand(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Accepted || !result.Ignored || guard.calls != 1 || guard.businessID != "business-1" || guard.connectionID != "connection-1" {
		t.Fatalf("history guard not applied: result=%+v guard=%+v", result, guard)
	}
	if raw.payload != nil || events.draft.ID != "" || inbound.draft.InboundEventID != "" {
		t.Fatalf("old content was persisted despite cutoff: raw=%q event=%+v inbound=%+v", raw.payload, events.draft, inbound.draft)
	}
}

func TestWebhookHistoryPurgeAllowsNewMessage(t *testing.T) {
	raw := &webhookRawPayloadStore{}
	events := &webhookEventStore{created: true}
	inbound := &providerInboundStore{result: ports.ProviderInboundResult{BusinessID: "business-1", ConversationID: "new-conversation", CommunicationMessageID: "new-message"}}
	guard := &webhookHistoryGuardFake{ignore: false}
	service := resolvedSocialWebhookService(inbound)
	service.RawPayloads = raw
	service.Events = events
	service.History = guard
	body := []byte(`{"event":"dm.received","data":{"id":"event-new","type":"dm","platform":"instagram","account_id":"account-1","conversation_id":"conversation-2","author":{"id":"customer-1"},"content":{"text":"brand new"}}}`)
	result, err := service.Handle(context.Background(), signedSocialCommand(t, body))
	if err != nil || !result.Accepted || result.Ignored || events.draft.ID == "" || inbound.draft.InboundEventID == "" || guard.calls != 1 {
		t.Fatalf("new message rejected: result=%+v event=%+v err=%v", result, events.draft, err)
	}
}

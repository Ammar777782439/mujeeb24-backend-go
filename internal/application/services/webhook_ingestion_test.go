package services

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/providers/socialapi"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type webhookRawPayloadStore struct {
	payload   []byte
	reference string
	sha256    string
}

func (s *webhookRawPayloadStore) Put(_ context.Context, provider, deliveryID string, payload []byte) (ports.RawPayload, error) {
	digest := sha256.Sum256(payload)
	s.payload = append([]byte(nil), payload...)
	s.reference = provider + "/" + deliveryID
	s.sha256 = hex.EncodeToString(digest[:])
	return ports.RawPayload{Reference: s.reference, SHA256: s.sha256}, nil
}

type webhookConnectionRepository struct {
	record ports.ChannelConnectionRecord
	err    error
}

type repositoryKindError string

func (e repositoryKindError) Error() string     { return string(e) }
func (e repositoryKindError) ErrorKind() string { return string(e) }

func (r webhookConnectionRepository) GetByID(context.Context, string, string) (ports.ChannelConnectionRecord, error) {
	return ports.ChannelConnectionRecord{}, errors.New("not used")
}
func (r webhookConnectionRepository) GetByProviderReferences(context.Context, string, string, string) (ports.ChannelConnectionRecord, error) {
	return r.record, r.err
}

type webhookEventStore struct {
	draft   ports.InboundEventDraft
	created bool
}

func (s *webhookEventStore) RecordIfAbsent(_ context.Context, draft ports.InboundEventDraft) (bool, ports.InboundEventRecord, error) {
	s.draft = draft
	return s.created, ports.InboundEventRecord{ID: draft.ID}, nil
}
func (s *webhookEventStore) Get(context.Context, string, string) (ports.InboundEventRecord, error) {
	return ports.InboundEventRecord{}, errors.New("not used")
}
func (s *webhookEventStore) List(context.Context, ports.InboundEventFilter) (ports.InboundEventPage, error) {
	return ports.InboundEventPage{}, errors.New("not used")
}
func (s *webhookEventStore) Claim(context.Context, string, ports.InboundEventLease) (ports.InboundEventClaimResult, error) {
	return ports.InboundEventClaimResult{}, errors.New("not used")
}
func (s *webhookEventStore) MarkProcessed(context.Context, string, ports.InboundEventCompletion) (ports.InboundEventRecord, error) {
	return ports.InboundEventRecord{}, errors.New("not used")
}
func (s *webhookEventStore) MarkRetryableFailure(context.Context, string, ports.InboundEventFailure) (ports.InboundEventRecord, error) {
	return ports.InboundEventRecord{}, errors.New("not used")
}
func (s *webhookEventStore) MoveToDeadLetter(context.Context, string, ports.InboundEventFailure) (ports.InboundEventRecord, error) {
	return ports.InboundEventRecord{}, errors.New("not used")
}

type providerInboundStore struct {
	draft  ports.ProviderInboundDraft
	result ports.ProviderInboundResult
	err    error
}

func (s *providerInboundStore) Materialize(_ context.Context, draft ports.ProviderInboundDraft) (ports.ProviderInboundResult, error) {
	s.draft = draft
	return s.result, s.err
}

type deliveryStatusStore struct {
	draft  ports.DeliveryStatusDraft
	result ports.DeliveryStatusResult
	err    error
}

func (s *deliveryStatusStore) Apply(_ context.Context, draft ports.DeliveryStatusDraft) (ports.DeliveryStatusResult, error) {
	s.draft = draft
	return s.result, s.err
}

type autoReplyHandler struct {
	command commands.AutoReplyCommand
	calls   int
	err     error
}

func (s *autoReplyHandler) Handle(_ context.Context, command commands.AutoReplyCommand) (commands.AutoReplyResult, error) {
	s.calls++
	s.command = command
	return commands.AutoReplyResult{Enqueued: true}, s.err
}

func signedSocialCommand(t *testing.T, body []byte) commands.IngestWebhookCommand {
	t.Helper()
	secret := "secret"
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	return commands.IngestWebhookCommand{RouteKey: "socialapi", DeliveryID: "delivery-1", RequestID: "request-1", ProviderHeaders: map[string]string{
		"X-SocialAPI-Signature-V2": "sha256=" + hex.EncodeToString(mac.Sum(nil)),
		"X-SocialAPI-Timestamp":    timestamp,
		"X-SocialAPI-Delivery":     "delivery-1",
	}, RawPayload: body}
}

func resolvedSocialWebhookService(inbound ports.ProviderInboundStore) SocialAPIWebhookService {
	return SocialAPIWebhookService{
		Receiver:    socialapi.NewClient(socialapi.Config{WebhookSecret: "secret"}),
		RawPayloads: &webhookRawPayloadStore{},
		Connections: webhookConnectionRepository{record: ports.ChannelConnectionRecord{ID: "connection-1", BusinessID: "business-1", ProviderReference: "socialapi", ProviderAccountReference: stringPointer("account-1"), ProviderConnectionRef: "connection-ref-1"}},
		Events:      &webhookEventStore{created: true},
		Inbound:     inbound,
	}
}

func TestSocialAPIWebhookServiceRecordsResolvedEventAndRawPayloadHash(t *testing.T) {
	body := []byte(`{"event":"dm.received","data":{"id":"event-1","type":"dm","platform":"instagram","account_id":"account-1","conversation_id":"conversation-1","author":{"id":"customer-1"},"content":{"text":"hello"}}}`)
	raw := &webhookRawPayloadStore{}
	events := &webhookEventStore{created: true}
	inbound := &providerInboundStore{result: ports.ProviderInboundResult{BusinessID: "business-1", CustomerID: "customer-1", ConversationID: "conversation-1", ConversationReferenceID: "reference-1", CommunicationMessageID: "message-1"}}
	service := resolvedSocialWebhookService(inbound)
	service.RawPayloads = raw
	service.Events = events
	result, err := service.Handle(context.Background(), signedSocialCommand(t, body))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !result.Accepted || !result.Resolved || result.Duplicate || result.Ignored {
		t.Fatalf("unexpected result: %#v", result)
	}
	if events.draft.BusinessID == nil || *events.draft.BusinessID != "business-1" || events.draft.ConnectionID == nil || *events.draft.ConnectionID != "connection-1" {
		t.Fatalf("unexpected tenant mapping: %#v", events.draft)
	}
	if events.draft.ProviderEventID != "event-1" || events.draft.DedupeStrategy != "provider_event_id" || !events.draft.SignatureVerified || events.draft.RawPayloadReference != "socialapi/delivery-1" || events.draft.PayloadHash != raw.sha256 || string(raw.payload) != string(body) {
		t.Fatalf("unexpected event draft: %#v raw=%#v", events.draft, raw)
	}
	if inbound.draft.InboundEventID == "" || inbound.draft.BusinessID != "business-1" || inbound.draft.ConnectionID != "connection-1" || inbound.draft.ProviderAccountRef != "account-1" || inbound.draft.ProviderConversationID != "conversation-1" || inbound.draft.ProviderMessageID != "event-1" || inbound.draft.EventType != "interaction_received" {
		t.Fatalf("unexpected provider materialization draft: %#v", inbound.draft)
	}
}

func TestSocialAPIWebhookServiceRunsAutoReplyOnlyForNewInboundMessage(t *testing.T) {
	body := []byte(`{"event":"dm.received","data":{"id":"event-1","type":"dm","platform":"instagram","account_id":"account-1","conversation_id":"conversation-1","author":{"id":"customer-1"},"content":{"text":"hello"}}}`)
	autoReply := &autoReplyHandler{}
	service := resolvedSocialWebhookService(&providerInboundStore{result: ports.ProviderInboundResult{BusinessID: "business-1", ConversationID: "conversation-1", CommunicationMessageID: "message-1"}})
	service.AutoReply = autoReply
	if _, err := service.Handle(context.Background(), signedSocialCommand(t, body)); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if autoReply.calls != 1 || autoReply.command.Meta.Actor.BusinessID != "business-1" || autoReply.command.ConversationID != "conversation-1" || autoReply.command.SourceMessageReference != "event-1" || autoReply.command.Text != "hello" || autoReply.command.ProviderRef != "socialapi" || autoReply.command.Channel != "instagram" {
		t.Fatalf("unexpected auto reply command: %#v calls=%d", autoReply.command, autoReply.calls)
	}
}

func TestSocialAPIWebhookServiceDoesNotRunAutoReplyForDuplicateOrDeliveryStatus(t *testing.T) {
	t.Run("duplicate message", func(t *testing.T) {
		body := []byte(`{"event":"dm.received","data":{"id":"event-1","type":"dm","platform":"instagram","account_id":"account-1","conversation_id":"conversation-1","author":{"id":"customer-1"},"content":{"text":"hello"}}}`)
		autoReply := &autoReplyHandler{}
		service := resolvedSocialWebhookService(&providerInboundStore{result: ports.ProviderInboundResult{BusinessID: "business-1", ConversationID: "conversation-1", Duplicate: true}})
		service.AutoReply = autoReply
		result, err := service.Handle(context.Background(), signedSocialCommand(t, body))
		if err != nil || !result.Duplicate || autoReply.calls != 0 {
			t.Fatalf("unexpected duplicate result=%#v err=%v calls=%d", result, err, autoReply.calls)
		}
	})
	t.Run("delivery status", func(t *testing.T) {
		body := []byte(`{"event":"dm.status.delivered","data":{"id":"status-event-1","type":"dm_status","platform":"whatsapp","account_id":"account-1","conversation_id":"conversation-1","mids":["provider-message-1"],"status":"delivered"}}`)
		statuses := &deliveryStatusStore{result: ports.DeliveryStatusResult{Applied: true, OutboundMessageID: "outbound-1", Status: "delivered"}}
		inbound := &providerInboundStore{}
		autoReply := &autoReplyHandler{}
		service := resolvedSocialWebhookService(inbound)
		service.DeliveryStatuses = statuses
		service.AutoReply = autoReply
		result, err := service.Handle(context.Background(), signedSocialCommand(t, body))
		if err != nil || !result.Accepted || !result.Resolved || statuses.draft.InboundEventID == "" || statuses.draft.ProviderMessageID != "provider-message-1" || inbound.draft.InboundEventID != "" || autoReply.calls != 0 {
			t.Fatalf("unexpected delivery status result=%#v draft=%#v inbound=%#v err=%v calls=%d", result, statuses.draft, inbound.draft, err, autoReply.calls)
		}
	})
}

func TestSocialAPIWebhookServicePersistsValidUnknownConnectionAsUnresolved(t *testing.T) {
	body := []byte(`{"event":"dm.received","data":{"id":"event-2","type":"dm","platform":"facebook","account_id":"unknown-account","conversation_id":"conversation-2","content":{"text":"hello"}}}`)
	events := &webhookEventStore{created: true}
	service := SocialAPIWebhookService{
		Receiver:    socialapi.NewClient(socialapi.Config{WebhookSecret: "secret"}),
		RawPayloads: &webhookRawPayloadStore{},
		Connections: webhookConnectionRepository{err: repositoryKindError("not_found")},
		Events:      events,
	}
	result, err := service.Handle(context.Background(), signedSocialCommand(t, body))
	if err != nil || !result.Accepted || result.Resolved || result.Duplicate || events.draft.BusinessID != nil || events.draft.ConnectionID != nil || events.draft.ProcessingState != "unresolved" {
		t.Fatalf("unexpected unresolved result=%#v event=%#v err=%v", result, events.draft, err)
	}
}

func TestSocialAPIWebhookServiceRejectsInvalidSignature(t *testing.T) {
	service := resolvedSocialWebhookService(&providerInboundStore{})
	command := commands.IngestWebhookCommand{RouteKey: "socialapi", ProviderHeaders: map[string]string{"X-SocialAPI-Signature-V2": "sha256=00", "X-SocialAPI-Timestamp": strconv.FormatInt(time.Now().Unix(), 10)}, RawPayload: []byte(`{"event":"dm.received"}`)}
	_, err := service.Handle(context.Background(), command)
	var appErr *appErrors.Error
	if !errors.As(err, &appErr) || appErr.Code != appErrors.CodeUnauthenticated {
		t.Fatalf("unexpected error: %v", err)
	}
}

var _ ports.ChannelConnectionRepository = webhookConnectionRepository{}
var _ ports.EventStore = (*webhookEventStore)(nil)
var _ ports.ProviderInboundStore = (*providerInboundStore)(nil)
var _ ports.DeliveryStatusStore = (*deliveryStatusStore)(nil)
var _ commands.AutoReplyHandler = (*autoReplyHandler)(nil)

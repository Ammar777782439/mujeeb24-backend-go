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
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/workspaces/chatwoot"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/channel"
	"github.com/google/uuid"
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

type chatwootInboundStore struct {
	draft  ports.ChatwootInboundDraft
	result ports.ChatwootInboundResult
	err    error
}

func (s *chatwootInboundStore) Materialize(_ context.Context, draft ports.ChatwootInboundDraft) (ports.ChatwootInboundResult, error) {
	s.draft = draft
	return s.result, s.err
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

func TestSocialAPIWebhookServiceRecordsResolvedEventAndRawPayloadHash(t *testing.T) {
	body := []byte(`{"event":"dm.received","data":{"id":"event-1","type":"dm","platform":"instagram","account_id":"account-1","conversation_id":"conversation-1","author":{"id":"customer-1"},"content":{"text":"hello"}}}`)
	raw := &webhookRawPayloadStore{}
	events := &webhookEventStore{created: true}
	service := SocialAPIWebhookService{
		Receiver:    socialapi.NewClient(socialapi.Config{WebhookSecret: "secret"}),
		RawPayloads: raw,
		Connections: webhookConnectionRepository{record: ports.ChannelConnectionRecord{ID: "connection-1", BusinessID: "business-1", ProviderReference: "socialapi", ProviderAccountReference: stringPointer("account-1"), ProviderConnectionRef: "connection-ref-1"}},
		Events:      events,
	}
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
	if err != nil || !result.Accepted || result.Resolved || result.Duplicate {
		t.Fatalf("unexpected unresolved result=%#v err=%v", result, err)
	}
	if events.draft.BusinessID != nil || events.draft.ConnectionID != nil || events.draft.ProcessingState != "unresolved" {
		t.Fatalf("unexpected unresolved draft: %#v", events.draft)
	}
}

func TestChatwootWebhookServiceMaterializesVerifiedCallback(t *testing.T) {
	body := []byte(`{"event":"message_created","id":901,"content":"hello","account":{"id":12},"inbox":{"id":34},"conversation":{"id":78},"sender":{"id":56}}`)
	store := &chatwootInboundStore{result: ports.ChatwootInboundResult{CustomerID: "customer-1", ConversationID: "conversation-1", CommunicationMessageID: "message-1"}}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	command := commands.IngestWebhookCommand{RouteKey: "chatwoot-test", ProviderHeaders: map[string]string{"X-Chatwoot-Signature": "sha256=" + hex.EncodeToString(mac.Sum(nil)), "X-Chatwoot-Timestamp": timestamp}, RawPayload: body}
	result, err := (ChatwootWebhookService{Receiver: chatwoot.NewClient(chatwoot.Config{WebhookSecret: "secret"}), Inbound: store}).Handle(context.Background(), command)
	if err != nil || !result.Accepted || result.Ignored || !result.Resolved || result.Duplicate {
		t.Fatalf("unexpected result=%#v err=%v", result, err)
	}
	if store.draft.RouteKey != "chatwoot-test" || store.draft.AccountID != "12" || store.draft.InboxID != "34" || store.draft.ConversationID != "78" || store.draft.ExternalUserID != "56" || store.draft.ProviderMessageID != "901" || store.draft.EventType != "interaction_received" || store.draft.Content != "hello" || store.draft.PayloadHash == "" {
		t.Fatalf("unexpected materialization draft=%#v", store.draft)
	}
}

func TestChatwootWebhookServiceRejectsInvalidSignature(t *testing.T) {
	command := commands.IngestWebhookCommand{RouteKey: "chatwoot", ProviderHeaders: map[string]string{"X-Chatwoot-Signature": "sha256=00", "X-Chatwoot-Timestamp": strconv.FormatInt(time.Now().Unix(), 10)}, RawPayload: []byte(`{"event":"message_created","id":901}`)}
	_, err := (ChatwootWebhookService{Receiver: chatwoot.NewClient(chatwoot.Config{WebhookSecret: "secret"}), Inbound: &chatwootInboundStore{}}).Handle(context.Background(), command)
	var appErr *appErrors.Error
	if !errors.As(err, &appErr) || appErr.Code != appErrors.CodeUnauthenticated {
		t.Fatalf("unexpected error: %v", err)
	}
}

var _ ports.ChannelConnectionRepository = webhookConnectionRepository{}
var _ ports.EventStore = (*webhookEventStore)(nil)
var _ ports.ChatwootInboundStore = (*chatwootInboundStore)(nil)
var _ = channel.ProviderSocialAPI
var _ = uuid.Nil

func signedChatwootCommand(t *testing.T, body []byte) commands.IngestWebhookCommand {
	t.Helper()
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	return commands.IngestWebhookCommand{RouteKey: "chatwoot-test", DeliveryID: "chatwoot-delivery", RequestID: "chatwoot-request", ProviderHeaders: map[string]string{"X-Chatwoot-Signature": "sha256=" + hex.EncodeToString(mac.Sum(nil)), "X-Chatwoot-Timestamp": timestamp}, RawPayload: body}
}

func testChatwootAutoReplyBridge() (ChatwootAutoReplyBridge, *fakeOutboxStore) {
	reference, connection := providerReferenceBindingFixtures()
	reference.ChatwootAccountID = stringPtr("12")
	reference.ChatwootInboxID = stringPtr("34")
	reference.ChatwootConversationID = stringPtr("78")
	connection.ProviderAccountReference = stringPtr("12")
	references := fakeReferenceRepository{record: reference}
	outbox := &fakeOutboxStore{}
	autoReply := NewAutoReplyService(SafeAutoReplyRuntime{}, &fakeDecisionRepository{}, references, &fakeOutboundRepository{}, outbox, fakeTransactionManager{})
	return ChatwootAutoReplyBridge{Resolver: ChatwootProviderReferenceResolver{References: references, Connections: resolverConnectionRepository{record: connection}}, AutoReply: autoReply}, outbox
}

func TestChatwootWebhookServiceOrchestratesIncomingAutoReplyToOutbox(t *testing.T) {
	body := []byte(`{"event":"message_created","id":1001,"content":"hello","message_type":"incoming","account":{"id":12},"inbox":{"id":34},"conversation":{"id":78},"sender":{"id":56,"type":"contact"}}`)
	store := &chatwootInboundStore{result: ports.ChatwootInboundResult{BusinessID: "business-1", ConversationID: "conversation-1", CustomerID: "customer-1", CommunicationMessageID: "message-1"}}
	bridge, outbox := testChatwootAutoReplyBridge()
	service := ChatwootWebhookService{Receiver: chatwoot.NewClient(chatwoot.Config{WebhookSecret: "secret"}), Inbound: store, AutoReply: &bridge}
	result, err := service.Handle(context.Background(), signedChatwootCommand(t, body))
	if err != nil || !result.Accepted || !result.Resolved || result.Duplicate || outbox.calls != 1 {
		t.Fatalf("unexpected incoming orchestration result=%#v err=%v outbox_calls=%d", result, err, outbox.calls)
	}
	if store.draft.Direction != "inbound" || store.draft.Origin != "customer" || store.draft.MessageType != "incoming" {
		t.Fatalf("unexpected inbound draft=%#v", store.draft)
	}
}

func TestChatwootWebhookServiceDoesNotAutoReplyToOutgoingOrPrivateMessages(t *testing.T) {
	cases := []struct {
		name string
		body []byte
	}{
		{name: "outgoing", body: []byte(`{"event":"message_created","id":1002,"content":"agent reply","message_type":"outgoing","account":{"id":12},"inbox":{"id":34},"conversation":{"id":78},"sender":{"id":56,"type":"user"}}`)},
		{name: "private", body: []byte(`{"event":"message_created","id":1003,"content":"private note","message_type":"incoming","private":true,"account":{"id":12},"inbox":{"id":34},"conversation":{"id":78},"sender":{"id":56,"type":"user"}}`)},
		{name: "activity", body: []byte(`{"event":"message_created","id":1005,"content":"activity","message_type":"activity","account":{"id":12},"inbox":{"id":34},"conversation":{"id":78},"sender":{"id":56,"type":"user"}}`)},
		{name: "template", body: []byte(`{"event":"message_created","id":1006,"content":"template","message_type":"template","account":{"id":12},"inbox":{"id":34},"conversation":{"id":78},"sender":{"id":56,"type":"user"}}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &chatwootInboundStore{result: ports.ChatwootInboundResult{BusinessID: "business-1", ConversationID: "conversation-1"}}
			bridge, outbox := testChatwootAutoReplyBridge()
			service := ChatwootWebhookService{Receiver: chatwoot.NewClient(chatwoot.Config{WebhookSecret: "secret"}), Inbound: store, AutoReply: &bridge}
			result, err := service.Handle(context.Background(), signedChatwootCommand(t, tc.body))
			if err != nil || !result.Accepted || outbox.calls != 0 {
				t.Fatalf("unexpected echo suppression result=%#v err=%v outbox_calls=%d", result, err, outbox.calls)
			}
			if store.draft.Direction != "outbound" {
				t.Fatalf("expected outbound direction, draft=%#v", store.draft)
			}
		})
	}
}

func TestChatwootWebhookServiceDoesNotAutoReplyToDuplicateOrUnresolvedReference(t *testing.T) {
	body := []byte(`{"event":"message_created","id":1004,"content":"hello again","message_type":"incoming","account":{"id":12},"inbox":{"id":34},"conversation":{"id":78},"sender":{"id":56,"type":"contact"}}`)
	duplicateStore := &chatwootInboundStore{result: ports.ChatwootInboundResult{BusinessID: "business-1", ConversationID: "conversation-1", Duplicate: true}}
	bridge, outbox := testChatwootAutoReplyBridge()
	service := ChatwootWebhookService{Receiver: chatwoot.NewClient(chatwoot.Config{WebhookSecret: "secret"}), Inbound: duplicateStore, AutoReply: &bridge}
	result, err := service.Handle(context.Background(), signedChatwootCommand(t, body))
	if err != nil || !result.Accepted || !result.Duplicate || outbox.calls != 0 {
		t.Fatalf("unexpected duplicate result=%#v err=%v outbox_calls=%d", result, err, outbox.calls)
	}

	unresolvedStore := &chatwootInboundStore{result: ports.ChatwootInboundResult{BusinessID: "business-1", ConversationID: "conversation-1"}}
	unresolvedBridge := ChatwootAutoReplyBridge{Resolver: ChatwootProviderReferenceResolver{References: resolverErrorReferenceRepository{err: repositoryKindError("not_found")}, Connections: resolverConnectionRepository{}}, AutoReply: bridge.AutoReply}
	service = ChatwootWebhookService{Receiver: chatwoot.NewClient(chatwoot.Config{WebhookSecret: "secret"}), Inbound: unresolvedStore, AutoReply: &unresolvedBridge}
	result, err = service.Handle(context.Background(), signedChatwootCommand(t, body))
	if err != nil || !result.Accepted || result.Resolved || outbox.calls != 0 {
		t.Fatalf("unexpected unresolved result=%#v err=%v outbox_calls=%d", result, err, outbox.calls)
	}
}

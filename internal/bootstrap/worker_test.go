package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/adapters/secondary/providers/socialapi"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/services"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/channel"
)

type workerOutboxFake struct {
	entries   []ports.OutboxEntryRecord
	claimed   []string
	completed int
}

func (f *workerOutboxFake) ListClaimable(context.Context, int) ([]ports.OutboxEntryRecord, error) {
	return f.entries, nil
}
func (f *workerOutboxFake) Enqueue(context.Context, ports.OutboxEntryDraft) (ports.OutboxEntryRecord, error) {
	return ports.OutboxEntryRecord{}, errors.New("not used")
}
func (f *workerOutboxFake) Get(context.Context, string, string) (ports.OutboxEntryRecord, error) {
	return ports.OutboxEntryRecord{}, errors.New("not used")
}
func (f *workerOutboxFake) List(context.Context, ports.OutboxFilter) (ports.OutboxPage, error) {
	return ports.OutboxPage{}, errors.New("not used")
}
func (f *workerOutboxFake) Claim(_ context.Context, entryID string, lease ports.OutboxLease) (ports.OutboxClaimResult, error) {
	f.claimed = append(f.claimed, entryID)
	record := f.entries[0]
	record.LeaseOwner = &lease.Owner
	record.LeaseToken = &lease.Token
	return ports.OutboxClaimResult{Claimed: true, Record: record}, nil
}
func (f *workerOutboxFake) MarkCompleted(context.Context, string, ports.OutboxCompletion) (ports.OutboxEntryRecord, error) {
	f.completed++
	return ports.OutboxEntryRecord{}, nil
}
func (f *workerOutboxFake) MarkRetryableFailure(context.Context, string, ports.OutboxFailure) (ports.OutboxEntryRecord, error) {
	return ports.OutboxEntryRecord{}, nil
}
func (f *workerOutboxFake) MoveToDeadLetter(context.Context, string, ports.OutboxFailure) (ports.OutboxEntryRecord, error) {
	return ports.OutboxEntryRecord{}, nil
}
func (f *workerOutboxFake) Requeue(context.Context, string, time.Time, time.Time) (ports.OutboxEntryRecord, error) {
	return ports.OutboxEntryRecord{}, nil
}

type workerResolverFake struct{}

func (workerResolverFake) Resolve(context.Context, string, string) (ports.OutboundDelivery, error) {
	return ports.OutboundDelivery{ConnectionID: "connection-1", ProviderAccountID: "account-1", ProviderConversationID: "provider-conversation-1", Text: "reply", IdempotencyKey: "auto-reply:inbound-1"}, nil
}

type workerProviderFake struct{ sends int }

func (p *workerProviderFake) VerifyWebhook(context.Context, map[string]string, []byte) error {
	return errors.New("not used")
}
func (p *workerProviderFake) NormalizeWebhook(context.Context, map[string]string, []byte) ([]channel.InboundEvent, error) {
	return nil, errors.New("not used")
}
func (p *workerProviderFake) SendMessage(context.Context, ports.SendMessageCommand) (ports.ProviderSendResult, error) {
	p.sends++
	return ports.ProviderSendResult{ProviderMessageID: "provider-message-1", Status: channel.DeliveryAccepted}, nil
}
func (p *workerProviderFake) GetDeliveryStatus(context.Context, ports.DeliveryReference) (channel.DeliveryStatus, error) {
	return channel.DeliveryUnknown, errors.New("not used")
}

func TestWorkerRunOnceProcessesClaimableOutboxEntries(t *testing.T) {
	outbox := &workerOutboxFake{entries: []ports.OutboxEntryRecord{{ID: "entry-1", BusinessID: "business-1", OutboundMessageID: "message-1", CommandType: services.OutboundSendCommandType}}}
	provider := &workerProviderFake{}
	runtime := &WorkerRuntime{Outbox: outbox, BatchSize: 10, Processor: &services.OutboxProcessor{Outbox: outbox, Resolver: workerResolverFake{}, Provider: provider, Owner: "worker-test", Now: func() time.Time { return time.Unix(100, 0) }}}
	processed, err := runtime.RunOnce(context.Background())
	if err != nil || processed != 1 || len(outbox.claimed) != 1 || outbox.claimed[0] != "entry-1" || outbox.completed != 1 || provider.sends != 1 {
		t.Fatalf("processed=%d err=%v claimed=%v completed=%d sends=%d", processed, err, outbox.claimed, outbox.completed, provider.sends)
	}
}

func TestWorkerRunOnceDoesNotSendWhenProviderIsNotConfigured(t *testing.T) {
	runtime := &WorkerRuntime{Outbox: &workerOutboxFake{entries: []ports.OutboxEntryRecord{{ID: "entry-1"}}}, BatchSize: 10}
	processed, err := runtime.RunOnce(context.Background())
	if err != nil || processed != 0 {
		t.Fatalf("processed=%d err=%v", processed, err)
	}
}

func TestWorkerRuntimeDefaultsCycleTimeoutToBoundGracefulWork(t *testing.T) {
	runtime := &WorkerRuntime{}
	if runtime.cycleTimeout() != 10*time.Second {
		t.Fatalf("unexpected default cycle timeout: %s", runtime.cycleTimeout())
	}
	runtime.CycleTimeout = 3 * time.Second
	if runtime.cycleTimeout() != 3*time.Second {
		t.Fatalf("unexpected configured cycle timeout: %s", runtime.cycleTimeout())
	}
}

func TestWorkerRunOnceExecutesThroughSocialAPIAdapterContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/inbox/conversations/provider-conversation-1/messages" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Fatalf("unexpected authorization header: %q", request.Header.Get("Authorization"))
		}
		var body struct {
			AccountID string `json:"account_id"`
			Text      string `json:"text"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.AccountID != "account-1" || body.Text != "reply" {
			t.Fatalf("unexpected send body: %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"success":true,"message_id":"sapi-message-1"}`))
	}))
	defer server.Close()

	outbox := &workerOutboxFake{entries: []ports.OutboxEntryRecord{{ID: "entry-1", BusinessID: "business-1", OutboundMessageID: "message-1", CommandType: services.OutboundSendCommandType}}}
	provider := socialapi.NewClient(socialapi.Config{BaseURL: server.URL, APIKey: "test-api-key", HTTPClient: server.Client()})
	runtime := &WorkerRuntime{Outbox: outbox, BatchSize: 10, Processor: &services.OutboxProcessor{Outbox: outbox, Resolver: workerResolverFake{}, Provider: provider, Owner: "worker-test", Now: func() time.Time { return time.Unix(100, 0) }}}
	processed, err := runtime.RunOnce(context.Background())
	if err != nil || processed != 1 || len(outbox.claimed) != 1 || outbox.completed != 1 {
		t.Fatalf("processed=%d err=%v claimed=%v completed=%d", processed, err, outbox.claimed, outbox.completed)
	}
}

var _ ports.OutboxStore = (*workerOutboxFake)(nil)
var _ ports.OutboundDeliveryResolver = workerResolverFake{}
var _ ports.ChannelProvider = (*workerProviderFake)(nil)

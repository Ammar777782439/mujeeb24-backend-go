package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/channel"
)

type processorOutbox struct {
	claimRecord ports.OutboxEntryRecord
	claimed     bool
	completed   *ports.OutboxCompletion
	failure     *ports.OutboxFailure
	deadLetter  *ports.OutboxFailure
}

func (o *processorOutbox) Enqueue(context.Context, ports.OutboxEntryDraft) (ports.OutboxEntryRecord, error) {
	return ports.OutboxEntryRecord{}, errors.New("not used")
}
func (o *processorOutbox) Get(context.Context, string, string) (ports.OutboxEntryRecord, error) {
	return ports.OutboxEntryRecord{}, errors.New("not used")
}
func (o *processorOutbox) List(context.Context, ports.OutboxFilter) (ports.OutboxPage, error) {
	return ports.OutboxPage{}, errors.New("not used")
}
func (o *processorOutbox) Claim(_ context.Context, _ string, lease ports.OutboxLease) (ports.OutboxClaimResult, error) {
	o.claimRecord.LeaseOwner = &lease.Owner
	o.claimRecord.LeaseToken = &lease.Token
	return ports.OutboxClaimResult{Claimed: o.claimed, Record: o.claimRecord}, nil
}
func (o *processorOutbox) MarkCompleted(_ context.Context, _ string, completion ports.OutboxCompletion) (ports.OutboxEntryRecord, error) {
	o.completed = &completion
	return o.claimRecord, nil
}
func (o *processorOutbox) MarkRetryableFailure(_ context.Context, _ string, failure ports.OutboxFailure) (ports.OutboxEntryRecord, error) {
	o.failure = &failure
	return o.claimRecord, nil
}
func (o *processorOutbox) MoveToDeadLetter(_ context.Context, _ string, failure ports.OutboxFailure) (ports.OutboxEntryRecord, error) {
	o.deadLetter = &failure
	return o.claimRecord, nil
}
func (o *processorOutbox) Requeue(context.Context, string, time.Time, time.Time) (ports.OutboxEntryRecord, error) {
	return ports.OutboxEntryRecord{}, errors.New("not used")
}

type processorResolver struct {
	delivery ports.OutboundDelivery
}

func (r processorResolver) Resolve(context.Context, string, string) (ports.OutboundDelivery, error) {
	return r.delivery, nil
}

type processorProvider struct {
	command *ports.SendMessageCommand
	err     error
}

func (p *processorProvider) VerifyWebhook(context.Context, map[string]string, []byte) error {
	return errors.New("not used")
}
func (p *processorProvider) NormalizeWebhook(context.Context, map[string]string, []byte) ([]channel.InboundEvent, error) {
	return nil, errors.New("not used")
}
func (p *processorProvider) SendMessage(_ context.Context, command ports.SendMessageCommand) (ports.ProviderSendResult, error) {
	p.command = &command
	if p.err != nil {
		return ports.ProviderSendResult{}, p.err
	}
	return ports.ProviderSendResult{ProviderRequestID: "request-1", ProviderMessageID: "message-1", Status: channel.DeliveryAccepted}, nil
}
func (p *processorProvider) GetDeliveryStatus(context.Context, ports.DeliveryReference) (channel.DeliveryStatus, error) {
	return channel.DeliveryUnknown, errors.New("not used")
}

func TestOutboxProcessorClaimsThenSendsAndCompletes(t *testing.T) {
	outbox := &processorOutbox{claimed: true, claimRecord: ports.OutboxEntryRecord{ID: "entry-1", BusinessID: "business-1", OutboundMessageID: "message-1", CommandType: OutboundSendCommandType}}
	provider := &processorProvider{}
	processor := OutboxProcessor{Outbox: outbox, Resolver: processorResolver{delivery: ports.OutboundDelivery{ConnectionID: "connection-1", ProviderAccountID: "account-1", ProviderConversationID: "conversation-1", Text: "hello", IdempotencyKey: "idem-1"}}, Provider: provider, Owner: "worker-1", Now: func() time.Time { return time.Unix(100, 0) }}
	if err := processor.Process(context.Background(), "entry-1"); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if provider.command == nil || provider.command.ProviderAccountID != "account-1" || provider.command.ProviderConversationID != "conversation-1" || provider.command.Text != "hello" || provider.command.IdempotencyKey != "idem-1" {
		t.Fatalf("unexpected provider command: %#v", provider.command)
	}
	if outbox.completed == nil || outbox.completed.Owner != "worker-1" || outbox.completed.Token == "" || outbox.completed.ResultCode != "provider_accepted:message-1" || outbox.failure != nil {
		t.Fatalf("unexpected completion: %#v failure=%#v", outbox.completed, outbox.failure)
	}
}

func TestOutboxProcessorPreservesUnknownNetworkOutcome(t *testing.T) {
	outbox := &processorOutbox{claimed: true, claimRecord: ports.OutboxEntryRecord{ID: "entry-1", BusinessID: "business-1", OutboundMessageID: "message-1", CommandType: OutboundSendCommandType}}
	provider := &processorProvider{err: errors.New("connection reset")}
	processor := OutboxProcessor{Outbox: outbox, Resolver: processorResolver{delivery: ports.OutboundDelivery{ConnectionID: "connection-1", ProviderAccountID: "account-1", ProviderConversationID: "conversation-1", Text: "hello", IdempotencyKey: "idem-1"}}, Provider: provider, Owner: "worker-1", Now: func() time.Time { return time.Unix(100, 0) }}
	if err := processor.Process(context.Background(), "entry-1"); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if outbox.failure == nil || outbox.failure.ErrorCode != "provider_send_outcome_unknown" || outbox.failure.NextAttempt != nil || outbox.failure.Owner != "worker-1" || outbox.failure.Token == "" || outbox.completed != nil {
		t.Fatalf("unexpected unknown-outcome handling: failure=%#v completed=%#v", outbox.failure, outbox.completed)
	}
}

var _ ports.OutboxStore = (*processorOutbox)(nil)
var _ ports.OutboundDeliveryResolver = processorResolver{}
var _ ports.ChannelProvider = (*processorProvider)(nil)

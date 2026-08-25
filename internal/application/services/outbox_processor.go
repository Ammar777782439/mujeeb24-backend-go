package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

const OutboundSendCommandType = "channel.send_message"

type OutboxProcessor struct {
	Outbox   ports.OutboxStore
	Resolver ports.OutboundDeliveryResolver
	Provider ports.ChannelProvider
	Messages ports.ProviderAcceptanceRecorder
	Owner    string
	LeaseFor time.Duration
	Now      func() time.Time
}

func (p OutboxProcessor) Process(ctx context.Context, entryID string) error {
	if p.Outbox == nil || p.Resolver == nil || p.Provider == nil {
		return errors.New("outbox processor dependencies are not configured")
	}
	if strings.TrimSpace(entryID) == "" {
		return errors.New("outbox entry id is required")
	}
	if p.Owner == "" {
		p.Owner = "worker"
	}
	if p.LeaseFor <= 0 {
		p.LeaseFor = 2 * time.Minute
	}
	if p.Now == nil {
		p.Now = time.Now
	}
	now := p.Now().UTC()
	claim, err := p.Outbox.Claim(ctx, entryID, ports.OutboxLease{Owner: p.Owner, Token: uuid.NewString(), ExpiresAt: now.Add(p.LeaseFor)})
	if err != nil || !claim.Claimed {
		return err
	}
	record := claim.Record
	if record.CommandType != OutboundSendCommandType {
		return p.deadLetter(ctx, record, "unsupported_outbox_command")
	}
	delivery, err := p.Resolver.Resolve(ctx, record.BusinessID, record.OutboundMessageID)
	if err != nil {
		return p.deadLetter(ctx, record, "outbound_mapping_missing")
	}
	result, err := p.Provider.SendMessage(ctx, ports.SendMessageCommand{ConnectionID: delivery.ConnectionID, ProviderAccountID: delivery.ProviderAccountID, ProviderConversationID: delivery.ProviderConversationID, Text: delivery.Text, IdempotencyKey: delivery.IdempotencyKey})
	if err != nil {
		// A transport failure does not prove that the provider did not accept
		// the request. Quarantine it in dead-letter until reconciliation decides
		// whether a retry is safe; never blindly duplicate a customer message.
		return p.deadLetter(ctx, record, "provider_send_outcome_unknown")
	}
	resultCode := "provider_accepted"
	if p.Messages != nil {
		if result.ProviderMessageID == "" {
			return p.deadLetter(ctx, record, "provider_acceptance_missing_message_id")
		}
		if _, err := p.Messages.MarkProviderAccepted(ctx, record.BusinessID, record.OutboundMessageID, result.ProviderMessageID); err != nil {
			return p.deadLetter(ctx, record, "provider_acceptance_persistence_failed")
		}
	}
	if result.ProviderMessageID != "" {
		resultCode += ":" + result.ProviderMessageID
	}
	_, err = p.Outbox.MarkCompleted(ctx, record.ID, ports.OutboxCompletion{Owner: owner(record, p.Owner), Token: token(record), ResultCode: resultCode, CompletedAt: p.Now().UTC(), UpdatedAt: p.Now().UTC()})
	return err
}

func (p OutboxProcessor) deadLetter(ctx context.Context, record ports.OutboxEntryRecord, code string) error {
	_, err := p.Outbox.MoveToDeadLetter(ctx, record.ID, ports.OutboxFailure{Owner: owner(record, p.Owner), Token: token(record), ErrorCode: code, UpdatedAt: p.Now().UTC()})
	return err
}

func owner(record ports.OutboxEntryRecord, fallback string) string {
	if record.LeaseOwner != nil && *record.LeaseOwner != "" {
		return *record.LeaseOwner
	}
	return fallback
}

func token(record ports.OutboxEntryRecord) string {
	if record.LeaseToken != nil {
		return *record.LeaseToken
	}
	return ""
}

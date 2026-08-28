package services

import (
	"context"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

type ManualOutboundMessageService struct {
	References   ports.ConversationReferenceRepository
	Connections  ports.ChannelConnectionRepository
	Outbound     ports.OutboundMessageRepository
	Outbox       ports.OutboxStore
	Transactions ports.TransactionManager
	Now          func() time.Time
	NewID        func() string
}

func (s ManualOutboundMessageService) Handle(ctx context.Context, command commands.CreateOutboundMessageCommand) (commands.MessageResult, error) {
	if s.References == nil || s.Connections == nil || s.Outbound == nil || s.Outbox == nil || s.Transactions == nil {
		return commands.MessageResult{}, appErrors.NotImplemented()
	}
	text := strings.TrimSpace(command.Text)
	idempotencyKey := strings.TrimSpace(command.Meta.IdempotencyKey)
	if text == "" || idempotencyKey == "" {
		return commands.MessageResult{}, appErrors.New(appErrors.CodeValidation, "message text and Idempotency-Key are required")
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	newID := uuid.NewString
	if s.NewID != nil {
		newID = s.NewID
	}
	businessID := string(command.Meta.Actor.BusinessID)
	conversationID := string(command.ConversationID)
	dedupeKey := "manual-outbound:" + idempotencyKey
	var result commands.MessageResult
	err := s.Transactions.Within(ctx, func(txCtx context.Context) error {
		reference, referenceErr := s.References.GetCurrentByConversation(txCtx, businessID, conversationID, "provider")
		if referenceErr != nil {
			return mapAIRepositoryError(referenceErr)
		}
		if reference.MappingStatus != "active" || reference.ConnectionID == nil || strings.TrimSpace(*reference.ConnectionID) == "" || strings.TrimSpace(reference.ResourceID) == "" {
			return appErrors.New(appErrors.CodeConflict, "current provider conversation reference is not sendable")
		}
		connection, connectionErr := s.Connections.GetByID(txCtx, businessID, *reference.ConnectionID)
		if connectionErr != nil {
			return mapAIRepositoryError(connectionErr)
		}
		if connection.Status != "active" || connection.ProviderReference != reference.ProviderRef || strings.TrimSpace(connection.Channel) == "" {
			return appErrors.New(appErrors.CodeConflict, "channel connection is not active for the conversation reference")
		}
		outboundID := newID()
		outbound, outboundErr := s.Outbound.CreatePending(txCtx, ports.OutboundMessageDraft{ID: outboundID, BusinessID: businessID, ConversationID: conversationID, ConversationReferenceID: reference.ID, ConnectionID: *reference.ConnectionID, ProviderRef: reference.ProviderRef, Channel: connection.Channel, Origin: "human", Transport: "provider", ContentReference: EncodeInlineTextContentReference(text), ProviderIdempotencyKey: dedupeKey, CorrelationID: uuidStringPointer(command.Meta.CorrelationID)})
		if outboundErr != nil {
			return mapAIRepositoryError(outboundErr)
		}
		if _, outboxErr := s.Outbox.Enqueue(txCtx, ports.OutboxEntryDraft{ID: newID(), BusinessID: businessID, OutboundMessageID: outbound.ID, CommandType: OutboundSendCommandType, DedupeKey: dedupeKey, AvailableAt: now, CreatedAt: now, UpdatedAt: now}); outboxErr != nil {
			return mapAIRepositoryError(outboxErr)
		}
		result.Message = commands.MessageView{ID: commands.MessageID(outbound.ID), ConversationID: commands.ConversationID(outbound.ConversationID), Direction: outbound.Direction, Origin: outbound.Origin, Status: outbound.Status, Text: text, ProviderMessageReference: outbound.ProviderMessageID, OccurredAt: now, CreatedAt: now}
		return nil
	})
	return result, err
}

var _ commands.CreateOutboundMessageHandler = ManualOutboundMessageService{}

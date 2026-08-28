package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/channel"
)

type SocialAPIWebhookService struct {
	Receiver         ports.WebhookReceiver
	RawPayloads      ports.RawPayloadStore
	Connections      ports.ChannelConnectionRepository
	Events           ports.EventStore
	Inbound          ports.ProviderInboundStore
	DeliveryStatuses ports.DeliveryStatusStore
	AutoReply        commands.AutoReplyHandler
	Now              func() time.Time
}

func (s SocialAPIWebhookService) Handle(ctx context.Context, command commands.IngestWebhookCommand) (commands.WebhookAcceptedResult, error) {
	if s.Receiver == nil || s.RawPayloads == nil || s.Connections == nil || s.Events == nil {
		return commands.WebhookAcceptedResult{}, appErrors.NotImplemented()
	}
	if err := validateWebhookCommand(command); err != nil {
		return commands.WebhookAcceptedResult{}, err
	}
	if err := s.Receiver.VerifyWebhook(ctx, command.ProviderHeaders, command.RawPayload); err != nil {
		return commands.WebhookAcceptedResult{}, webhookAuthenticationError(err)
	}
	events, err := s.Receiver.NormalizeWebhook(ctx, command.ProviderHeaders, command.RawPayload)
	if err != nil {
		return commands.WebhookAcceptedResult{}, webhookAuthenticationError(err)
	}
	if len(events) == 0 {
		return commands.WebhookAcceptedResult{Accepted: true, Ignored: true, RequestID: command.RequestID}, nil
	}
	if s.Now == nil {
		s.Now = time.Now
	}
	stored, err := s.RawPayloads.Put(ctx, "socialapi", command.DeliveryID, command.RawPayload)
	if err != nil {
		return commands.WebhookAcceptedResult{}, externalDependencyError("raw webhook payload could not be stored", err)
	}
	result := commands.WebhookAcceptedResult{Accepted: true, RequestID: command.RequestID}
	for _, event := range events {
		if event.Provider != channel.ProviderSocialAPI || strings.TrimSpace(event.ProviderConnectionID) == "" {
			return commands.WebhookAcceptedResult{}, appErrors.New(appErrors.CodeValidation, "verified SocialAPI webhook has no provider account reference")
		}
		connection, resolveErr := s.Connections.GetByProviderReferences(ctx, string(channel.ProviderSocialAPI), event.ProviderConnectionID, "")
		var businessID, connectionID *string
		if resolveErr == nil {
			businessIDValue, connectionIDValue := connection.BusinessID, connection.ID
			businessID, connectionID = &businessIDValue, &connectionIDValue
			result.Resolved = true
		} else if repositoryErrorKind(resolveErr) == "not_found" {
			result.Resolved = false
		} else if repositoryErrorKind(resolveErr) == "conflict" {
			return commands.WebhookAcceptedResult{}, appErrors.New(appErrors.CodeConflict, "provider account maps to multiple channel connections")
		} else {
			return commands.WebhookAcceptedResult{}, externalDependencyError("channel connection lookup failed", resolveErr)
		}
		created, record, recordErr := s.Events.RecordIfAbsent(ctx, ports.InboundEventDraft{
			ID:                     event.ID,
			ProviderRef:            string(event.Provider),
			ProviderConnectionRef:  event.ProviderConnectionID,
			ProviderEventID:        event.ProviderEventID,
			DedupeStrategy:         nonEmptyOr(event.DedupeStrategy, "provider_event_id"),
			BusinessID:             businessID,
			ConnectionID:           connectionID,
			EventType:              event.EventType,
			InteractionKind:        stringPointer(string(event.InteractionKind)),
			ProviderMessageID:      stringPointer(event.ProviderMessageID),
			ProviderConversationID: stringPointer(event.ProviderConversationID),
			ExternalUserID:         stringPointer(event.ExternalUserID),
			ExternalCreatedAt:      event.ExternalCreatedAt,
			ReceivedAt:             event.ReceivedAt,
			RawPayloadReference:    stored.Reference,
			PayloadHash:            stored.SHA256,
			SignatureVerified:      true,
			ProcessingState:        nonEmptyOr(dedupeState(businessID), "unresolved"),
			CreatedAt:              s.Now().UTC(),
			UpdatedAt:              s.Now().UTC(),
		})
		if recordErr != nil {
			return commands.WebhookAcceptedResult{}, externalDependencyError("inbound event could not be recorded", recordErr)
		}
		if !created {
			result.Duplicate = true
		}
		if resolveErr == nil && event.EventType == "delivery_status_changed" {
			if s.DeliveryStatuses == nil {
				return commands.WebhookAcceptedResult{}, appErrors.NotImplemented()
			}
			statusResult, statusErr := s.DeliveryStatuses.Apply(ctx, ports.DeliveryStatusDraft{InboundEventID: record.ID, BusinessID: connection.BusinessID, ConnectionID: connection.ID, ProviderRef: string(event.Provider), ProviderAccountRef: event.ProviderConnectionID, ProviderMessageID: event.ProviderMessageID, Status: event.DeliveryStatus, OccurredAt: event.ReceivedAt})
			if statusErr != nil {
				return commands.WebhookAcceptedResult{}, externalDependencyError("SocialAPI delivery status could not be applied", statusErr)
			}
			if statusResult.Duplicate {
				result.Duplicate = true
			}
			continue
		}
		if resolveErr == nil {
			if s.Inbound == nil {
				return commands.WebhookAcceptedResult{}, appErrors.NotImplemented()
			}
			materialized, materializeErr := s.Inbound.Materialize(ctx, ports.ProviderInboundDraft{
				InboundEventID:         record.ID,
				BusinessID:             connection.BusinessID,
				ConnectionID:           connection.ID,
				ProviderRef:            string(event.Provider),
				ProviderEventID:        event.ProviderEventID,
				Channel:                string(event.Channel),
				ProviderAccountRef:     event.ProviderConnectionID,
				ProviderConversationID: event.ProviderConversationID,
				ExternalUserID:         event.ExternalUserID,
				ProviderMessageID:      event.ProviderMessageID,
				EventType:              event.EventType,
				InteractionKind:        string(event.InteractionKind),
				Text:                   event.Text,
				ExternalCreatedAt:      event.ExternalCreatedAt,
				ReceivedAt:             event.ReceivedAt,
				RawPayloadReference:    stored.Reference,
				PayloadHash:            stored.SHA256,
			})
			if materializeErr != nil {
				return commands.WebhookAcceptedResult{}, externalDependencyError("SocialAPI inbound event could not be materialized", materializeErr)
			}
			if materialized.Duplicate {
				result.Duplicate = true
				continue
			}
			if s.AutoReply != nil && event.EventType == "interaction_received" && event.Direction == channel.DirectionInbound && event.Origin == channel.OriginCustomer && strings.TrimSpace(event.ProviderMessageID) != "" && strings.TrimSpace(event.Text) != "" {
				if _, autoReplyErr := s.AutoReply.Handle(ctx, commands.AutoReplyCommand{
					Meta:                   commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(connection.BusinessID)}},
					ConversationID:         commands.ConversationID(materialized.ConversationID),
					SourceMessageReference: event.ProviderMessageID,
					Text:                   event.Text,
					Channel:                string(event.Channel),
					ProviderRef:            string(event.Provider),
				}); autoReplyErr != nil {
					return commands.WebhookAcceptedResult{}, externalDependencyError("SocialAPI AutoReply could not be executed", autoReplyErr)
				}
			}
		}

	}
	return result, nil
}

func validateWebhookCommand(command commands.IngestWebhookCommand) error {
	if strings.TrimSpace(command.RouteKey) == "" {
		return appErrors.New(appErrors.CodeValidation, "webhook route key is required")
	}
	if len(command.RawPayload) == 0 {
		return appErrors.New(appErrors.CodeValidation, "webhook raw payload is required")
	}
	return nil
}

func webhookAuthenticationError(err error) error {
	return &appErrors.Error{Code: appErrors.CodeUnauthenticated, Message: "webhook signature verification failed", Cause: err}
}

func externalDependencyError(message string, err error) error {
	return &appErrors.Error{Code: appErrors.CodeExternalDependency, Message: message, Retryable: true, Cause: err}
}

func repositoryErrorKind(err error) string {
	var classified interface{ ErrorKind() string }
	if errors.As(err, &classified) {
		return classified.ErrorKind()
	}
	return ""
}

func dedupeState(businessID *string) string {
	if businessID != nil {
		return "received"
	}
	return "unresolved"
}

func stringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func nonEmptyOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

var _ commands.IngestSocialAPIWebhookHandler = SocialAPIWebhookService{}

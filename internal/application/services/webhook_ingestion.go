package services

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/channel"
	"github.com/google/uuid"
)

type SocialAPIWebhookService struct {
	Receiver         ports.WebhookReceiver
	RawPayloads      ports.RawPayloadStore
	Connections      ports.ChannelConnectionRepository
	Events           ports.EventStore
	Inbound          ports.ProviderInboundStore
	DeliveryStatuses ports.DeliveryStatusStore
	Automation       commands.ApplyInboundAutomationHandler
	AutoReply        commands.AutoReplyHandler
	Realtime         ports.RealtimePublisher
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
			} else if s.Realtime != nil {
				statusData, _ := json.Marshal(map[string]any{
					"provider_message_id": event.ProviderMessageID,
					"status":              event.DeliveryStatus,
					"occurred_at":         event.ReceivedAt,
				})
				_ = s.Realtime.Publish(ctx, ports.RealtimeEvent{
					EventID:      uuid.NewString(),
					EventType:    "conversation.message_status_changed",
					BusinessID:   connection.BusinessID,
					ResourceType: "message",
					ResourceID:   event.ProviderMessageID,
					OccurredAt:   event.ReceivedAt,
					Data:         statusData,
				})
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
			if s.Realtime != nil {
				msgData, _ := json.Marshal(map[string]any{
					"message_id":          materialized.CommunicationMessageID,
					"conversation_id":     materialized.ConversationID,
					"direction":           "inbound",
					"origin":              "customer",
					"text":                event.Text,
					"provider_message_id": event.ProviderMessageID,
					"channel":             string(event.Channel),
					"received_at":         event.ReceivedAt,
				})
				_ = s.Realtime.Publish(ctx, ports.RealtimeEvent{
					EventID:      uuid.NewString(),
					EventType:    "conversation.message_received",
					BusinessID:   connection.BusinessID,
					ResourceType: "conversation",
					ResourceID:   materialized.ConversationID,
					OccurredAt:   event.ReceivedAt,
					Data:         msgData,
				})
			}
			if s.Automation != nil && event.EventType == "interaction_received" && event.Direction == channel.DirectionInbound && event.Origin == channel.OriginCustomer && strings.TrimSpace(event.ProviderMessageID) != "" {
				if _, automationErr := s.Automation.Handle(ctx, commands.ApplyInboundAutomationCommand{BusinessID: commands.BusinessID(connection.BusinessID), ConversationID: commands.ConversationID(materialized.ConversationID), InboundEventID: commands.ID(record.ID), Channel: string(event.Channel), Text: event.Text}); automationErr != nil {
					return commands.WebhookAcceptedResult{}, externalDependencyError("SocialAPI inbound automation could not be executed", automationErr)
				}
			}
			if s.AutoReply != nil && event.EventType == "interaction_received" && event.Direction == channel.DirectionInbound && event.Origin == channel.OriginCustomer && strings.TrimSpace(event.ProviderMessageID) != "" && strings.TrimSpace(event.Text) != "" {
				log.Printf("[Webhook] AUTO_REPLY_TRIGGER business=%s conversation=%s text=%q", connection.BusinessID, materialized.ConversationID, truncate(event.Text, 60))
				// Run AutoReply in a detached context with a generous timeout.
				// The HTTP request context (ctx) gets cancelled when SocialAPI
				// disconnects or when the HTTP server's WriteTimeout fires.
				// If we pass ctx to AutoReply.Handle, the Gemini API call
				// fails with "context canceled" mid-flight.
				//
				// Per contract ⑨ §11: every external operation (Gemini call)
				// must have a Timeout — but that timeout should be generous
				// enough for the full AutoReply flow (context build + Gemini
				// call + validation + outbound message creation).
				//
				// Per ADR-014 (At-Least-Once + Idempotency): the webhook
				// handler returns 202 Accepted immediately; AutoReply runs
				// asynchronously. The outbox pattern ensures the reply is
				// delivered even if the webhook handler has already returned.
				autoReplyCmd := commands.AutoReplyCommand{
					Meta:                   commands.CommandMeta{Actor: commands.ActorContext{BusinessID: commands.BusinessID(connection.BusinessID)}},
					ConversationID:         commands.ConversationID(materialized.ConversationID),
					SourceMessageReference: event.ProviderMessageID,
					Text:                   event.Text,
					Channel:                string(event.Channel),
					ProviderRef:            string(event.Provider),
				}
				go func(cmd commands.AutoReplyCommand, businessID, conversationID string) {
					autoReplyCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
					defer cancel()
					if _, autoReplyErr := s.AutoReply.Handle(autoReplyCtx, cmd); autoReplyErr != nil {
						log.Printf("[Webhook] AUTO_REPLY_ERROR business=%s conversation=%s err=%v", businessID, conversationID, autoReplyErr)
					}
				}(autoReplyCmd, connection.BusinessID, materialized.ConversationID)
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

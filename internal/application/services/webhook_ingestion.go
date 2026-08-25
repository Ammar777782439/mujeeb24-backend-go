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
	Receiver    ports.WebhookReceiver
	RawPayloads ports.RawPayloadStore
	Connections ports.ChannelConnectionRepository
	Events      ports.EventStore
	Now         func() time.Time
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
		created, _, recordErr := s.Events.RecordIfAbsent(ctx, ports.InboundEventDraft{
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
	}
	return result, nil
}

type ChatwootWebhookService struct {
	Receiver ports.WebhookReceiver
}

// Chatwoot is an internal communication workspace, not Mujeeb's provider
// transport or source of customer truth. Valid callbacks are acknowledged and
// ignored to prevent mirror/echo loops until an explicit workspace callback
// use-case and mapping contract is approved.
func (s ChatwootWebhookService) Handle(ctx context.Context, command commands.IngestWebhookCommand) (commands.WebhookAcceptedResult, error) {
	if s.Receiver == nil {
		return commands.WebhookAcceptedResult{}, appErrors.NotImplemented()
	}
	if err := validateWebhookCommand(command); err != nil {
		return commands.WebhookAcceptedResult{}, err
	}
	if err := s.Receiver.VerifyWebhook(ctx, command.ProviderHeaders, command.RawPayload); err != nil {
		return commands.WebhookAcceptedResult{}, webhookAuthenticationError(err)
	}
	if _, err := s.Receiver.NormalizeWebhook(ctx, command.ProviderHeaders, command.RawPayload); err != nil {
		return commands.WebhookAcceptedResult{}, webhookAuthenticationError(err)
	}
	return commands.WebhookAcceptedResult{Accepted: true, Ignored: true, RequestID: command.RequestID}, nil
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
var _ commands.IngestChatwootWebhookHandler = ChatwootWebhookService{}

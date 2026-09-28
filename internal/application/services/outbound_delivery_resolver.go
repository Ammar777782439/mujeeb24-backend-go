package services

import (
	"context"
	"strings"

	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type MujeebOutboundDeliveryResolver struct {
	OutboundRepository   ports.OutboundMessageRepository
	ReferenceRepository  ports.ConversationReferenceRepository
	ConnectionRepository ports.ChannelConnectionRepository
}

func (r MujeebOutboundDeliveryResolver) Resolve(ctx context.Context, businessID, outboundMessageID string) (ports.OutboundDelivery, error) {
	if r.OutboundRepository == nil || r.ReferenceRepository == nil || r.ConnectionRepository == nil {
		return ports.OutboundDelivery{}, appErrors.NotImplemented()
	}
	outbound, err := r.OutboundRepository.GetByID(ctx, businessID, outboundMessageID)
	if err != nil {
		return ports.OutboundDelivery{}, mapAIRepositoryError(err)
	}
	if outbound.Status != "pending" && outbound.Status != "sending" {
		return ports.OutboundDelivery{}, appErrors.New(appErrors.CodeInvalidState, "outbound message is not deliverable")
	}
	connection, err := r.ConnectionRepository.GetByID(ctx, businessID, outbound.ConnectionID)
	if err != nil {
		return ports.OutboundDelivery{}, mapAIRepositoryError(err)
	}
	if connection.Status != "active" {
		return ports.OutboundDelivery{}, appErrors.New(appErrors.CodeInvalidState, "channel connection is not active")
	}
	if connection.ProviderReference != outbound.ProviderRef || connection.Channel != outbound.Channel {
		return ports.OutboundDelivery{}, appErrors.New(appErrors.CodeInvalidState, "outbound provider and channel do not match connection")
	}
	if strings.TrimSpace(providerAccountReferenceValue(connection)) == "" {
		return ports.OutboundDelivery{}, appErrors.New(appErrors.CodeInvalidState, "provider account reference is missing")
	}
	reference, err := r.ReferenceRepository.GetByID(ctx, businessID, outbound.ConversationReferenceID)
	if err != nil {
		return ports.OutboundDelivery{}, mapAIRepositoryError(err)
	}
	if !reference.IsCurrent || reference.System != "provider" || reference.ProviderRef != outbound.ProviderRef || reference.ConnectionID == nil || *reference.ConnectionID != connection.ID || strings.TrimSpace(reference.ResourceID) == "" {
		return ports.OutboundDelivery{}, appErrors.New(appErrors.CodeInvalidState, "provider conversation reference is not current or does not match connection")
	}
	text, err := DecodeInlineTextContentReference(outbound.ContentReference)
	if err != nil {
		return ports.OutboundDelivery{}, appErrors.New(appErrors.CodeInvalidState, "outbound content is not a supported inline text reference")
	}
	return ports.OutboundDelivery{ConnectionID: connection.ID, ProviderAccountID: providerAccountReferenceValue(connection), ProviderConversationID: reference.ResourceID, Text: text, IdempotencyKey: outbound.ProviderIdempotencyKey}, nil
}

func providerAccountReferenceValue(connection ports.ChannelConnectionRecord) string {
	if connection.ProviderAccountReference == nil {
		return ""
	}
	return strings.TrimSpace(*connection.ProviderAccountReference)
}

var _ ports.OutboundDeliveryResolver = MujeebOutboundDeliveryResolver{}

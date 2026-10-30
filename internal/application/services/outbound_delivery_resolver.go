package services

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

// ErrRequiredCatalogMediaUnavailable means a selected follow-up image became
// unresolvable; the worker must dead-letter it rather than send a caption alone.
var ErrRequiredCatalogMediaUnavailable = errors.New("required catalog media unavailable")

type MujeebOutboundDeliveryResolver struct {
	OutboundRepository   ports.OutboundMessageRepository
	ReferenceRepository  ports.ConversationReferenceRepository
	ConnectionRepository ports.ChannelConnectionRepository
	CatalogMedia         ports.CatalogMediaRepository
	MediaObjects         ports.MediaObjectStorage
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
	message, err := DecodeInteractiveContentReference(outbound.ContentReference)
	if err != nil {
		return ports.OutboundDelivery{}, appErrors.New(appErrors.CodeInvalidState, "outbound content is not a supported inline text reference")
	}
	delivery := ports.OutboundDelivery{ConnectionID: connection.ID, ProviderAccountID: providerAccountReferenceValue(connection), ProviderConversationID: reference.ResourceID, Text: message.Text, Buttons: message.Buttons, Channel: connection.Channel, IdempotencyKey: outbound.ProviderIdempotencyKey}
	if message.MediaID != "" && message.MediaLinkID != "" && r.CatalogMedia != nil && r.MediaObjects != nil {
		link, linkErr := r.CatalogMedia.FindApprovedMediaLink(ctx, businessID, message.MediaLinkID)
		if linkErr == nil && link != nil && link.MediaID == message.MediaID {
			asset, assetErr := r.CatalogMedia.GetAsset(ctx, businessID, message.MediaID)
			if assetErr == nil && asset.Status == "ready" && asset.DeletedAt == nil {
				url, urlErr := PresignCatalogAsset(ctx, r.MediaObjects, asset, 45*time.Minute)
				if urlErr == nil {
					delivery.Attachments = []ports.OutboundAttachment{{Type: "image", URL: url}}
				}
			}
		}
	}
	if message.MediaRequired && len(delivery.Attachments) != 1 {
		// Do not send a misleading image caption if the link was removed,
		// the asset became unavailable, or signed R2 access failed.
		log.Printf("[CatalogMedia] stage=dispatch_rejected business=%s outbound=%s media_link=%s reason=required_image_unavailable", businessID, outboundMessageID, message.MediaLinkID)
		return ports.OutboundDelivery{}, ErrRequiredCatalogMediaUnavailable
	}
	if message.MediaLinkID != "" {
		if len(delivery.Attachments) == 0 {
			log.Printf("[CatalogMedia] stage=dispatch_text_fallback business=%s outbound=%s media_link=%s reason=photo_unavailable", businessID, outboundMessageID, message.MediaLinkID)
		} else {
			log.Printf("[CatalogMedia] stage=dispatch_image_ready business=%s outbound=%s media_link=%s channel=%s", businessID, outboundMessageID, message.MediaLinkID, delivery.Channel)
		}
	}
	return delivery, nil
}

func providerAccountReferenceValue(connection ports.ChannelConnectionRecord) string {
	if connection.ProviderAccountReference == nil {
		return ""
	}
	return strings.TrimSpace(*connection.ProviderAccountReference)
}

var _ ports.OutboundDeliveryResolver = MujeebOutboundDeliveryResolver{}

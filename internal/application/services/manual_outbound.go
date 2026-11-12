package services

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/google/uuid"
)

type ManualOutboundMessageService struct {
	References              ports.ConversationReferenceRepository
	Connections             ports.ChannelConnectionRepository
	Capabilities            ports.ChannelCapabilityRepository
	MediaAttachmentsEnabled bool
	MediaAssets             ports.ConversationMediaAssetReader
	Attachments             ports.OutboundMessageAttachmentRepository
	Outbound                ports.OutboundMessageRepository
	Outbox                  ports.OutboxStore
	Messages                ports.MessageRepository
	Conversations           ports.ConversationRuntimeRepository
	Inbox                   ports.ConversationInboxRepository
	Transactions            ports.TransactionManager
	Realtime                ports.RealtimePublisher
	Now                     func() time.Time
	NewID                   func() string
}

func (s ManualOutboundMessageService) Handle(ctx context.Context, command commands.CreateOutboundMessageCommand) (commands.MessageResult, error) {
	if s.References == nil || s.Connections == nil || s.Outbound == nil || s.Outbox == nil || s.Transactions == nil || s.Messages == nil || s.Inbox == nil {
		return commands.MessageResult{}, appErrors.NotImplemented()
	}
	text := strings.TrimSpace(command.Text)
	idempotencyKey := strings.TrimSpace(command.Meta.IdempotencyKey)
	if idempotencyKey == "" {
		return commands.MessageResult{}, appErrors.New(appErrors.CodeValidation, "Idempotency-Key is required")
	}
	if len(command.AttachmentMediaIDs) > 1 {
		return commands.MessageResult{}, appErrors.New(appErrors.CodeValidation, "at most one image attachment is currently supported")
	}
	mediaIDs := make([]string, 0, len(command.AttachmentMediaIDs))
	seen := make(map[string]struct{}, len(command.AttachmentMediaIDs))
	for _, raw := range command.AttachmentMediaIDs {
		id, parseErr := uuid.Parse(strings.TrimSpace(raw))
		if parseErr != nil {
			return commands.MessageResult{}, appErrors.New(appErrors.CodeValidation, "attachment_media_ids must contain valid media asset UUIDs")
		}
		normalized := id.String()
		if _, exists := seen[normalized]; exists {
			return commands.MessageResult{}, appErrors.New(appErrors.CodeValidation, "duplicate attachment media asset")
		}
		seen[normalized] = struct{}{}
		mediaIDs = append(mediaIDs, normalized)
	}
	if text == "" && len(mediaIDs) == 0 {
		return commands.MessageResult{}, appErrors.New(appErrors.CodeValidation, "message text or at least one attachment is required")
	}
	if len(mediaIDs) > 0 && !s.MediaAttachmentsEnabled {
		return commands.MessageResult{}, appErrors.New(appErrors.CodeInvalidState, "outbound conversation media is disabled")
	}
	if len(mediaIDs) > 0 && (s.Capabilities == nil || s.MediaAssets == nil || s.Attachments == nil) {
		return commands.MessageResult{}, appErrors.NotImplemented()
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
		current, accessErr := readInboxConversation(txCtx, s.Inbox, command.Meta.Actor, conversationID, true)
		if accessErr != nil {
			return accessErr
		}
		if !authorization.CanReplyToConversation(command.Meta.Actor, current) {
			return appErrors.New(appErrors.CodeConflict, "استلم المحادثة قبل الرد، وتأكد أنها غير مغلقة.")
		}
		reference, referenceErr := s.References.GetCurrentByConversation(txCtx, businessID, conversationID, "provider")
		if referenceErr != nil {
			return mapAIRepositoryError(referenceErr)
		}
		if reference.MappingStatus != "active" || reference.ConnectionID == nil ||
			strings.TrimSpace(*reference.ConnectionID) == "" || strings.TrimSpace(reference.ResourceID) == "" {
			return appErrors.New(appErrors.CodeConflict, "current provider conversation reference is not sendable")
		}
		connection, connectionErr := s.Connections.GetByID(txCtx, businessID, *reference.ConnectionID)
		if connectionErr != nil {
			return mapAIRepositoryError(connectionErr)
		}
		if connection.Status != "active" || connection.ProviderReference != reference.ProviderRef || strings.TrimSpace(connection.Channel) == "" {
			return appErrors.New(appErrors.CodeConflict, "channel connection is not active for the conversation reference")
		}

		validatedAssets := make([]ports.MediaAsset, 0, len(mediaIDs))
		if len(mediaIDs) > 0 {
			capabilities, capabilityErr := s.Capabilities.ListByConnection(txCtx, businessID, connection.ID)
			if capabilityErr != nil {
				return mapAIRepositoryError(capabilityErr)
			}
			if !hasVerifiedChannelCapability(capabilities, "media_outbound") {
				return appErrors.New(appErrors.CodeConflict, "this channel has no verified outbound media capability")
			}
			for _, mediaID := range mediaIDs {
				asset, assetErr := s.MediaAssets.GetAsset(txCtx, businessID, mediaID)
				if assetErr != nil {
					return mapAIRepositoryError(assetErr)
				}
				if asset.BusinessID != businessID || asset.ID != mediaID || asset.Purpose != "conversation_attachment" {
					return appErrors.New(appErrors.CodeNotFound, "conversation media asset not found")
				}
				if asset.Status != "ready" || asset.DeletedAt != nil {
					return appErrors.New(appErrors.CodeConflict, "selected conversation media asset is not ready for outbound use")
				}
				mime := strings.ToLower(strings.TrimSpace(asset.MimeType))
				if asset.Kind != "image" || (mime != "image/jpeg" && mime != "image/png") {
					return appErrors.New(appErrors.CodeValidation, "only verified JPEG and PNG images are currently supported")
				}
				if strings.TrimSpace(asset.StorageProvider) == "" || strings.TrimSpace(asset.StorageKey) == "" {
					return appErrors.New(appErrors.CodeInvalidState, "conversation media storage reference is incomplete")
				}
				validatedAssets = append(validatedAssets, asset)
			}
		}

		contentReference := EncodeInlineTextContentReference(text)
		contentType := "text"
		if len(validatedAssets) > 0 {
			if text == "" {
				contentType = "image"
			} else {
				contentType = "mixed"
			}
		}
		var textContent *string
		if text != "" {
			textContent = &text
		}
		outbound, outboundErr := s.Outbound.CreatePending(txCtx, ports.OutboundMessageDraft{
			ID: newID(), BusinessID: businessID, ConversationID: conversationID,
			ConversationReferenceID: reference.ID, ConnectionID: *reference.ConnectionID,
			ProviderRef: reference.ProviderRef, Channel: connection.Channel, Origin: "human",
			Transport: "provider", ContentReference: contentReference,
			ProviderIdempotencyKey: dedupeKey, CorrelationID: uuidStringPointer(command.Meta.CorrelationID),
		})
		if outboundErr != nil {
			return mapAIRepositoryError(outboundErr)
		}

		author := string(command.Meta.Actor.PrincipalID)
		msgDraft := ports.CommunicationMessageDraft{
			AuthorPrincipalID: &author, ID: newID(), BusinessID: businessID,
			ConversationReferenceID: reference.ID, OutboundMessageID: &outbound.ID,
			Direction: "outbound", Origin: "human", Transport: "provider",
			ContentType: contentType, TextContent: textContent, ContentReference: contentReference,
			Visibility: "public", OccurredAt: now, CreatedAt: now,
		}
		recordedMessage, msgErr := s.Messages.Record(txCtx, msgDraft)
		if msgErr != nil {
			return mapAIRepositoryError(msgErr)
		}

		createdAttachments := make([]ports.MessageAttachmentRecord, 0, len(validatedAssets))
		for sortOrder, asset := range validatedAssets {
			attachment, attachmentErr := s.Attachments.CreateReadyOutbound(txCtx, ports.MessageAttachmentDraft{
				ID: newID(), BusinessID: businessID, CommunicationMessageID: recordedMessage.ID,
				MediaAssetID: asset.ID, Kind: "image", SortOrder: sortOrder, CreatedAt: now,
			})
			if attachmentErr != nil {
				return mapAIRepositoryError(attachmentErr)
			}
			createdAttachments = append(createdAttachments, attachment)
		}

		if _, outboxErr := s.Outbox.Enqueue(txCtx, ports.OutboxEntryDraft{
			ID: newID(), BusinessID: businessID, OutboundMessageID: outbound.ID,
			CommandType: OutboundSendCommandType, DedupeKey: dedupeKey,
			AvailableAt: now, CreatedAt: now, UpdatedAt: now,
		}); outboxErr != nil {
			return mapAIRepositoryError(outboxErr)
		}
		if s.Conversations != nil {
			waitingCustomerState, humanOwnership := "waiting_customer", "human"
			if _, convErr := s.Conversations.TransitionLifecycle(txCtx, ports.ConversationLifecycleTransition{
				BusinessID: businessID, ConversationID: conversationID,
				State: &waitingCustomerState, Ownership: &humanOwnership, LastActivityAt: &now,
			}); convErr != nil {
				return mapAIRepositoryError(convErr)
			}
		}

		recordedMessage.ConversationID = conversationID
		result.Message = messageView(recordedMessage)
		for _, attachment := range createdAttachments {
			result.Message.Attachments = append(result.Message.Attachments, commands.MessageAttachmentView{
				AttachmentID: attachment.ID, Type: attachment.Kind, MediaID: attachment.MediaAssetID,
				Caption: attachment.Caption, Status: attachment.Status,
			})
		}
		return nil
	})
	if err == nil && s.Realtime != nil {
		data, _ := json.Marshal(result.Message)
		var correlationID *string
		if command.Meta.CorrelationID != "" {
			correlationID = &command.Meta.CorrelationID
		}
		_ = s.Realtime.Publish(ctx, ports.RealtimeEvent{
			EventID: uuid.NewString(), EventType: "conversation.message_sent",
			BusinessID: businessID, ResourceType: "conversation", ResourceID: conversationID,
			OccurredAt: now, CorrelationID: correlationID, Data: data,
		})
	}
	return result, err
}

var _ commands.CreateOutboundMessageHandler = ManualOutboundMessageService{}

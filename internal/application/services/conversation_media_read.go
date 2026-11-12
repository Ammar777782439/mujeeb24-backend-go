package services

import (
	"context"
	"errors"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

const conversationAttachmentReadURLTTL = 5 * time.Minute

type ConversationAttachmentReadService struct {
	Inbox       ports.ConversationInboxRepository
	Attachments ports.ConversationAttachmentReadRepository
	Objects     ports.MediaObjectProviderReader
}

func (s ConversationAttachmentReadService) Handle(ctx context.Context, query queries.GetConversationAttachmentReadURLQuery) (commands.ConversationAttachmentReadURLView, error) {
	if s.Inbox == nil || s.Attachments == nil || s.Objects == nil {
		return commands.ConversationAttachmentReadURLView{}, appErrors.NotImplemented()
	}
	if query.Meta.Actor.BusinessID == "" || query.ConversationID == "" ||
		query.MessageID == "" || query.AttachmentID == "" {
		return commands.ConversationAttachmentReadURLView{}, appErrors.New(appErrors.CodeValidation, "business, conversation, message and attachment ids are required")
	}
	if _, err := readInboxConversation(ctx, s.Inbox, query.Meta.Actor, string(query.ConversationID), false); err != nil {
		return commands.ConversationAttachmentReadURLView{}, err
	}
	attachment, err := s.Attachments.GetReadyForMessage(
		ctx,
		string(query.Meta.Actor.BusinessID),
		string(query.ConversationID),
		string(query.MessageID),
		query.AttachmentID,
	)
	if err != nil {
		return commands.ConversationAttachmentReadURLView{}, err
	}
	if attachment.StorageProvider == "" || attachment.StorageKey == "" {
		return commands.ConversationAttachmentReadURLView{}, appErrors.New(appErrors.CodeNotFound, "conversation attachment not found")
	}
	url, err := s.Objects.PresignGetFromProvider(ctx, attachment.StorageProvider, attachment.StorageKey, conversationAttachmentReadURLTTL)
	if err != nil {
		return commands.ConversationAttachmentReadURLView{}, &appErrors.Error{
			Code: appErrors.CodeExternalDependency, Message: "private attachment access is temporarily unavailable",
			Retryable: true, Cause: err,
		}
	}
	if url == "" {
		return commands.ConversationAttachmentReadURLView{}, errors.New("private object storage returned an empty signed URL")
	}
	return commands.ConversationAttachmentReadURLView{
		URL: url, ExpiresInSeconds: int(conversationAttachmentReadURLTTL.Seconds()),
		MimeType: attachment.Attachment.MimeType, SizeBytes: attachment.Attachment.SizeBytes,
	}, nil
}

var _ queries.GetConversationAttachmentReadURLHandler = ConversationAttachmentReadService{}

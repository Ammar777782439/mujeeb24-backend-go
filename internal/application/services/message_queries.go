package services

import (
	"context"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	appErrors "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/errors"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type MessageQueryService struct {
	Repository ports.MessageRepository
	Inbox      ports.ConversationInboxRepository
}

func (s MessageQueryService) Handle(ctx context.Context, query queries.ListConversationMessagesQuery) (commands.ListResult[commands.MessageView], error) {
	return s.ListConversationMessages(ctx, query)
}

func (s MessageQueryService) ListConversationMessages(ctx context.Context, query queries.ListConversationMessagesQuery) (commands.ListResult[commands.MessageView], error) {
	if s.Repository == nil {
		return commands.ListResult[commands.MessageView]{}, appErrors.NotImplemented()
	}
	if _, err := readInboxConversation(ctx, s.Inbox, query.Meta.Actor, string(query.ConversationID), false); err != nil {
		return commands.ListResult[commands.MessageView]{}, err
	}
	page, err := s.Repository.ListByConversation(ctx, string(query.Meta.Actor.BusinessID), string(query.ConversationID), query.Limit, query.Cursor)
	if err != nil {
		return commands.ListResult[commands.MessageView]{}, err
	}
	items := make([]commands.MessageView, 0, len(page.Items))
	for _, record := range page.Items {
		items = append(items, messageView(record))
	}
	return commands.ListResult[commands.MessageView]{Items: items, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

var _ queries.ListConversationMessagesHandler = MessageQueryService{}

func messageView(record ports.CommunicationMessageRecord) commands.MessageView {
	text := ""
	if record.TextContent != nil {
		text = *record.TextContent
	}
	view := commands.MessageView{
		ID: commands.MessageID(record.ID), ConversationID: commands.ConversationID(record.ConversationID),
		Direction: record.Direction, Origin: record.Origin, Status: record.Status, Text: text,
		ContentType: record.ContentType, ProviderMessageReference: record.ProviderMessageID,
		AuthorPrincipalID: record.AuthorPrincipalID, AuthorDisplayName: record.AuthorDisplayName,
		OccurredAt: record.OccurredAt, CreatedAt: record.CreatedAt, Private: record.Visibility == "private",
	}
	if view.Private {
		if reference, ok := decodePrivateNoteReference(record.ContentReference); ok {
			view.RelatedMessageID = &reference.RelatedMessageID
			view.RelatedMessageText = &reference.Quote
		}
	}
	if record.Visibility == "public" && record.Direction == "outbound" && record.ContentType == "mixed" {
		message, err := DecodeInteractiveContentReference(record.ContentReference)
		if err == nil && message.MediaID != "" && message.MediaLinkID != "" {
			view.Attachments = []commands.MessageAttachmentView{{Type: "image", MediaID: message.MediaID}}
		}
	}
	return view
}

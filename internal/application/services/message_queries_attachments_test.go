package services

import (
	"context"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type messageAttachmentsQueryRepositoryFake struct {
	page ports.MessagePage
}
func (f messageAttachmentsQueryRepositoryFake) Record(context.Context, ports.CommunicationMessageDraft) (ports.CommunicationMessageRecord, error) {
	return ports.CommunicationMessageRecord{}, nil
}
func (f messageAttachmentsQueryRepositoryFake) ListByConversation(context.Context, string, string, int, string) (ports.MessagePage, error) {
	return f.page, nil
}

type messageAttachmentsBatchReaderFake struct {
	records []ports.MessageAttachmentRecord
	messageIDs []string
}
func (f *messageAttachmentsBatchReaderFake) ListByMessages(_ context.Context, _ string, ids []string) ([]ports.MessageAttachmentRecord, error) {
	f.messageIDs = append([]string(nil), ids...)
	return f.records, nil
}

func TestMessageQueryServiceProjectsInboundAttachmentsWithoutPrivateMetadata(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	publicText, privateText := "صورة من العميل", "ملاحظة داخلية"
	repository := messageAttachmentsQueryRepositoryFake{page: ports.MessagePage{Items: []ports.CommunicationMessageRecord{
		{ID: "message-public", BusinessID: "business-1", ConversationID: "conversation-1", Direction: "inbound", Visibility: "public", ContentType: "text", TextContent: &publicText, OccurredAt: now, CreatedAt: now, Status: "received"},
		{ID: "message-private", BusinessID: "business-1", ConversationID: "conversation-1", Direction: "outbound", Visibility: "private", ContentType: "text", TextContent: &privateText, OccurredAt: now, CreatedAt: now, Status: "sent"},
	}}}
	attachments := &messageAttachmentsBatchReaderFake{records: []ports.MessageAttachmentRecord{
		{ID: "attachment-public", BusinessID: "business-1", CommunicationMessageID: "message-public", MediaAssetID: "asset-public", Kind: "image", Caption: "صورة", Status: "ready", SizeBytes: 321},
		{ID: "attachment-private", BusinessID: "business-1", CommunicationMessageID: "message-private", MediaAssetID: "asset-private", Kind: "file", Caption: "ملف خاص", Status: "ready"},
	}}
	inbox := conversationAttachmentReadInboxFake{record: ports.ConversationRecord{
		ID: "conversation-1", BusinessID: "business-1", State: "open",
	}}
	service := MessageQueryService{Repository: repository, Inbox: inbox, Attachments: attachments}
	actor := commands.ActorContext{
		BusinessID: "business-1", PrincipalID: "principal-1", Role: "owner",
		Permissions: []string{authorization.PermissionConversationsRead},
	}
	page, err := service.ListConversationMessages(context.Background(), queries.ListConversationMessagesQuery{
		Meta: commands.QueryMeta{Actor: actor}, ConversationID: "conversation-1", Limit: 20,
	})
	if err != nil {
		t.Fatalf("ListConversationMessages: %v", err)
	}
	if len(page.Items) != 2 || len(attachments.messageIDs) != 2 ||
		len(page.Items[0].Attachments) != 1 || page.Items[0].Attachments[0].AttachmentID != "attachment-public" ||
		page.Items[0].Attachments[0].MediaID != "asset-public" || page.Items[0].Attachments[0].Caption != "صورة" ||
		len(page.Items[1].Attachments) != 0 {
		t.Fatalf("unexpected message attachment projection: page=%#v queried=%#v", page, attachments.messageIDs)
	}
}

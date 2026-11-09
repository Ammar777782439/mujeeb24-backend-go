package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/authorization"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/queries"
)

type conversationAttachmentReadInboxFake struct {
	record ports.ConversationRecord
	err error
}

func (f conversationAttachmentReadInboxFake) ListInbox(context.Context, ports.ConversationInboxFilter) (ports.ConversationPage, error) {
	return ports.ConversationPage{}, nil

}

func (f conversationAttachmentReadInboxFake) InboxOverview(context.Context, ports.ConversationInboxFilter) (ports.ConversationInboxOverview, error) {
	return ports.ConversationInboxOverview{}, nil
}
func (f conversationAttachmentReadInboxFake) GetInboxConversation(context.Context, string, string, ports.ConversationAccess, bool) (ports.ConversationRecord, error) {
	return f.record, f.err
}

type conversationAttachmentReadRepositoryFake struct {
	record ports.ConversationAttachmentReadRecord
	err error
	calls int

}

func (f *conversationAttachmentReadRepositoryFake) GetReadyForMessage(context.Context, string, string, string, string) (ports.ConversationAttachmentReadRecord, error) {
	f.calls++
	return f.record, f.err
}

type conversationAttachmentObjectReaderFake struct {
	provider string
	key string
	ttl time.Duration
	url string
	err error
	calls int

}

func (f *conversationAttachmentObjectReaderFake) PresignGetFromProvider(_ context.Context, provider, key string, ttl time.Duration) (string, error) {
	f.calls++
	f.provider, f.key, f.ttl = provider, key, ttl
	return f.url, f.err
}

func TestConversationAttachmentReadServiceUsesTenantConversationAndPinnedStorage(t *testing.T) {
	inbox := conversationAttachmentReadInboxFake{record: ports.ConversationRecord{
		ID: "conversation-1", BusinessID: "business-1", State: "open",
	}}
	attachments := &conversationAttachmentReadRepositoryFake{record: ports.ConversationAttachmentReadRecord{
		Attachment: ports.MessageAttachmentRecord{
			ID: "attachment-1", MediaAssetID: "asset-1", MimeType: "image/png", SizeBytes: 1234, Status: "ready",
		},
		StorageProvider: "backblaze", StorageKey: "conversation/business-1/asset-1",
	}}
	objects := &conversationAttachmentObjectReaderFake{url: "https://private.example/signed"}
	service := ConversationAttachmentReadService{Inbox: inbox, Attachments: attachments, Objects: objects}
	actor := commands.ActorContext{
		BusinessID: "business-1", PrincipalID: "principal-1", Role: "owner",
		Permissions: []string{authorization.PermissionConversationsRead},
	}
	view, err := service.Handle(context.Background(), queries.GetConversationAttachmentReadURLQuery{
		Meta: commands.QueryMeta{Actor: actor}, ConversationID: "conversation-1",
		MessageID: "message-1", AttachmentID: "attachment-1",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if view.URL != objects.url || view.ExpiresInSeconds != 300 || view.MimeType != "image/png" || view.SizeBytes != 1234 {
		t.Fatalf("unexpected read URL view: %#v", view)
	}
	if objects.calls != 1 || objects.provider != "backblaze" || objects.key != "conversation/business-1/asset-1" || objects.ttl != 5*time.Minute {
		t.Fatalf("signed URL did not use asset-pinned provider and key: %#v", objects)
	}
	if attachments.calls != 1 {
		t.Fatalf("expected one tenant-scoped attachment lookup, got %d", attachments.calls)
	}
}

func TestConversationAttachmentReadServiceDoesNotSignWhenConversationIsNotReadable(t *testing.T) {
	inbox := conversationAttachmentReadInboxFake{record: ports.ConversationRecord{
		ID: "conversation-1", BusinessID: "business-1", State: "open",
	}}
	attachments := &conversationAttachmentReadRepositoryFake{}
	objects := &conversationAttachmentObjectReaderFake{url: "https://private.example/signed"}
	service := ConversationAttachmentReadService{Inbox: inbox, Attachments: attachments, Objects: objects}
	actor := commands.ActorContext{BusinessID: "business-1", PrincipalID: "principal-1", Role: "agent"}
	_, err := service.Handle(context.Background(), queries.GetConversationAttachmentReadURLQuery{
		Meta: commands.QueryMeta{Actor: actor}, ConversationID: "conversation-1",
		MessageID: "message-1", AttachmentID: "attachment-1",
	})
	if err == nil || attachments.calls != 0 || objects.calls != 0 {
		t.Fatalf("unreadable conversation must not reach attachment storage: err=%v repo=%d storage=%d", err, attachments.calls, objects.calls)
	}
}

func TestConversationAttachmentReadServiceDoesNotSignMissingOrUnreadyAttachment(t *testing.T) {
	inbox := conversationAttachmentReadInboxFake{record: ports.ConversationRecord{
		ID: "conversation-1", BusinessID: "business-1", State: "open",
	}}
	attachments := &conversationAttachmentReadRepositoryFake{err: errors.New("not found")}
	objects := &conversationAttachmentObjectReaderFake{url: "https://private.example/signed"}
	service := ConversationAttachmentReadService{Inbox: inbox, Attachments: attachments, Objects: objects}
	actor := commands.ActorContext{
		BusinessID: "business-1", PrincipalID: "principal-1", Role: "owner",
		Permissions: []string{authorization.PermissionConversationsRead},
	}
	_, err := service.Handle(context.Background(), queries.GetConversationAttachmentReadURLQuery{
		Meta: commands.QueryMeta{Actor: actor}, ConversationID: "conversation-1",
		MessageID: "message-1", AttachmentID: "attachment-1",
	})
	if err == nil || attachments.calls != 1 || objects.calls != 0 {
		t.Fatalf("missing/unready attachment must not be signed: err=%v repo=%d storage=%d", err, attachments.calls, objects.calls)
	}
}

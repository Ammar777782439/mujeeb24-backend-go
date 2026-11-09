package handlers

import (
	"testing"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/commands"
)

func TestMessageListUsesConversationScopedReadPathForInboundAttachments(t *testing.T) {
	page := commands.ListResult[commands.MessageView]{Items: []commands.MessageView{{
		ID: "message-1", ConversationID: "conversation-1",
		Attachments: []commands.MessageAttachmentView{{
			AttachmentID: "attachment-1", Type: "image", MediaID: "asset-1",
			Caption: "صورة العميل", Status: "ready",
		}},
	}}}
	result := messageList(page, "business-1")
	if len(result.Body.Data) != 1 || len(result.Body.Data[0].Attachments) != 1 {
		t.Fatalf("unexpected message projection: %#v", result)
	}
	attachment := result.Body.Data[0].Attachments[0]
	wantPath := "/api/v1/businesses/business-1/conversations/conversation-1/messages/message-1/attachments/attachment-1/read-url"
	if attachment.AttachmentID == nil || string(*attachment.AttachmentID) != "attachment-1" ||
		attachment.MediaID != "asset-1" || attachment.Type != "image" ||
		attachment.Caption != "صورة العميل" || attachment.Status != "ready" || attachment.PreviewPath != wantPath {
		t.Fatalf("conversation attachment metadata or scoped read path is incorrect: %#v", attachment)
	}
}

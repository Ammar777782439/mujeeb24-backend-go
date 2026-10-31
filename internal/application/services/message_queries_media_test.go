package services

import (
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

func TestMerchantMessageProjectionPreservesSelectedPhotoWithoutProviderMetadata(t *testing.T) {
	encoded, err := EncodeInteractiveContentReference(ports.InteractiveMessage{
		Text: "هذه صورة المنتج", MediaID: "asset-id", MediaLinkID: "media-link-id",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	text := "هذه صورة المنتج"
	record := ports.CommunicationMessageRecord{
		ID: "message-id", ConversationID: "conversation-id", Direction: "outbound",
		Origin: "ai", Status: "failed", ContentType: "mixed",
		TextContent: &text, ContentReference: encoded, Visibility: "public", CreatedAt: now, OccurredAt: now,
	}
	view := messageView(record)
	if view.Status != "failed" || view.ContentType != "mixed" || len(view.Attachments) != 1 ||
		view.Attachments[0].Type != "image" || view.Attachments[0].MediaID != "asset-id" || view.Text != text {
		t.Fatalf("selected merchant photo was lost in response: %+v", view)
	}
	if view.ProviderMessageReference != nil {
		t.Fatal("provider identifiers must not be manufactured")
	}
	record.Visibility = "private"
	if len(messageView(record).Attachments) != 0 {
		t.Fatal("private records must not emit media attachments")
	}
	record.Visibility = "public"
	record.ContentType = "text"
	if len(messageView(record).Attachments) != 0 {
		t.Fatal("text-only messages must not emit media")
	}
}

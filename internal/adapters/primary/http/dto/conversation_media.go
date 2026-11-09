package dto

type ConversationAttachmentReadInput struct {
	BusinessID     UUID `path:"business_id" format:"uuid"`
	ConversationID UUID `path:"conversation_id" format:"uuid"`
	MessageID      UUID `path:"message_id" format:"uuid"`
	AttachmentID   UUID `path:"attachment_id" format:"uuid"`
}

type ConversationAttachmentReadURLView struct {
	URL              string `json:"url"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
	MimeType         string `json:"mime_type"`
	SizeBytes        int64  `json:"size_bytes"`
}

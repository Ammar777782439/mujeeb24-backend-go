package ports

import "context"

// CustomerMediaOption contains only approved gallery evidence, never R2 keys or signed URLs.
type CustomerMediaOption struct {
	MediaLinkID string  `json:"media_link_id"`
	ItemID      string  `json:"item_id"`
	VariantID   *string `json:"variant_id"`
	AltText     string  `json:"alt_text"`
	IsPrimary   bool    `json:"is_primary"`
	SortOrder   int     `json:"sort_order"`
}

type CustomerMediaSelectionInput struct {
	BusinessID      string
	ConversationID  string
	RunID           string
	CustomerMessage string
	ResponseText    string
	Options         []CustomerMediaOption
}

type CustomerMediaSelectionOutput struct {
	MediaLinkIDs []string
	// ReplyText is authored after image selection, not a delivery confirmation.
	ReplyText string
	Usage        CustomerSalesUsageTelemetry
}

// CustomerMediaSelectionPort proposes media IDs only; it cannot perform an outbound send.
type CustomerMediaSelectionPort interface {
	SelectMedia(context.Context, CustomerMediaSelectionInput) (CustomerMediaSelectionOutput, error)
}

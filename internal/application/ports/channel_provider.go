package ports

import (
	"context"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/channel"
)

type WebhookReceiver interface {
	VerifyWebhook(ctx context.Context, headers map[string]string, rawBody []byte) error
	NormalizeWebhook(ctx context.Context, headers map[string]string, rawBody []byte) ([]channel.InboundEvent, error)
}

type ChannelProvider interface {
	WebhookReceiver
	SendMessage(ctx context.Context, command SendMessageCommand) (ProviderSendResult, error)
	GetDeliveryStatus(ctx context.Context, reference DeliveryReference) (channel.DeliveryStatus, error)
}

type SendMessageCommand struct {
	ConnectionID           string
	ProviderAccountID      string
	ProviderConversationID string
	Text                   string
	IdempotencyKey         string
}

type ProviderSendResult struct {
	ProviderRequestID string
	ProviderMessageID string
	Status            channel.DeliveryStatus
}

type DeliveryReference struct {
	ConnectionID           string
	ProviderAccountID      string
	ProviderConversationID string
	ProviderMessageID      string
	OutboundMessageID      string
}

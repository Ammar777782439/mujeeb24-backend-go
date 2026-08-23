package ports

import (
	"context"
	"net/http"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/domain/channel"
)

type ChannelProvider interface {
	VerifyWebhook(ctx context.Context, headers http.Header, rawBody []byte) error
	NormalizeWebhook(ctx context.Context, headers http.Header, rawBody []byte) ([]channel.InboundEvent, error)
	SendMessage(ctx context.Context, command SendMessageCommand) (ProviderSendResult, error)
	GetDeliveryStatus(ctx context.Context, reference DeliveryReference) (channel.DeliveryStatus, error)
}

type SendMessageCommand struct {
	ConnectionID            string
	ProviderConversationID  string
	Text                    string
	IdempotencyKey          string
}

type ProviderSendResult struct {
	ProviderRequestID string
	ProviderMessageID string
	Status            channel.DeliveryStatus
}

type DeliveryReference struct {
	ConnectionID       string
	ProviderMessageID  string
	OutboundMessageID  string
}

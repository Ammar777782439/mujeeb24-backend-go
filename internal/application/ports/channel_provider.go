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

// ConversationProfile is the customer-identity snapshot returned by a provider
// (e.g. SocialAPI's GET /v1/inbox/conversations/{id}) when enriching a customer
// record after an inbound DM webhook.
//
// The provider's webhook payload for DM events carries only `author.id` (the
// platform user ID — WhatsApp wa_id / Facebook PSID / Instagram IGSID). The
// customer's name and profile picture are NOT in the webhook payload; they
// must be fetched via the provider's REST API and merged into the local
// customer.profile JSONB column.
//
// `ParticipantID` is the platform user ID returned by the REST endpoint.
// Callers MUST verify it matches the webhook's `ExternalUserID` to defend
// against any cross-merchant contamination (the provider API key is shared
// across all merchants — the conversation_id is the per-merchant anchor).
type ConversationProfile struct {
	ParticipantID      string
	ParticipantName    string
	ParticipantPicture string
}

// ConversationEnricher is an OPTIONAL interface that a WebhookReceiver may
// implement. When wired into SocialAPIWebhookService.Enricher, the webhook
// handler will call GetConversationProfile after Materialize to fetch the
// customer's display name / picture from the provider's REST API and merge
// them into the local customer record.
//
// Implementations MUST be safe for concurrent use across multiple merchants
// (the underlying provider client uses a single API key; per-merchant
// isolation is anchored by the conversation_id passed in).
type ConversationEnricher interface {
	GetConversationProfile(ctx context.Context, providerConversationID string) (ConversationProfile, error)
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

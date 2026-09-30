package channel

import "time"

type Provider string

const (
	ProviderSocialAPI Provider = "socialapi"
)

type Channel string

const (
	ChannelFacebook  Channel = "facebook"
	ChannelInstagram Channel = "instagram"
	ChannelWhatsApp  Channel = "whatsapp"
)

type InteractionKind string

const (
	InteractionDM         InteractionKind = "dm"
	InteractionComment    InteractionKind = "comment"
	InteractionStoryReply InteractionKind = "story_reply"
	InteractionMention    InteractionKind = "mention"
	InteractionOther      InteractionKind = "other"
)

type ConnectionStatus string

const (
	ConnectionPending           ConnectionStatus = "pending"
	ConnectionConnecting        ConnectionStatus = "connecting"
	ConnectionActive            ConnectionStatus = "active"
	ConnectionDegraded          ConnectionStatus = "degraded"
	ConnectionDisconnected      ConnectionStatus = "disconnected"
	ConnectionReconnectRequired ConnectionStatus = "reconnect_required"
	ConnectionFailed            ConnectionStatus = "failed"
)

type MessageDirection string

const (
	DirectionInbound  MessageDirection = "inbound"
	DirectionOutbound MessageDirection = "outbound"
)

type MessageOrigin string

const (
	OriginCustomer   MessageOrigin = "customer"
	OriginAI         MessageOrigin = "ai"
	OriginHuman      MessageOrigin = "human"
	OriginAutomation MessageOrigin = "automation"
	OriginSystem     MessageOrigin = "system"
)

type DeliveryStatus string

const (
	DeliveryPending    DeliveryStatus = "pending"
	DeliverySending    DeliveryStatus = "sending"
	DeliveryAccepted   DeliveryStatus = "accepted"
	DeliverySent       DeliveryStatus = "sent"
	DeliveryDelivered  DeliveryStatus = "delivered"
	DeliveryRead       DeliveryStatus = "read"
	DeliveryFailed     DeliveryStatus = "failed"
	DeliveryDeadLetter DeliveryStatus = "dead_letter"
	DeliveryUnknown    DeliveryStatus = "unknown"
)

type InboundEvent struct {
	ID                     string
	BusinessID             string
	ConnectionID           string
	Provider               Provider
	Channel                Channel
	ProviderConnectionID   string
	ProviderEventID        string
	DeliveryID             string
	DedupeStrategy         string
	EventType              string
	InteractionKind        InteractionKind
	ProviderMessageID      string
	ProviderConversationID string
	ExternalUserID         string
	Text                   string
	MessageType            string
	Direction              MessageDirection
	Origin                 MessageOrigin
	Private                bool
	SenderType             string
	DeliveryStatus         string
	ExternalCreatedAt      *time.Time
	ReceivedAt             time.Time
	RawPayloadReference    string
}

type OutboundMessage struct {
	ID                      string
	BusinessID              string
	ConversationReferenceID string
	Channel                 Channel
	Origin                  MessageOrigin
	Content                 string
	ProviderIdempotencyKey  string
	Status                  DeliveryStatus
	ProviderMessageID       string
	AttemptCount            int
	NextRetryAt             *time.Time
}

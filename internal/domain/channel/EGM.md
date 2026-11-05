# internal/domain/channel

## Overview
Defines core domain entities, value types, and lifecycle statuses for omnichannel communication channels (WhatsApp, Instagram, Facebook), event ingestion, and outbound message dispatches.

SocialAPI Gateway Role: `SocialAPI` is the external unified messaging transport gateway providing webhook delivery and API dispatch for WhatsApp Cloud API, Instagram Graph API, and Facebook Messenger.

## Interface
- `Provider`: Provider identifier enum (`ProviderSocialAPI`) ([channel.go](./channel.go#L5)).
- `Channel`: Supported social channel platforms (`ChannelFacebook`, `ChannelInstagram`, `ChannelWhatsApp`) ([channel.go](./channel.go#L11)).
- `InteractionKind`: Classification of incoming message interaction (`InteractionDM`, `InteractionComment`, `InteractionStoryReply`, `InteractionMention`, `InteractionOther`) ([channel.go](./channel.go#L19)).
- `ConnectionStatus`: Health and authorization state of a channel connection (`ConnectionActive`, `ConnectionDegraded`, `ConnectionDisconnected`, `ConnectionReconnectRequired`, etc.) ([channel.go](./channel.go#L29)).
- `MessageDirection` / `MessageOrigin`: Traffic direction (`DirectionInbound`, `DirectionOutbound`) and originator (`OriginCustomer`, `OriginAI`, `OriginHuman`, `OriginAutomation`, `OriginSystem`) ([channel.go](./channel.go#L41-L56)).
- `DeliveryStatus`: Outbox and provider delivery lifecycle (`DeliveryPending`, `DeliverySending`, `DeliverySent`, `DeliveryDelivered`, `DeliveryDeadLetter`, `DeliveryUnknown`, etc.) ([channel.go](./channel.go#L58-L70)).
- `InboundEvent`: Canonical domain representation of an ingested social webhook event ([channel.go](./channel.go#L72)).
- `OutboundMessage`: Canonical domain representation of an outgoing channel dispatch ([channel.go](./channel.go#L101)).

## Invariants
- `ProviderSocialAPI` is the single supported external social communication provider enum.
- No network calls, SQL drivers, or third-party SDKs are imported into this package.
- Delivery Lifecycle & Dead-Letter Isolation: `DeliveryUnknown` represents an ephemeral unconfirmed provider transmission state. To prevent duplicate customer messaging, unknown transmission results must transition directly to `DeliveryDeadLetter` terminal quarantine rather than triggering automated retries.

## Side Effects
- None (pure domain definitions).

## Verification
- Run: `go test ./internal/domain/...`

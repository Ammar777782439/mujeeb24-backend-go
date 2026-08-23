# Channel Orchestration Contract

## Provider-independent domain concepts

The Domain knows only `Channel`, `InboundEvent`, `CustomerIdentity`, `ConversationReference`, `CommunicationMessage`, and `OutboundMessage`. SocialAPI and Chatwoot DTOs remain inside adapters.

## Inbound contract

```text
provider webhook
  -> verify signature on raw bytes
  -> resolve channel connection
  -> idempotency key: provider + provider_connection_id + provider_event_id
  -> persist durable inbound event
  -> persist outbox task
  -> ACK 202
  -> process asynchronously
```

`ACK` means the event was accepted for processing. It does not mean the customer received a response.

## Outbound contract

```text
merchant / AI / automation
  -> authorization and policy
  -> durable outbound message + outbox task
  -> provider adapter send
  -> delivery status webhook or reconciliation
  -> Chatwoot mirror
```

A network ambiguity produces `unknown`; it must not cause a blind resend. The same logical outbound operation keeps the same provider idempotency key when the provider supports it. Provider idempotency support must be verified by contract tests.

## Loop prevention

Every communication message records:

```text
direction: inbound | outbound
origin: customer | ai | human | automation | system
transport: provider | chatwoot | mujeeb
```

A Chatwoot mirror event for a Mujeeb-created outbound message is recorded but is never sent back to the provider as a new customer event.

## Tenant resolution

Provider events resolve to `channel_connections`, then to `business_id`. Tenant identity is never inferred from display names or message content.

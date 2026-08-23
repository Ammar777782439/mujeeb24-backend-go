# Mujeeb 24 Backend Architecture

## System boundary

```text
SocialAPI.ai = external channel transport
Chatwoot     = internal communication workspace
Mujeeb Go    = orchestration, AI, sales domain, tenant boundary
Dashboard    = merchant-facing application
```

The Go application owns the integration contract and the commercial domain. It does not own or replicate the internal database of SocialAPI.ai or Chatwoot.

## Inbound

```text
Provider webhook
  -> signature verification
  -> connection resolution
  -> idempotency insert
  -> durable event commit
  -> outbox task
  -> normalization
  -> identity resolution
  -> conversation mapping
  -> Chatwoot mirror
  -> AI and sales processing
```

The webhook handler acknowledges only after the event and its processing task are durably stored. The acknowledgement does not mean that a customer reply was sent.

## Outbound

```text
Merchant / AI / automation
  -> authorization and policy
  -> outbound message + outbox transaction
  -> provider adapter
  -> SocialAPI send API
  -> delivery status webhook
  -> outbound state update
  -> Chatwoot mirror
```

A provider call that ends with an ambiguous network result produces `unknown`, not an automatic retry. Reconciliation must resolve whether the provider accepted the message before another send is attempted.

## Domain isolation

The Domain uses concepts such as `Channel`, `InboundEvent`, `CustomerIdentity`, `ConversationReference`, `OutboundMessage`, `CatalogItem`, `Lead`, and `CommercialTransaction`. Vendor-specific DTOs stay inside their adapters.

## Runtime shape

The first deployment is a modular monolith with two processes:

- `api`: merchant API and signed webhook ingress.
- `worker`: outbox dispatch, inbound processing, outbound delivery, and reconciliation.

PostgreSQL is the durable source for integration state. Redis/Asynq is a delivery accelerator and worker mechanism, not the only source of truth.

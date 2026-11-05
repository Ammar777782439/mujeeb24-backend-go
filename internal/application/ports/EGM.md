# internal/application/ports

## Overview
Defines the application boundary interfaces (Hexagonal Inbound and Outbound Ports). These abstract interfaces decouple application use-cases, commands, and query services from concrete PostgreSQL persistence, SocialAPI networking, AI vendor clients, and real-time pub/sub publishers.

## Interface
- `TransactionManager`: Atomic transaction application boundary (`Within(ctx, fn)`) ([transaction.go](./transaction.go#L8)).
- `CustomerRepository` / `CustomerRuntimeRepository`: Customer persistence, versioned updates, and profile merging ([core_repositories.go](./core_repositories.go#L20)).
- `CatalogRepository` / `CatalogMediaRepository`: Product catalogs, attributes, variants, offers, and media attachment assets ([catalog_repository.go](./catalog_repository.go) / [catalog_media.go](./catalog_media.go)).
- `OutboxStore`: Outbox pattern state transitions, claim leases, completions, `MarkRetryableFailure`, `Requeue`, and dead-letter moves ([outbox.go](./outbox.go#L8)).
- `ChannelProvider` / `WebhookReceiver`: Omnichannel dispatch and inbound webhook ingestion ([channel_provider.go](./channel_provider.go)).
- `CustomerSalesDecisionPort`: Canonical AI inference port for customer sales decisions and capability tool execution ([customer_sales_contract.go](./customer_sales_contract.go)).
  - Current Code Behavior: In `internal/bootstrap/integrations.go:102`, wires DeepSeek direct client with `standby=nil`.
  - Design Target / Branch Decision (Issue 21): Architecture documents Gemini as primary provider with DeepSeek as fallback on HTTP 503 errors at the transport adapter level without business logic impact.
- `BusinessAIExecutionGuard`: Evaluates merchant AI opt-out policy flags and halts AI processing if disabled ([customer_sales_contract.go](./customer_sales_contract.go)).
- `AIRunRepository`: Manages persistence and state transitions for the 8-stage `AIRunLifecycle` state machine.
- `AIUsageRepository`: Tracks token consumption and quota limits.
- `CatalogAIReadRepository`: Provides read models for AI catalog candidate evaluation and projections.
- `CustomerSalesContextBuilder`: Port for assembling sanitized conversation context and evidence.
- `RealtimePublisher`: Realtime websocket/event broadcasting ([realtime.go](./realtime.go)).
- `BusinessRepository`, `TeamRepository`, `LeadRepository`, `TransactionRepository`, `SubscriptionRepository`: Persistence ports for core enterprise domains. (`SubscriptionRepository` is defined in [subscriptions.go](./subscriptions.go#L122)).

## Invariants
- All port interface methods must take `context.Context` as their first parameter.
- Tenant Scoping: Every tenant-scoped data access method requires explicit `businessID` scoping; system-level infrastructure ports (e.g. outbox leasing, invite token verification, platform administration) operate on globally unique tokens or identifiers.
- Business transactions must execute via `TransactionManager.Within` without exposing SQL transactions to callers.
- No network or HTTP calls inside local database transactions.

## Side Effects
- Declares contracts for operations that mutate PostgreSQL storage, claim worker leases, invoke SocialAPI HTTP endpoints, or call LLM inference APIs.

## Verification
- Run: `go test ./internal/application/ports`

# internal/application

## Overview
Hexagonal application layer orchestrating Mujeeb 24's business use-cases, command execution, query handling, and background processing. Sits between the pure domain models and concrete persistence/network adapters, coordinating data flow while maintaining architectural boundaries.

## Subdirectories & Boundaries
- [ports](./ports/EGM.md) - Inbound and outbound contract interfaces decoupling use-cases from PostgreSQL, SocialAPI, and AI providers.
- [authorization](./authorization/EGM.md) - Role-based access control, OpenAPI operation permission mapping, and conversation security scopes.
- [catalogvalidation](./catalogvalidation/EGM.md) - Dynamic attribute validation rules, data types, and partial variant schema validation.
- [commands](./commands/EGM.md) - CQRS mutation inputs, metadata envelopes, actor contexts, and optimistic concurrency versioning.
- [queries](./queries/EGM.md) - CQRS read queries, cursor-based pagination parameters, and projection view models.
- [services](./services/EGM.md) - Core business services, conversational AI engine (`AutoReplyService`), outbox processor, webhook ingestion, sales workflows, and catalog operations.
- [errors](./errors/EGM.md) - Canonical application error codes, structured error types, and retryability flags.
- [merchantassist](./merchantassist/EGM.md) - Read-only merchant assistant for analytics, dashboard overviews, and conversation insights.
- [merchantcatalogai](./merchantcatalogai/EGM.md) - Two-phase conversational AI authoring for catalog items, variants, and pricing offers.

## Global Invariants
- Strict Hexagonal Architecture: Application services depend solely on domain entities and `ports` interfaces, never on concrete PostgreSQL or HTTP adapter packages.
- Zero network calls inside DB transactions: All external communications (SocialAPI, AI inference, cloud storage) must execute outside database transaction blocks.
- Mandatory Tenant Scoping: All business commands, queries, and persistence operations strictly require and enforce `business_id` isolation via `ScopeProvider`. System infrastructure operations (outbox worker queue leases, invite token verification) operate on globally unique tokens or IDs.
- At-least-once outbox delivery: All outbound communications are staged locally via the outbox store within the initiating transaction.
- Perimeter Security Model (Known Inversion): API permission checks are enforced at the HTTP transport perimeter based on OpenAPI `operationID`. CQRS command handlers do not enforce permissions internally, relying on upstream middleware verification. Defense-in-depth requires handlers to validate `ActorContext.Role` for sensitive mutations.

## Global Side Effects
- Orchestrates transactional updates to PostgreSQL.
- Coordinates worker polling, lease acquisitions, and background outbound transmissions.
- Dispatches AI completions and realtime notifications.

## System Verification
- Run: `go test ./internal/application/...`

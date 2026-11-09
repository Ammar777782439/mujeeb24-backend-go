# internal/domain

## Overview
Root of the domain layer in Mujeeb 24's hexagonal architecture. Contains core business logic, value objects, lifecycle states, and AI contracts completely isolated from databases, HTTP frameworks, and external network services.

Note on Domain Scope: In Mujeeb 24's codebase, `internal/domain` is strictly partitioned into two subsystems: `ai` (AI contracts & prompts) and `channel` (omnichannel communication models & delivery statuses). Core commercial business entities (Customer, Catalog, Offer, Variant, Lead, Transaction, Team) do not reside as rich domain aggregate roots here; they exist as persistence data records in `internal/application/ports/core_repositories.go` and are mutated via application CQRS command handlers and PostgreSQL updates.

## Subdirectories & Boundaries
- [ai](./ai/EGM.md) - AI system contracts, evidence data models, sanitized sales projections, and compiled system prompts.
- [channel](./channel/EGM.md) - Omnichannel social communication domain models, lifecycle states, delivery statuses, and event representations. It also defines provider-neutral attachment references; durable media storage and tenant-scoped attachment persistence remain application/infrastructure concerns.

## Global Invariants
- Zero external imports: No SQL packages (`pgx`, `database/sql`), no HTTP frameworks, and no third-party cloud SDKs are allowed in the domain layer.
- Immutable business rules: State transitions and value constraints are enforced by pure Go functions.
- Domain contracts decouple storage schemas from external provider protocols and AI model representations.

## Global Side Effects
- None (pure in-memory computations and domain entity definitions).

## System Verification
- Run: `go test ./internal/domain/...`

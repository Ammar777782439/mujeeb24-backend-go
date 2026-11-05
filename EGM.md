# Mujeeb 24 Backend Blueprint

## System Overview
Mujeeb 24 Backend is a multi-tenant conversational commerce operating system built with Go 1.22 and PostgreSQL 16. It follows a Modular Monolith Hexagonal Architecture, strictly decoupling domain business logic and application orchestration from external network providers (SocialAPI) and AI inference engines (DeepSeek / Gemini).

- Channel Gateway: SocialAPI is an external unified messaging transport gateway providing webhook delivery and REST dispatch for WhatsApp Cloud API, Instagram Graph API, and Facebook Messenger.
- AI Inference Architecture: Design Target: Dual-provider architecture with Gemini as primary and DeepSeek as fallback on HTTP 503 errors at the transport adapter level. Active Runtime Wiring (`internal/bootstrap/integrations.go:102`): Currently wires DeepSeek directly with `standby=nil` (fallback router is a documented design policy, not yet activated in runtime bootstrap on `main`).

## Subdirectories & System Boundaries
- [internal/domain](./internal/domain/EGM.md) - Domain layer containing AI metadata contracts & prompts (`ai/`) and omnichannel channel models & lifecycle enums (`channel/`). Commercial business entities (Customer, Catalog, Offer, Lead, Transaction, Team) operate as persistence records in `internal/application/ports/` rather than rich domain aggregate roots.
- [internal/application](./internal/application/EGM.md) - Application layer orchestrating CQRS commands, queries, use-case services (including `AutoReplyService`), role authorization, and outbox workers.
- `internal/adapters` - Hexagonal adapters layer (Primary: HTTP handlers, Webhook receivers, OpenAPI routes; Secondary: PostgreSQL repositories via `pgxpool`, SocialAPI HTTP client, AI model clients).
- `internal/bootstrap` - Composition root (ADR-021) wiring concrete adapters, database connection pools, services, and runtime daemons.
- `cmd/` - Executable entrypoints (`cmd/api` for HTTP/REST dashboard and webhook server; `cmd/worker` for asynchronous lease-claiming outbox message dispatcher).

## Global Invariants
- Chatwoot-Free Boundary: Mujeeb / PostgreSQL is the single authoritative source of commercial truth; SocialAPI is strictly an external channel transport. No Chatwoot runtime services or adapters exist (historical migrations `000034`, `000035`, `000039` are retained solely for PostgreSQL schema sequence integrity).
- No Network Calls Inside DB Transactions: All external HTTP requests (SocialAPI, AI models, S3/B2 storage) execute strictly outside database transaction blocks.
- No Blind Retries on Unknown Outcome: Unknown message transmission results immediately isolate to `dead_letter` to prevent duplicate customer messaging. (Known Operational Limitation: No automated retry daemon or dead-letter listing queries currently exist in the codebase; manual database inspection is required for reconciliation).
- Multi-Tenant Scoping: Commercial queries, commands, and persistence operations enforce `business_id` isolation via `ScopeProvider` (`internal/adapters/primary/http/handlers/postgres_scope.go`). System infrastructure operations (outbox worker queue leases, invite token verification) operate on globally unique tokens or IDs.
- Last Owner Protection: A business cannot demote or revoke its last active owner (enforced atomically via PostgreSQL row locking).
- Forward-Only Database Migrations: Schema evolution is strictly append-only across PostgreSQL migrations (baseline `000001` upwards; historical files `000001` through `000052` are immutable).
- Contract Grounded AI: AI models operate exclusively through structured function calls and proposals; they possess zero direct SQL execution or unvalidated message dispatch permissions.
- Worker Lifecycle & Known Risks: Outbox background worker runs with a per-cycle timeout (default 10s). Current Code Behavior: In `cmd/worker/main.go`, cycle processing errors invoke `log.Fatal(err)`, terminating the entire OS process and requiring external supervisor restart (e.g. systemd/k8s). Proposed Improvement: Per-item error isolation without crashing the worker process.

## Global Side Effects
- PostgreSQL database persistence (`pgx/v5` connection pool).
- Background worker lease claims on `outbox_entries` and asynchronous SocialAPI dispatches.
- Inbound webhook processing and deduplication in `inbound_event_ledger`.
- Real-time websocket broadcasts and structured audit logging.

## System Verification
- Run: `go test ./...`
- Repository Verification Gate: `bash scripts/verify-repository.sh`

# Mujeeb 24 — Merchant AI Assist | Contract Implementation Ledger

**Status:** DRAFT — incremental implementation only. **Branch:** `feat/merchant-ai-assist` (forked from backend `main`). No merge to main is authorized by this document.

## Non-negotiable architecture boundaries
1. Preserve closed contracts and existing implementations: `CatalogEntityContract`, `CatalogAIProjection`, `MerchantCatalogAI`, `CustomerSalesContext`, sales workflow authorization/execution, existing provider wiring.
2. Catalog→CatalogItem→AttributeSchema/Definitions→Attributes→Variants→Offers retains exact domain identities, pricing and availability invariants. **Do not create a competing catalog contract in this package.**
3. Merchant AI Assist is a separate dashboard use case. V1 may **suggest/read/explain**, but cannot write, approve/reject orders, send messages or change policies. The existing AI customer-response automation is unaffected.
4. Database rows are never sent directly to models. Every model-bound payload requires a reviewed domain projection, verified tenancy/permissions, allowed task and evidence, and a server-validated response. No generic SQL tool.
5. No prompt-defined permissions, no dynamic selection of business by model, and no exposure of raw auth tokens, provider credentials, phone numbers, payment references, webhook payloads or audit internals.
6. RAG/vector database, agent framework, new provider runtime and a separate microservice are out of scope unless evidence requires them.

## Phases (this file is the authoritative progress ledger)
- [x] **P1A — Foundation (initial code):** versioned `EvidenceEnvelopeV1` with source/authority/completeness/freshness validation; typed task IDs; fail-closed `Registry` for allowlisted projection metadata and tests. Pure Go, no database or HTTP dependencies.
- [x] **P1B-A — Table-source audit (current changes):** inventory 80 migration-created tables from `000001..000081`, including 3 removed in `000053`; categorize 33 projection candidates, 33 internal-only, 11 prohibited, 3 retired. New migrations with unknown `CREATE TABLE` entries fail the source-inventory test. See `14-merchant-assist-source-inventory-ar.md`.
- [ ] **P1B-B — Column/final-schema audit (NOT DONE):** apply all migrations on PostgreSQL test DB and reconcile `schema.sql`, all fields/types/constraints/ALTER/DROP, permissions, tenant isolation, PII and field-level allowlists. Candidates cannot be exported to AI before this gate passes.
- [ ] **P2 — Canonical domain projections:** domain source contracts for business, conversation, customer, leads, sales workflow, transaction, policies, channels, automation, subscriptions. Reuse existing catalog projection; no new enums inferred from UI. Add validators/allowlists and cross-reference integrity tests one domain at a time.
- [ ] **P3 — Authorized read facades and evidence:** tenant-scoped reads, domain permissions, freshness rules, version/change checks, deterministic safe output. No model calls during a PostgreSQL transaction.
- [ ] **P4 — Merchant AI Assist application:** context budget/assembly, limited read-tool registry, typed task/output contracts, result and provenance validation, cost and timeout checks, prompt version.
- [ ] **P5 — HTTP/API/provider transport:** Huma DTO/OpenAPI generated, model-adapter tests against actual configured DeepSeek/API gateway model, syntax and schema checks, 401/403/429/503/timeout fail-safe behavior. No new hardcoded provider/model.
- [ ] **P6 — Frontend integrations and acceptance:** user-controlled proposals within product, inbox, leads, workflows etc, accessibility/mobile/RTL, typed generated API, no automatic mutations, Postgres integration, security, cross-business, regression and production gate.

P1B-A is source-inventory work only. P1B-B and phases P2-P6 are NOT implemented or verified merely because listed here.

## P1A contract: `internal/domain/ai/contracts`
`EvidenceEnvelopeV1` fields:
- `schema_version` fixed at 1; `resource_ref` opaque, never constructed by model.
- `source_type` denotes domain source, `authority` one of `merchant_record|merchant_verified|system_record|customer_statement|published_policy|derived`.
- `completeness` one of `complete|partial|missing|stale`; `retrieved_at`, optional `valid_until`, optional `resource_version`.
- `Validate(now)` rejects missing/unknown mandatory metadata and expired evidence mislabeled complete. Expiration absent does not mean everlasting truth: domain-specific checks required.
- `UsableAsCurrentFact(now)` is a conservative gate only; domain-specific price/stock/policy checks are *additional and required*.

`ProjectionDescriptor` fields: `ID`, `Version`, `SourceEntities`, `AllowedFields`, `RequiredPermission`, `AllowedTasks`, `ReadFacade`, `MaxRows`, `MaxBytes`, `MaxAge`, `RedactionPolicy`, `Owner`.

`Registry` is immutable after construction; no defaults and no dynamic table discovery. A successful `CheckSelection` checks **descriptor metadata only**. It does not authorize a principal or prove tenant ownership. Those checks belong in the application read facades and must be implemented before this is wired to a provider.

**Current state:** No production projection registered or model-facing runtime connected yet. Test descriptors in `registry_test.go` do not grant model access to catalog data.

## Future P2 review checklist for every source
- Existing Go domain/port type and current migration/SQL constraints verified.
- Exact allowlist of fields and forbidden fields; enum and value semantics.
- Tenant scope and domain permission; appropriate read facade and upper limits.
- Required relationships, valid snapshot/revision, provenance and freshness.
- `unknown` vs `stale` vs `missing` vs `false` preserved.
- Decimals (not float64 money), RFC3339 timestamps, dynamic schema keys resolved with their definitions.
- Tests: valid, null, missing, unauthorized, cross-business, stale, oversized, injected instructions.
- Source descriptor/contract version updated; OpenAPI and docs generated/checked when exposed.

## Verification required before advancing
`gofmt -l internal/domain/ai/contracts/*.go` must be empty.
`go test ./internal/domain/ai/contracts`
`go test ./...`
`go vet ./...` and existing `scripts/verify-repository.sh` on CI where available.
A passing suite does NOT prove P1B-P6 complete.

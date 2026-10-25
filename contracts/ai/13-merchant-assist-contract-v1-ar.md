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
- [x] **P1B-B1 — PostgreSQL 16 structural audit:** applied all 81 migrations via the canonical `database.RunMigrations` on an isolated CI database; captured 77 active tables plus `schema_migrations`, 978 typed columns, 676 constraints and 25 standalone unique indexes. Committed canonical `testdata/merchant_assist_schema_pg16.json` with drift comparison wired into the existing integration verification script. Documented all 425 columns belonging to 33 projection-candidate sources, their actual PostgreSQL types, nullability, defaults and constraints in `15-merchant-assist-postgres-schema-audit-ar.md`. A structural snapshot is **not** a model allowlist.
- [ ] **P1B-B2 — Field-level domain/security review (IN PROGRESS):** reviewed typed projection prototypes against current Go ports and migrated PostgreSQL for business (`16-...`), published policy/knowledge, customers, conversations/messages/summaries/replies, leads and core transaction/workflow surfaces (`17-merchant-assist-knowledge-inbox-sales-fields-review-ar.md`). Added SQL type, exact DTO JSON field-set, enum and invalid-state tests. **Not complete:** dynamic JSONB schemas (policy rules, customer profile, lead qualification, transaction snapshots, sales proposals/requirements), domain-by-domain PII redaction and authorized read facades, plus remaining source candidates. No runtime reader, tool, API or provider exposure has been approved.
- [ ] **P2 — Canonical domain projections:** domain source contracts for business, conversation, customer, leads, sales workflow, transaction, policies, channels, automation, subscriptions. Reuse existing catalog projection; no new enums inferred from UI. Add validators/allowlists and cross-reference integrity tests one domain at a time.
- [ ] **P3 — Authorized read facades and evidence (IN PROGRESS):** P3-A published policy/knowledge (`18-...`) and P3-B customer/conversation/lead/transaction header reads (`19-merchant-assist-record-reader-ar.md`) now use existing tenant-scoped ports with per-domain permission and live BusinessAIGuard, output DTO validation, bounded size, source version evidence, and denial/cross-tenant tests. **NOT DONE:** model-safe PII scrubbing, detailed text/JSONB and message timeline reads, full runtime registry binding, HTTP/provider integration, authenticated handler integration and full Postgres read tests. None of these new readers is available to an LLM or frontend.
- [ ] **P4 — Merchant AI Assist application:** context budget/assembly, limited read-tool registry, typed task/output contracts, result and provenance validation, cost and timeout checks, prompt version.
- [ ] **P5 — HTTP/API/provider transport:** Huma DTO/OpenAPI generated, model-adapter tests against actual configured DeepSeek/API gateway model, syntax and schema checks, 401/403/429/503/timeout fail-safe behavior. No new hardcoded provider/model.
- [ ] **P6 — Frontend integrations and acceptance:** user-controlled proposals within product, inbox, leads, workflows etc, accessibility/mobile/RTL, typed generated API, no automatic mutations, Postgres integration, security, cross-business, regression and production gate.

P1B-A and P1B-B1 are verified source and structural database audits only. P1B-B2/P2 and the remaining P3-P6 steps are not complete merely because listed here. P3-A is internal only and cannot supply a model before independent PII/tenant/integration gates.

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

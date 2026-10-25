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
- [ ] **P1B-B2 — Field-level domain/security review (IN PROGRESS):** reviewed SQL-backed DTOs for business, published policies/knowledge, customers, conversations, messages and summary, leads/attributions/scores, core transactions and workflows. Qualified lead JSONB has a strict reference-array parser and emits only count plus `unverified_references`; other dynamic JSONB and broad PII remain blocked. See `16-...`, `17-...`, `20-...`, `21-merchant-assist-integrated-runtime-ar.md`.
- [ ] **P2 — Canonical domain projections:** domain source contracts for business, conversation, customer, leads, sales workflow, transaction, policies, channels, automation, subscriptions. Reuse existing catalog projection; no new enums inferred from UI. Add validators/allowlists and cross-reference integrity tests one domain at a time.
- [ ] **P3 — Authorized readers (IMPLEMENTED FOR EIGHT SCOPED PROJECTIONS; NOT CLOSED):** tenant/permission and BusinessAIGuard checked for policies, knowledge, customer, conversation, history, lead, historical lead evidence, and transaction. Bounded evidence, checkpoint revalidation, PostgreSQL tenant isolation integration and a conservative content sanitizer exist. It is **not a comprehensive DLP filter**; race/snapshot and privacy review remain open. See `18-...` through `21-...`.
- [ ] **P4 — Merchant AI Assist application (INITIAL RUNTIME IMPLEMENTED; NOT CLOSED):** centralized `NewMerchantAssistRegistry()` binds eight read-only projections to `Assistant.Assist`, with bounded evidence and prompt construction. Model responses are review-only. Still pending: proven provider-aware cost/usage metering, answer-fact checks, prompt version metrics and end-to-end resilience testing.
- [ ] **P5 — HTTP/API/provider transport (IMPLEMENTED, LIVE TEST PENDING):** Huma `merchantAIAssist` endpoint with generated OpenAPI, authenticated operation permission and existing native completion router alias `mujeeb-merchant`. No live provider smoke or 429/503/timeout E2E verified; must validate model configuration and usage protections on a nonproduction environment before deployment.
- [ ] **P6 — Frontend integrations and acceptance (DRAFT IMPLEMENTATION; CI NOT GREEN):** companion frontend branch `feat/merchant-ai-assist-ui` has a permission-aware RTL dashboard sheet and generated TypeScript against backend feature contract. Frontend CI failed and its latest logs were inaccessible; user UX, mobile/a11y/browser and cross-business E2E checks remain pending. This is not an approval to merge either branch.

P1B-A and P1B-B1 are verified inventories/schema audits. P3-P5 now have an **initial integrated runtime on the feature branch**, but runtime presence does not prove model accuracy, comprehensive PII safety, live provider readiness or production suitability. P6 exists only on the separate frontend draft branch with failing CI. Do not merge or deploy before all blockers recorded in `21-merchant-assist-integrated-runtime-ar.md` are closed.

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

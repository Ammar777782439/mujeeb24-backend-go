# Mujeeb 24 — Native Gemini / DeepSeek migration

Scope: branch `feat/gemini-deepseek` branched from Atomic.
This is an **implementation log**, not a declaration of production readiness.

## Invariants

- Customer Sales, Catalog Batch, Final Catalog and Merchant AI continue using the existing canonical business contracts.
- Gemini is the primary model provider; DeepSeek is standby for operational unavailability (initial trigger: HTTP 503).
- Only inference generation attempts may switch providers. Retry and fallback must never execute a commercial tool or mutate a transaction.
- No response that fails canonical schema, capability selection, referential integrity or catalog coverage may be accepted.
- Previously completed catalog batches must not be rerun as a side effect of provider failure.
- Provider-specific HTTP schemas are transport concerns. No provider detection in the application business logic.
- Do not delete deployed migration history or cost records. Remove old LiteLLM runtime code, routing and documentation only after every caller has a native replacement.

## Staged delivery

1. Native fallback policy and failure boundary, with fake-provider tests. **Not yet wired to production.**
2. Direct Gemini transport (structured outputs and function selection) and transport-contract tests.
3. Direct DeepSeek transport (forced functions and structured parsing) and transport-contract tests.
4. Connect existing sales, catalog, merchant and token/usage ports to native providers with a single consistent contract.
5. Remove obsolete LiteLLM clients, configuration/UI contracts and deployment scripts; generate OpenAPI and update migration-backed metadata safely.
6. Full automated acceptance including Go, DB, OpenAPI and provider integration. Live model qualification is separate and cannot be inferred from HTTP mocks.

Operational breaker settings belong to infrastructure/configuration and must not be hard-coded into business rules.

## Verified implementation checkpoints — 17 October 2026

All changes below are **on the feature branch only**. GitHub's `Native AI Contracts` job compiles and tests the repository and runs `go vet`; HTTP tests use local fake servers, **not real Gemini/DeepSeek credentials**.

- Native `Router`: defaults to Gemini; only an HTTP 503 triggers DeepSeek fallback. A bounded per-process breaker throttles repeated Gemini failures. No merchant tool, sale, payment or outbound customer message is executed by this router.
- Native `Gemini` direct REST adapter: separate system/user context, mandatory `ANY` function selection for customer capabilities, JSON Schema output for catalog batches; fail-closed on incomplete model answers.
- Native `DeepSeek` direct `/beta/chat/completions` adapter: non-thinking forced tools, strict schemas, reversible tagged representation of nullable fields; Go restores and validates the canonical JSON on return. Unrepresentable schema keywords are rejected before provider dispatch.
- Native `CompletionClient`: carries per-call provider, model, token metadata and the actual attempt trace without storing mutable metadata shared by concurrent conversations.
- `NativeCustomerSalesAdapter`: preserves existing Customer Sales prompts, capability names and validation, with model-only 503 fallback. Tests cover both Gemini success and DeepSeek fallback.
- `NativeCatalogAdapter`: preserves catalog projection, batch coverage controller, strict candidate decoding, final terminal capability evaluation. Uses conservative serialized-byte count plus protocol reserve for admission budgeting; it is **not** an exact provider tokenizer. Both providers' minimum verified context capacity must still be configured before production.
- Strict validation and ref checks do not prove live provider behavior, pricing, or actual uptime. A 503 simulation only qualifies code behavior under the mocked response.

## Pending blockers before replacing LiteLLM in runtime

1. Implement and contract-test Native Merchant AI's read-only tool loop and canonical structured final proposal without dropping any required attributes.
2. Wire Native adapters in `internal/bootstrap` with explicit secure Gemini and DeepSeek configuration; reject missing credentials and unknown model versions, and remove Gateway-Only runtime flags only after every caller is migrated.
3. Replace gateway usage and cost metadata with exact native provider accounting, including failed attempts, and preserve historical financial data through a new migration.
4. Remove obsolete LiteLLM clients, Docker/Render configuration, scripts, handlers, documentation and disabled code paths. Keep applied migrations immutable.
5. Regenerate OpenAPI and frontend types for the final Admin AI configuration contract. Run full PostgreSQL, race, security and provider-compatibility acceptance tests before enabling production.

**Current deployment status:** not enabled; old LiteLLM runtime wiring remains in place until the migration is complete. Do not mistake a passing CI build for production readiness.

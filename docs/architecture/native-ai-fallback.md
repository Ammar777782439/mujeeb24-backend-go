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

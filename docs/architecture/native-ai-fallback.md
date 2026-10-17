# Mujeeb 24 — Native Gemini / DeepSeek migration

Scope: branch `feat/gemini-deepseek` branched from Atomic.
This is an **implementation log**, not a declaration of production readiness.

## Invariants

- Customer Sales, Catalog Batch, Final Catalog and Merchant AI continue using the existing canonical business contracts.
- Gemini is the required primary provider. DeepSeek is an optional standby for HTTP 503 when a DeepSeek API key is provided.
- Only inference generation attempts may switch providers. Retry and fallback must never execute a commercial tool or mutate a transaction.
- No response that fails canonical schema, capability selection, referential integrity or catalog coverage may be accepted.
- Previously completed catalog batches must not be rerun as a side effect of provider failure.
- Provider-specific HTTP schemas are transport concerns. No provider detection in the application business logic.
- Do not delete deployed migration history or cost records. The old proxy runtime has been retired from Go and its deployment files removed.

## Staged delivery

1. Native fallback policy and failure boundary, with fake-provider tests. **Not yet wired to production.**
2. Direct Gemini transport (structured outputs and function selection) and transport-contract tests.
3. Direct DeepSeek transport (forced functions and structured parsing) and transport-contract tests.
4. Connect existing sales, catalog, merchant and token/usage ports to native providers with a single consistent contract.
5. Remove obsolete proxy clients, configuration/UI contracts and deployment scripts; generate OpenAPI and update migration-backed metadata safely.
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

## Current verified status and production blockers

- Native Customer Sales, full catalog batch and final decision, and read-only Merchant AI adapters are connected in bootstrap. CI validates the branch HEAD on every code update; specific passing runs must match the final commit.
- The previous proxy runtime and files were removed. Deployment requires `NATIVE_AI_ENABLED=true` and `GEMINI_API_KEY`. `DEEPSEEK_API_KEY` is optional. Automatic deployment remains disabled.
- The historical SQL migration version 75 is retained byte-for-byte in its DDL, with a provider-neutral filename. The migration version and columns are not removed.
- HTTP 503 provider fallback is tested against mock HTTP servers. It does not prove real provider compatibility.
- Native catalog admission currently uses a conservative serialized-byte bound, not verified tokenization for both models. Context capacity must be qualified per deployed model.
- The provider-direct usage values report tokens, but there is **no independently validated actual USD price for every inference attempt or failed primary request**. Unknown spend must not be treated as zero. Pricing-version-based provider accounting and budget-enforcement acceptance remain incomplete.
- Merchant AI uses read-only discovery calls first, with parallel function calls explicitly disabled. The final commercial proposal is requested in an independent strict `json_schema` transport envelope with exactly one required `canonical_proposal_json` string field.
- This envelope preserves arbitrary merchant-defined attributes instead of silently stripping dynamic fields. Go strictly parses and validates the *entire* nested canonical proposal, including authorization and reference checks before execution. The envelope **does not mean** the provider itself enforces the full dynamic merchant JSON Schema; actual model reliability must be separately qualified.
- Provider-specific strict mode and response framing for both models are not claimed live-qualified until verified on the selected account and model versions.
- Regenerate OpenAPI from Go with `go run ./cmd/openapi-gen` and check the generated contract. Do not edit generated YAML manually.
- Run PostgreSQL integration, race, secret, full Go and live provider contract qualification before authorizing production. No actual model credentials were used in these mock tests.

**Runtime status:** Native source cutover is complete; production qualification and model/cost accounting are **not** complete. Do not declare production readiness merely from green unit tests.

## Optional DeepSeek standby — Gemini-only configuration

- Leave `DEEPSEEK_API_KEY` absent/unset to start the native AI runtime with Gemini alone. Set `NATIVE_AI_ENABLED=true` and a valid `GEMINI_API_KEY`.
- Bootstrap never constructs a DeepSeek client when its API key is absent. The router always invokes Gemini, including subsequent requests after 503, without opening a breaker that could suppress the only provider.
- On Gemini 503 without a standby, the inference fails with an explicit operational error. No fabricated response, silent provider substitution, business tool execution or repeated transaction is permitted.
- Once the DeepSeek key is added, restarting the API configures the existing HTTP 503 fallback and bounded breaker. Nothing in Customer Sales, Universal Catalog or Merchant AI business contracts changes.
- Platform Admin shows Gemini as active and does not list an unconfigured DeepSeek credential or model. Live model qualification remains required before production.

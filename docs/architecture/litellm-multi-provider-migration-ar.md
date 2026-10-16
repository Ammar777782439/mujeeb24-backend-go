# Mujeeb 24 — LiteLLM Multi-Provider Gateway

**Branch:** `feat/ai-litellm-multi-provider-gateway`  
**Baseline:** `feat/customer-sales-workflow-interactive` @ `f7662205fd9c42dcd56f6d998e79b2ecb288fa2f`  
**State (2026-10-14):** LiteLLM-only Go integration, with production-acceptance work outstanding. **Do not merge or deploy without the owner's explicit authorization.**

## Current implementation (authoritative)

- **Transport:** `internal/adapters/secondary/ai/litellm/`. All live B2C, catalog and B2B generation requests use LiteLLM `POST /v1/chat/completions` (the old Interactions wire endpoint is retired from these paths). The shared B2C/catalog bridge converts tools to OpenAI-compatible function declarations and normalizes strict results into Mujeeb's existing capability contract. The gateway owns model selection, retries and fallback; Mujeeb does **not** retry against specific providers in Go.
- **B2C decision and catalog evaluation:** `internal/adapters/secondary/ai/gateway/` and `internal/application/services/`. The AI returns a strictly validated Capability; Go validates and executes the authorized action.
- **Customer conversation continuity:** **stateless at the provider**. Each call receives Mujeeb's canonical conversation context and summary. `previous_interaction_id` is not used or persisted by the live AI path.
- **Universal Catalog:** `CatalogBatchController` retains full pagination, schemas and definitions, variants, offers, evidence coverage, revision checks, candidate reduction and final validation. The `CatalogEvaluationClient` and token estimator interfaces are provider-neutral.
- **Token budgets:** LiteLLM's counting utility may estimate tokens for some deployments. The controller counts the assembled request, applies configured context/output bounds and rejects oversize requests; it must not claim exact tokenization across providers.
- **B2B merchant authoring:** tool calls and replies are replayed as explicit messages, scoped to merchant/session and read-only tools. Proposal normalization, validation and approval remain in Go.
- **Administration:** Platform Admin reads LiteLLM route configuration and gateway health. Legacy Google credential mutation/activation is retired, but public endpoint shapes are preserved. The runtime kill switch remains authoritative.
- **Accounting:** use native LiteLLM spend in USD with gateway call/deployment metadata, and explicit `LITELLM_USD_TO_YER_RATE` for Mujeeb budget reports. Customer-facing AI reply entitlements are tracked separately from internal model consumption.
- **Timeout:** `LITELLM_HTTP_TIMEOUT` bounds each model HTTP request. The AutoReply worker no longer overrides that deadline; AI Run failure finalization uses its own short DB context.

## Required runtime configuration

```dotenv
LITELLM_GATEWAY_ONLY=true
LITELLM_BASE_URL=http://127.0.0.1:4000
LITELLM_API_KEY=<virtual-key>
LITELLM_CUSTOMER_MODEL=mujeeb-customer
LITELLM_CATALOG_MODEL=mujeeb-catalog
LITELLM_MERCHANT_MODEL=mujeeb-merchant
LITELLM_HTTP_TIMEOUT=200s
LITELLM_MAX_CONTEXT_TOKENS=32768
LITELLM_MAX_OUTPUT_TOKENS=4096
LITELLM_MAX_INPUT_CHARACTERS=48000
LITELLM_MERCHANT_MAX_REQUEST_BYTES=524288
LITELLM_USD_TO_YER_RATE=<approved-positive-accounting-rate>
```

These are **illustrative deployment values**, not hard-coded production defaults. Never commit an actual API key or invent an accounting FX rate. See `deploy/litellm/README-ar.md` for local operations.

## Non-negotiable business invariants

1. The selected model understands customer intent and chooses **only** an offered capability. Never add Go keyword routing, a secondary intent engine, or a hard-coded catalog search.
2. Mujeeb alone performs tenant-scoped validation, pricing and offer checks, authorization, approval, idempotency, audit, and execution. Neither LiteLLM nor its upstream models may mutate business state.
3. Universal Catalog coverage is complete: `Catalog → Item → AttributeSchema/Definitions → Attributes → Variants → Offers`. Every page/batch must complete, evidence and catalog revisions must remain consistent, and failures are fail-closed.
4. Leads, transaction drafts, updates and interactive customer confirmation respect canonical state, current buttons, identity, policy and resource versions.
5. B2B authoring preserves session isolation, read-tool authorization, call-result pairing and proposal approval.
6. A retry or fallback may incur multiple upstream charges, but one successful customer reply consumes at most **one** merchant reply entitlement.
7. Canonical conversation messages, ownership, human handoff, AI run states and historical records remain in Mujeeb's database, not a provider's transient interaction ID.
8. Alternate models are subject to **identical** function, schema, reference and policy validation; never silently weaken them.

## Historical compatibility — preserve, do not reactivate

- Earlier migrations and stored fields named `last_gemini_interaction_id`, `ai_gemini_interactions`, and failure stage `gemini_request` are **historical database contracts**. Their names may remain for stored-data/DTO compatibility; live B2C requests do not read or write Gemini continuation IDs.
- Do **not** edit or remove applied database migrations to cosmetically rename fields. Any future removal needs an inventory of existing rows and a separate backward-compatible data migration.
- Earlier migration stages, flags such as `LITELLM_FALLBACK_ENABLED`, `LITELLM_CATALOG_ENABLED` and `LITELLM_MERCHANT_ENABLED`, and the old `internal/adapters/secondary/ai/gemini/` implementation are **retired**. For their original design rationale, consult Git history rather than trying to run those obsolete instructions.

## Remaining production acceptance

- Reconcile **all billable retry/fallback attempts** with LiteLLM-native spend logs, without implementing token price formulas in Go.
- Prove real model compatibility for B2C tool calls, full-catalog batch/final responses, and B2B tool/JSON schemas, including 429/503 and oversized contexts.
- Add durable AutoReply processing recovery when the worker process or bounded queue fails; preserve business-effect idempotency.
- Verify private/TLS gateway connectivity, logging redaction, gateway image version pinning and provider retention controls.
- Complete CI, PostgreSQL integration, race tests and merchant-specific canary checks before enabling customer-channel traffic in production.

**Permanent branch rule:** Commit only to `feat/ai-litellm-multi-provider-gateway`. Do not merge into its source branch, `main`, or another branch without a new explicit owner instruction.

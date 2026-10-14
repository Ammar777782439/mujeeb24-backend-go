## Stage 3.6 — بدء تنظيف مسار Gemini المباشر (2026-10-14)

- **اعتماد Gateway في Bootstrap:** أزيل إنشاء عميل Gemini المباشر وفروع التبديل القديمة من `BuildExternalAdapters` و`BuildAPI`. كل من B2C، الملخص، تقييم Universal Catalog، وتأليف كتالوج التاجر يستخدم فقط مهايئات LiteLLM عند تمكين التشغيل؛ لا يوجد fallback داخل Go إلى Gemini.
- **حذف تشغيلي فعلي:** أزيلت متغيرات `LLM_ENABLED`، `LLM_BASE_URL`، `LLM_API_KEY`، `LLM_MODEL` ونظائرها من ProcessConfig وملفات البيئة النموذجية، وكذلك مفاتيح Gemini التشغيلية غير المستخدمة من ProcessConfig. أزيل كذلك `cmd/test-autoreply/main.go` الذي كان يتصل بـGoogle Gemini مباشرة. أي إعدادات LLM قديمة بقيت في ملف `.env` محلي لا تُستخدم في اختيار المسار.
- **عزل إدارة المنصة:** لا تزال واجهات Platform Admin لإدارة موفري الذكاء واستكشاف الموديلات وبعض مخزون `AIConfigurationCache.LoadFromEnv(GEMINI_*)` مرتبطة بعقد Google القديم. **لم تُحذف هذه الواجهات أو بياناتها** لحين توفير عقد بوابة حديث وتحديث DTO/OpenAPI/الفرونت بأمان.
- **لا تحذف بعد:** بعض الملفات الموجودة في `internal/adapters/secondary/ai/gemini` هي عقود مساعدة ومخططات JSON وقوالب Prompts مشتركة مع LiteLLM، وبعضها مهايئات Google غير مستخدمة في Bootstrap لكن اختباراتُها لم تُستبدل بالكامل. حذف الحزمة بالجملة يكسر LiteLLM؛ يجب فصل المشترك أولاً ثم حذف الملفات الميتة المثبتة.
- **موانع اعتماد الإنتاج:** تسعير `pricing_failed`، تكاليف الاستدعاءات غير الناجحة، التوافق الفعلي لجميع موديلات B2B والمزود البديل، استعادة المهام، ضبط مهلات الطلبات، وأمان اتصال البوابة؛ هذه ليست مسائل اختفت بحذف العميل القديم.
- يحتفظ Gateway نفسه بتعدد المزودين وFallback، بينما تبقى هذه الـPR **Draft** والفرع معزولًا، بلا دمج ولا نشر تلقائي.

## Stage 3.5 — Gateway-only runtime and local operation (2026-10-13)

- A single backend switch `LITELLM_GATEWAY_ONLY=true` makes Mujeeb use LiteLLM for B2C/customer-sales, conversation summaries, full catalog batch+final, and B2B merchant catalog authoring. It does not require `GEMINI_API_KEY`; the optional direct Gemini client is not instantiated in this mode.
- Removed the obsolete Go `FailoverCustomerSalesDecisionPort` and the three legacy `LITELLM_*_ENABLED` toggles. **Model fallback now belongs only in LiteLLM**. Keep the remaining old Gemini code until it can be removed with successful live provider acceptance; gateway-only operation bypasses it.
- Added a local LiteLLM+Postgres Compose stack, `deploy/litellm/.env.example`, operator instructions at `deploy/litellm/README-ar.md`, and gateway parameters in root and local backend `.env.example`. Secrets are local and excluded by Git.
- Runtime configuration check and bootstrap tests cover no-Gemini operation. All real customer-channel actions must remain disabled until live route/schema/capability tests and spending/reporting fixes are complete.
- **Known gap:** Platform Admin AI model/credential configuration and live model discovery still contain Google-centric code. Pricing ledger and B2B model usage, retry-level costs, durable message recovery and gateway privacy/security hardening remain pending. This is NOT a final removal of all old provider code.
- Never merge this branch. See top-level AGENTS.md for the owner's persistent no-merge requirement.

## Permanent project decisions — 2026-10-13

1. **NO MERGE:** Do not merge `feat/ai-litellm-multi-provider-gateway` into the source branch, `main`, or any other branch even when ALL migration phases are complete. No auto-merge and no PR merge without a fresh explicit instruction from the owner. Continue commits **on this isolated branch only**.
2. **AFTER COMPLETION:** Once the full LiteLLM-only architecture passes B2C, full Universal Catalog, B2B, billing, runtime recovery and live model acceptance checks, **remove the obsolete direct-provider/fallback code, unused adapters, duplicate config, stale docs and dead tests**, then retain the new unified LiteLLM gateway. This cleanup must happen **on this same feature branch**, not as a merge. Do not delete old code before all equivalents work.
3. **Keep LiteLLM's own multi-provider failover:** "Remove old fallbacks" means removing obsolete Mujeeb Go fallback paths and dual integrations, **NOT** removing the desired multi-provider failover configured in LiteLLM.
4. The owner explicitly requires these rules to remain in the repository as handoff/continuation constraints.

# Mujeeb 24 — LiteLLM multi-provider migration

**Branch:** `feat/ai-litellm-multi-provider-gateway`  
**Source:** `feat/customer-sales-workflow-interactive` @ `f7662205fd9c42dcd56f6d998e79b2ecb288fa2f`  
**Status:** In progress / **NOT production ready** / no merge into source or `main`.

## Non-negotiable invariants

1. The LLM understands customer intent and selects an offered **Capability**; Go does not grow an intent engine, keyword router, sector-specific mapping, or hard-coded catalog search.
2. Mujeeb alone validates, authorizes, audits, and executes operations. LiteLLM does not execute sales or merchant-catalog tools.
3. The **Universal Catalog** retains its full projection: Catalog -> Item -> AttributeSchema/Definitions -> Attributes -> Variants -> Offers. Schema-bound values must be paired with their schema definitions. Full-catalog evaluation must read **all** pages/batches and validate snapshot revision before final selection.
4. B2C lead / transaction draft / update / confirmation remain governed by canonical execution, idempotency, approval rules, customer identity, versions, prices, and offers. Confirmation requires the current actionable record/buttons rather than guessing from unbound text.
5. B2B merchant authoring retains read tools, conversation/tool-result history, structured proposals, authorization, and approval/execution boundaries.
6. Merchant subscription **AI replies** and platform **model consumption** remain separate metrics. Failed primary calls and retries can still incur a cost; count actual upstream billable attempts, and charge at most one final reply per successful customer response.
7. Conversation messages, source references, channel isolation, AI runs, historical records, and human handoff must not be lost during provider change or retry. No cross-tenant context.
8. Model output from a fallback must pass the **same** strict schemas, evidence checks, capability allowlist and policy validators. Do not silently repair unsupported actions into executable ones.

## Implemented: Stage 1 (isolated and opt-in)

- `internal/adapters/secondary/ai/litellm/client.go`: transport for LiteLLM `/v1beta/interactions`, bearer authentication, bounded response, sanitized errors, no nested retry.
- `internal/adapters/secondary/ai/gemini/litellm_customer_adapter.go`: strict B2C adapter reusing existing capabilities, tool descriptions, and parser. Accepts LiteLLM `outputs[]` / native `steps[]` with fail-closed decoding. No business-side effects.
- `internal/application/services/customer_sales_failover.go`: switches decisions only on transient provider errors (e.g. 429 / 503), never on invalid schema, 4xx authorization, or canceled calls.
- Bootstrap `LITELLM_FALLBACK_ENABLED` is **false by default**, uses `LITELLM_BASE_URL`, `LITELLM_API_KEY`, `LITELLM_CUSTOMER_MODEL`, timeout and input/output guards when enabled.
- B2C and summary usage records preserve `Usage.Provider` (legacy fallback: `google_gemini`; new gateway route: `litellm`). No token prices or secrets hard-coded.
- Added unit tests for HTTP transport, routing and strict customer capability outputs.

**Important:** Stage 1 only concerns *customer decision* fallback. Even with the flag on, full catalog evaluation and merchant authoring still use Gemini. Do not enable the feature on production until the remaining stages and cost controls are complete.

## Remaining work (must not be forgotten)

### Stage 2 — Full catalog correctness (implemented behind explicit opt-in; production acceptance pending)
- Implemented `LiteLLMCatalogAdapter` for batch and final evaluation, reusing the existing Gemini prompt/schema/capability builders and the existing complete `CatalogBatchController` paging, coverage and revision checks.
- Added `/utils/token_counter` with conservative safety allowance; **not an exact tokenizer** for every provider and still requires live route/model fit verification. Exact counting must not be claimed. The controller can split oversized pages; send fails closed when the model-context cap is exceeded.
- Budget **every actual target-model request** correctly, not by assuming Gemini `countTokens` is universally exact; enforce context limits with fail-closed margin when estimates are used.
- Preserve all candidate references, complete coverage, revision consistency, and evidence; fallback for a batch must never skip or silently narrow the catalog.
- Add tests for multipage catalogs, schema definitions, variants/offers, batch failure, catalog revision changes, and model context limits.

### Stage 3 — B2B merchant authoring (implementation added; gateway capability and price tests pending)
- Added `LiteLLMMerchantCatalogAuthoringAdapter` behind **`LITELLM_MERCHANT_ENABLED=false` by default**. It uses the unified gateway's `/v1/chat/completions` endpoint for **stateless multi-turn read tool exchanges**, rather than assuming Google's `previous_interaction_id` can be shared between providers. This is still **one LiteLLM provider integration**, not a model-specific Go integration.
- Kept the exact merchant authoring system prompt and contextual JSON shared with the original Gemini adapter; use the same canonical `merchantCatalogProposalSchema()`, strict proposal parser, `Normalize()`, `Validate()`, and scoped read-only `MerchantCatalogDiscoveryPort`.
- Preserve tool-call ID/result pairing and the full assistant/tool transcript per request; reject unauthorized tools, malformed arguments, parallel call budget breaches, mixed tool/text messages, invalid proposals and oversized contexts. Cap each round and the number of tool calls to avoid unbounded API costs.
- Mocked tests cover tool execution and replay, business/principal/session scoping, invalid proposals, unauthorized read tools, and request size. No live provider claim yet.
- **Pending:** verify each selected LiteLLM model supports *both* JSON Schema and tools in this endpoint; test real proposal mutations (item attributes, variants/offers, update/delete), ensure model access/bytes fit context; wire all B2B model attempts into platform AI usage and spending caps. Maintain merchant approval/execution as the current Go service.

### Stage 4 — AI telemetry and pricing
- Store the **actual upstream provider and model per attempt**, not just a LiteLLM alias; price each attempt using immutable versioned prices and model usage, including unsuccessful attempts where billed.
- Audit active subscription entitlement, separate summary calls, catalog batch/final calls, B2B calls, spending caps, fallback chains, and retry accounting.
- Do not treat `pricing_failed` as free use or suppress a final reply entitlement.

### Stage 5 — Reliable operations
- Make AutoReply task recovery durable (the existing worker pool is memory-backed); restore after process death and overflow without duplicate sales effects.
- Review time budgets jointly between Gemini, LiteLLM and worker deadlines. One layer owns retries; temporary service failures must not cause request storms.
- Secure gateway transport (private network / TLS), redact prompts and API keys from logs, pin/audit the LiteLLM image, and validate upstream data retention settings.
- Confirm platform kill switch, subscriptions, provider configuration/cache, and reporting remain correct with multiple providers.

### Stage 6 — Release
- Unit + PostgreSQL integration + race/build/vet/gofmt tests; mocked provider 503/429/400, delayed responses, invalid/multiple tools, and exact handoff/transaction idempotency.
- Run **actual provider capability contracts** on Gemini and selected fallbacks (not merely a successful simple chat request).
- Shadow/canary by business, evidence of all paths working, reviewed pricing and rollback. Then and only then enable on production.
- No changes to backend main and no automatic merge. Frontend DTO/OpenAPI updates only if a new platform-facing contract is necessary.

## Operator settings for Stage 1 (development/test only)

```env
LITELLM_FALLBACK_ENABLED=false
LITELLM_BASE_URL=http://127.0.0.1:4000
LITELLM_API_KEY=<gateway-virtual-key>
LITELLM_CUSTOMER_MODEL=mujeeb-customer
LITELLM_CATALOG_ENABLED=false
LITELLM_CATALOG_MODEL=mujeeb-catalog
LITELLM_MERCHANT_ENABLED=false
LITELLM_MERCHANT_MODEL=mujeeb-merchant
LITELLM_MERCHANT_MAX_REQUEST_BYTES=524288
LITELLM_MAX_CONTEXT_TOKENS=32768
LITELLM_HTTP_TIMEOUT=20s
LITELLM_MAX_OUTPUT_TOKENS=4096
LITELLM_MAX_INPUT_CHARACTERS=48000
```

The key is a **LiteLLM gateway virtual key**, not an upstream model-provider key. Configure actual model routes and ordered fallbacks in LiteLLM separately. A new provider is configuration-only **only if** it passes Mujeeb's supported API features and contract tests.

Docs: https://docs.litellm.ai/docs/interactions

**Stage 2 flag behavior:** With `LITELLM_CATALOG_ENABLED=false`, Gemini catalog evaluation is unchanged. With `true`, the complete catalog evaluation for that run (batch, reduction and final) is handled by the configured LiteLLM catalog route; the application still owns coverage, schema evidence, candidate validation and permissions. Do not enable until end-to-end tests with live model routes and pricing pass.

**Stage 3 flag behavior:** `LITELLM_MERCHANT_ENABLED=false` preserves the existing Gemini B2B authoring path. When enabled together with the configured LiteLLM connection, B2B authoring requests go through the gateway, but the existing `merchantcatalogai.Agent` still owns history/session isolation and its existing `ExecutionService` owns commit/mutation and approval. Do not enable in production before spending/telemetry and live model-feature acceptance.

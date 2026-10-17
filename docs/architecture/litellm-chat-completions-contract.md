# Mujeeb 24 — Chat Completions through LiteLLM (Atomic branch)

Scope: `feat/catalog-complete-commercial-entity-atomic` (B2C decision, batch catalog evaluation, final catalog capability). No change to sales execution or Universal Catalog domain.

## Supported wire contract

The Go backend sends only `POST /v1/chat/completions` to LiteLLM. The proxy's configured deployments (not Go) own Gemini/DeepSeek/provider credentials, retry, fallback, cooldown and model selection.

Logical aliases in Go defaults:

- `mujeeb-sales`: customer message -> **exactly one** offered function call. `tool_choice=required`, `parallel_tool_calls=false`.
- `mujeeb-catalog`: batch -> `response_format.type=json_schema` with `strict=true`; final decision -> exactly one terminal capability call.
- `mujeeb-merchant`: existing merchant catalog authoring Chat Completions route; must be tested separately for strict schema/tool support.

**Compatibility is per deployment, not per brand.** Do not add a fallback merely because the model has Function Calling: verify all required tool modes, structured outputs, output limits, and context size against each exact model/version through the gateway. DeepSeek thinking modes and some provider endpoints do not support forced tool choice. An incompatible deployment must be excluded rather than sending a relaxed request. This branch does not provision live routes, API keys, or dashboard settings.

## Configure in LiteLLM community UI

1. Add Google AI Studio and/or DeepSeek credentials **inside LiteLLM**; pin a vetted LiteLLM release.
2. Create deployments/aliases `mujeeb-sales`, `mujeeb-catalog`, `mujeeb-merchant`. Start with one verified provider per alias.
3. Add a second deployment to an alias or configure a fallback **only if** the same contract tests pass. Set per-model timeouts, budgets and retries in the proxy. Retrying must not execute any transaction twice: only Mujeeb services apply commercial effects.
4. Create a least-privileged virtual key limited to these aliases; configure `LITELLM_BASE_URL`, `LITELLM_API_KEY` and `LITELLM_GATEWAY_ONLY=true` in Go. Never use a LiteLLM master key in the application.
5. Keep `LITELLM_MAX_CONTEXT_TOKENS` below the smallest **verified** context window in each applicable route, including fallbacks. The token counter is conservative, not a proof of provider capacity.

## Contract boundaries

- Trusted Mujeeb instructions and catalog entity contract are in a separate `system` message; customer utterance + contextual evidence are in `user`.
- All B2C function tools use `strict=true`; every object is closed, and all properties are required (use JSON null for optional references).
- Dynamic merchant-defined sales fields cannot be open-ended JSON objects in the strict function schema. They are sent as `transaction_data_json` and `selected_attributes_json` *JSON-object strings*, decoded and checked in Go before passing unchanged to the existing sales runtime.
- Model results are proposals only; tenant policy, price, inventory, versioning and transaction writes remain inside Mujeeb.
- Catalog items must remain atomic and receive full commercial evidence; no silent truncation, incomplete candidate success, or weaker fallback schema.
- Provider errors do not expose raw response bodies or private prompts; cost headers and correlation IDs remain attributed to the AI run.

## Verify before enabling production

`bash scripts/verify-repository.sh`

Test all three roles on each exact deployment and on forced provider failures/429/503, malformed/multiple tool calls, incomplete responses, invalid selected references, oversized full items, token-budget denial, and real catalog schema/offer preservation. Check the gateway's actual `model_id`, usage, and cost. **Do not claim provider compatibility from Go mocks alone.**

No production LiteLLM database or dashboard is modified by this commit.

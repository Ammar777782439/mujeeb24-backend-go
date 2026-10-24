# Provider-neutral conversation memory — checkpointed summary

Scope: customer-sales automatic replies on `feat/gemini-deepseek`. Does not redefine the merchant/catalog domain, customer intents, or capabilities.

## Authority and sources
- PostgreSQL stores the complete original conversation and current operational states. Summaries are untrusted conversational memory, not proof of transfer, booking, payment, subscription, contact, or delivery.
- `conversation_state.summary` remains free-form Arabic text. No per-sector fields or keyword router.
- `conversation_state.summary_last_message_id` (migration 000080) records the last **chronological message** represented by committed memory. `summary_turn_count` remains inbound-count metadata, not a cursor.
- `verified_execution_state` is assembled from repository-backed conversation lifecycle. The model may select a capability but cannot execute it.
- Customer sales tool definitions, strict arguments and transaction authorization are unchanged by this memory patch.

## Complete, provider-independent context
- Requests to Gemini and DeepSeek use the same provider-neutral CustomerSalesContext. The provider does not retain authority for memory across stateless requests.
- For a checkpointed summary, load **every** message newer than the checkpoint in chronological order, excluding the current source message duplicated in the request.
- A legacy uncheckpointed summary retains the bounded four-message fallback until it is rebuilt from actual messages.
- The background summarizer pages stored history, normalizes DESC database order, preserves the latest four raw messages, and advances its checkpoint only with a verified version-compare-and-set.
- Customer messages may arrive consecutively; no assumption of one outgoing message for every incoming message.
- A concurrent state writer cannot overwrite summary text or its checkpoint with stale conversation focus.
- Every newly covered message is used in chronological order; no arbitrary UTF-8 substring truncation.
- Provider-summary generation is a **separate generation-only API request with NO sales tools**, using the same configured Gemini primary / optional DeepSeek 503 fallback.
- A plain-text Arabic rolling summary preserves ongoing goals, constraints, unresolved questions and corrections, and explicitly distinguishes assistant promises from execution facts without inventing a fixed business scenario.

## Safety and cost bounds
- Summaries are refreshed in the best-effort background path after eligible successful auto-reply runs; update is skipped if below the 4-inbound threshold, or if the complete history cannot be read.
- At most 10,000 historical messages and eight 3,900-byte chunks are allowed per refresh. Incomplete paging and oversized messages cause a visible error; summary/checkpoint are not committed.
- At most 256 uncheckpointed messages are attached to a decision. Beyond that limit, context construction fails safely instead of silently hiding messages.
- Summary usage is billed/tracked as an internal model request, not a final AI customer reply.
- A failed summary call does not alter committed summary. Failed model responses do not prove failure of the capability contract.
- No production database operations or customer-message sends occur from the live model-only tests.

## Test matrix and what it proves
- Go regression: descending multi-page history normalized to chronological order (including equal-timestamp ties); cursor corruption detected; no Unicode truncation; chunk budget bounded; unsummarized gap retrieval and missing checkpoint fail closed. A cold-start test verifies that creating memory for an informational-only conversation continues through generation and checkpoint commit.
- PostgreSQL integration: idempotent state initialization; checkpoint CAS rejects stale version; normal state writes preserve committed summary.
- Gemini/DeepSeek real integration workflow: three context representations plus independent tool-free summary generation, each using a real provider secret. Missing secrets / provider HTTP 503 are explicit incomplete results, not passes.
- Do not choose summary+one-message as an unconditional default until live comparisons prove it safe. The checkpoint-based delta design reduces duplication without discarding messages.

## Deployment note
Apply migration 000080 **before** starting the new binary, because state readers now select `summary_last_message_id`. Keep all existing summaries and messages; do not reset production history. No merge to backend main is part of this work.

## Verification limitations
- The latest live Gemini context-comparison attempt returned Google HTTP 503 before a model decision was received. Earlier real subscription handoff checks succeeded, but do not prove the new memory-strategy comparison.
- `DEEPSEEK_API_KEY` was not present in GitHub Actions during the last attempt; a real DeepSeek response has **not** been verified. The workflow fails explicitly rather than reporting success for an unexecuted provider.
- PostgreSQL integration in `quality-gate.yml` uses a disposable PostgreSQL 16 database, not `PLATFORM_DATABASE_URL` or customer data. Production remains unchanged. These checks establish schema/transaction compatibility only after the workflow completes successfully.

# Mujeeb 24 — Channel history purge & SocialAPI brand lifecycle (implementation gate)

Status: DESIGN ONLY / NOT IMPLEMENTED. Target branch: `feat/universal-catalog-ai-v3`. Do not merge or deploy until all gates pass.

## Verified provider contract (2026-10-08)
- `GET /v1/brands` lists **visible** brands; a brand-scoped API key hides out-of-scope brands and a 404 does NOT prove a brand was deleted. Only an authorized full-access management key may create/delete brands.
- `POST /v1/brands` is NOT idempotent; names are not unique. Persist the returned ID; prevent races and duplicate creation.
- `DELETE /v1/accounts/{id}` disconnects that account; `DELETE /v1/brands/{id}` deletes the brand AND disconnects every account under it (204). Never delete the brand on the first of several channel disconnects.
- Inbox API provides listing, reading, status/archiving, replies and sync; **no general delete-all-DM-history endpoint is documented**. The provider says it does not store DM message contents; the platform remains authoritative. Do not imply local purge deletes messages on Instagram/Facebook/WhatsApp.
- Sources: https://docs.social-api.ai/llms.txt ; https://docs.social-api.ai/api-reference/brands/delete-a-brand ; https://docs.social-api.ai/api-reference/accounts/disconnect-a-social-account ; https://social-api.ai/blog/direct-messaging-api-2026 ; https://social-api.ai/blog/apis-for-managing-multiple-social-media-accounts

## Product contract
1. `Disconnect channel` is a **separate** action from `Delete local channel message history`, and from billing / subscription state. No automatic message purge for disconnect, expiry, suspension or downgrade.
2. Explicit purge targets exactly ONE `business_id + channel_connection_id` (not an untrusted channel label, nor a whole provider brand). An authorized owner/admin must explicitly confirm irreversible deletion.
3. Purge affects only customer-facing conversation message content for that connection, including relevant AI/human replies if the UX promises the entire thread history; define private-note policy explicitly before implementation.
4. Keep business profile, catalogs, customers, leads, commercial transactions, subscription, billing and channel connection active. Never cascade into these domain entities.
5. A purge must remain effective against retries, webhook replay and inbox re-sync while allowing NEW messages to arrive after the purge boundary.
6. No manual production SQL or external provider DELETE from the implementation/test workflow.

## Database dependency audit (schema.sql)
- `communication_messages` stores message text plus content references and links to `conversation_references`, `inbound_event_ledger`, `outbound_messages`.
- `inbound_webhook_payloads.payload` contains original raw webhook bytes and can contain customer text. `inbound_event_ledger.raw_payload_reference` links to it by string; deleting `communication_messages` alone is NOT a complete privacy deletion.
- `outbound_messages`, `outbox_entries`, `outbound_delivery_updates`, `automation_executions`, `ai_decisions`, `ai_runs` and traces can reference conversation/message/event data. Audit actual payload fields and retention before writing a DELETE.
- `conversation_references` is referenced by `leads` and `commercial_transactions` (ON DELETE RESTRICT), so DO NOT blanket-delete conversations/references or change foreign keys to CASCADE.
- `channel_connections` has seven downstream foreign key relationships; avoid deleting the row as part of message-only purge.

## Required implementation sequence
A. Build full dependency/personal-data inventory for a single connection (not just text rows); identify materialized message text, raw webhook payload, uploaded attachment content, AI prompts/traces, summaries, outbox bodies, cache and async jobs; classify fields that must be erased vs non-content audit needed for idempotency.
B. Introduce a single application port and repository operation for `PreviewChannelHistoryPurge` and `PurgeChannelHistory`; reuse existing `TransactionManager`, auth, scoped repo, Huma DTO and response envelope. No parallel bespoke REST handler or duplicated provider adapter.
C. Proposed API (to finalize after storage audit): 
   - `GET /businesses/{business_id}/channel-connections/{connection_id}/message-purge-preview` (preview counts and exclusions).
   - `POST /businesses/{business_id}/channel-connections/{connection_id}/message-purge`, request DTO `{confirmation: "DELETE_CHANNEL_HISTORY", expected_resource_version: "..."}`, and mandatory `Idempotency-Key`. Return an operation ID/status, deleted counts and no message bodies. `202` if async, `200` only if complete.
   - Create request/response types in `internal/adapters/primary/http/dto`, alias through `contract`, register once with Huma OpenAPI, dispatch via existing command façade, wire dependencies in bootstrap. Frontend consumes the DTO; **do not add a disconnected UI button without a working backend**.
D. Before purge, acquire a tenant/connection-scoped lock, reject connection mismatch, verify authorization/operation idempotency, and prevent in-flight processing from re-creating old messages; use a durable cutoff/tombstone keyed by business+connection+event time. Preserve post-cutoff customer traffic. Complete multi-table removal in correct FK order via a transaction, with batched/async execution only if transaction size warrants it.
E. Retain minimal non-content event dedupe keys if necessary to prevent history replay; scrub raw bodies, message text, attachments and message-bearing traces. Validate that retained metadata contains no message text. Do NOT promise full deletion of third-party messages.
F. Separate provider brand lifecycle: verify the saved ID under an ADMIN key; if invisible due to key restriction, fail closed rather than silently creating duplicates. Reconcile a genuinely deleted brand with a stable tenant mapping and version check. Last-channel disconnection may delete a brand ONLY when there are no other active/reconnect/pending connections or OAuth sessions, and only after provider/DB cleanup can be safely retried. This operation must not purge local history.
G. Tests: tenant isolation (two merchants + two channels), only one channel affected, provider restricted-key 404, double create, partial disconnect, last disconnect, provider DELETE 404/429/500/timeouts, concurrency (new OAuth vs deletion), preview vs purge, double click/idempotent retry, webhook replay/sync older-than-cutoff, post-cutoff new incoming messages, rollback on DB errors, preserved customers/catalog/orders/leads/subscriptions, raw payload and AI-content scrub, DTO/OpenAPI compile, PostgreSQL integration fixture and `go test ./...`.
H. Shipping gate: no production deployment until PostgreSQL integration tests and DTO-contract tests pass; the repository alone cannot verify live SocialAPI credentials or production data.

## Known partial work on this feature branch
Three earlier preparatory commits introduced `ProviderBrandStore.Replace`, `ProviderBrandCleanupStore`, a PostgreSQL implementation and an OAuth locking change, but did **not** wire those capabilities into the application lifecycle or test them. Treat as unfinished scaffolding: integrate with tests or revert before merge. Do not leave unused ports/methods in a shippable branch.

## Confirmed merchant requirement
Delete the full local conversation history for one channel connection, including customer messages, merchant replies and AI replies; do not disconnect the channel or alter subscription, business, catalog, leads or commercial transactions.

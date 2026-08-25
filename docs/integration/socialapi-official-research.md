# SocialAPI.ai — research baseline

**التاريخ:** 25 أغسطس 2026

هذا الملف يلخص ما قرأه الفريق من توثيق SocialAPI.ai الرسمي قبل تعديل adapter. لا يحتوي مفاتيح API أو أسرارًا أو raw payloads من حسابات حقيقية.

## المصادر الرسمية

1. https://docs.social-api.ai/guides/authentication.md — Authentication.
2. https://docs.social-api.ai/guides/concepts.md — Core concepts.
3. https://docs.social-api.ai/guides/inbox.md — Unified inbox.
4. https://docs.social-api.ai/guides/webhooks.md — Webhooks.
5. https://docs.social-api.ai/guides/errors.md — Error handling.
6. https://docs.social-api.ai/api-reference/accounts/connect-a-social-account.md — Connect a social account.
7. https://docs.social-api.ai/api-reference/inbox/list-messages-in-a-conversation.md — List messages.
8. https://docs.social-api.ai/api-reference/inbox/send-a-message-in-a-conversation.md — Send a message.

## Facts extracted from the official docs

SocialAPI REST requests use `Authorization: Bearer <token>`. API keys use the `sapi_key_` form and are intended for server-side integrations; the docs explicitly warn not to expose keys in frontend code. OAuth 2.1 access tokens are intended for MCP clients and are short-lived. Error responses contain an `error` object with stable namespaced `code`, human-readable `message`, optional `meta`, and `request_id`.

SocialAPI models Brands, Accounts, Posts, Interactions, and Capabilities. Account IDs and interaction IDs are opaque strings and must be stored as text; the adapter must not parse their prefixes or embedded values as a contract dependency.

The unified inbox exposes DM conversations and messages through:

- `GET /v1/inbox/conversations`
- `GET /v1/inbox/conversations/:id`
- `PATCH /v1/inbox/conversations/:id`
- `POST /v1/inbox/conversations/:id/read`
- `GET /v1/inbox/conversations/:id/messages`
- `POST /v1/inbox/conversations/:id/messages`

Message list responses are paginated and include `data`, `pagination`, `sync_state`, and `last_synced_at`. Message rows include opaque `id`, `conversation_id`, `platform_id`, `sender_id`, `sender_name`, `text`, `direction`, `status`, `created_at`, and optional attachment fields. Send-message requests require `account_id` and at least one of text, attachment, or supported interactive content. The response includes `success`, `message_id` (deprecated compatibility field), and `message_ids`.

Account connection uses `POST /v1/accounts/connect`. OAuth-based platforms return HTTP 202 with `auth_url`, `message`, optional metadata, and state. Direct credential flows can return HTTP 201 with an account ID. The request can include `brand_id`, `platform`, `redirect_uri`, `state`, and platform metadata. Mujeeb must persist only provider references and connection state, never provider secrets in ordinary domain records.

SocialAPI webhooks are registered with `POST /v1/webhooks` and require an HTTPS URL plus a non-empty events array. Registration returns the endpoint secret once. Creating an endpoint sends a `webhook.test` verification ping before the secret can be used, so the receiver must acknowledge that verification event without normal signature verification, as documented. Event types include `comment.received`, `dm.received`, `dm.sent`, `dm.referral`, `dm.postback`, `review.received`, `mention.received`, `dm.status.sent`, `dm.status.delivered`, `dm.status.read`, and `dm.status.failed`, along with post/account lifecycle events.

Webhook payloads have top-level `event` and `data`; platform-originated events may also contain top-level `raw_payload`. The normalized `data` interaction is the preferred contract. Headers include `X-SocialAPI-Signature` (HMAC-SHA256 of raw body), `X-SocialAPI-Signature-V2` (HMAC-SHA256 of timestamp plus raw body), `X-SocialAPI-Timestamp`, `X-SocialAPI-Delivery`, and `X-SocialAPI-Event`. The receiver must preserve exact raw bytes and use constant-time comparison. V2 timestamp freshness and delivery ID are required for replay protection and dedupe when available; the delivery ID is stable across retries.

The docs distinguish ordinary reads from inbox sync. `POST /v1/inbox/sync` returns HTTP 202 and must be polled via `GET /v1/inbox/sync`; forced syncs are rate limited. This matters for a future resolver and must not be confused with webhook ingestion.

Provider/API errors are mapped by stable namespaces. Authentication failures are 401; account and permission problems can be 403/404; rate limits are 429; unsupported platform operations are 501; upstream failures can be 502. Mujeeb must preserve the stable code and request ID as adapter diagnostics without leaking provider raw payloads into the Dashboard contract.

## Implementation consequences for Mujeeb

The SocialAPI adapter should have separate methods for account listing/connection, inbox conversation/message reads, outbound message send, webhook registration, and webhook normalization/verification. It should use typed provider DTOs privately, map them to application ports, treat IDs as opaque strings, preserve pagination and sync metadata, and classify provider errors without inventing successful delivery. Network calls must remain outside database transactions; outbound delivery must be driven by Outbox/Worker later.

The first safe implementation target is the DM path plus webhook normalization: list conversations, list messages, send message, account connect response handling, webhook signature verification (v1 and v2 where the application contract supports it), event normalization, and typed error mapping. Comments/reviews/mentions and publishing should be added only when Mujeeb application use cases require them.

## Detailed endpoint contract extracted on 25 August 2026

`GET /v1/accounts` returns `{count,data}`. Account fields include required `id`, `brand_id`, `platform`, `status`, `name`, and `username`, with optional `bio`, `login_id`, `metadata`, `page_name`, `profile_picture_url`, and `reconnect_reason`. IDs are opaque strings.

`POST /v1/accounts/connect` accepts `platform` plus optional `brand_id`, `redirect_uri`, `state`, and platform metadata. OAuth flows return HTTP 202 with `auth_url`; direct credential flows can return HTTP 201 with `account_id`, `display_name`, `platform`, and `username`. The adapter must support both response shapes and must not require `redirect_uri` for a direct flow unless the application use case explicitly requires OAuth.

`GET /v1/inbox/conversations` returns paginated DM conversations with optional `account_id`, `brand_id`, `page_id`, `platform`, `status`, `limit` (1–100), and `cursor`. The response contains `data`, `pagination`, `sync_state`, and `last_synced_at`. `GET /v1/inbox/conversations/:id/messages` returns newest-first paginated message rows with `id`, `conversation_id`, `platform_id`, `sender_id`, `sender_name`, `text`, `direction`, `status`, `created_at`, `status_updated_at`, and optional attachment fields; its `limit` is 1–200.

`POST /v1/inbox/conversations/:id/messages` requires `account_id` and at least one text, attachment, or interactive field. The response is `{success,message_id,message_ids}`. The 24-hour platform window and platform permission errors are upstream business errors, not transport failures. Mujeeb must preserve stable SocialAPI error code/status/request ID and must not convert an accepted response into a delivered claim.

`POST /v1/webhooks` requires an HTTPS URL and non-empty events. It returns HTTP 201 with endpoint `id`, `url`, `events`, and a secret shown once. SocialAPI sends a signed `webhook.test` verification ping before saving the endpoint; the receiver must acknowledge that event without normal signature verification. Event deliveries include `X-SocialAPI-Signature` for raw-body HMAC, `X-SocialAPI-Signature-V2` for timestamp-plus-raw-body HMAC, `X-SocialAPI-Timestamp`, `X-SocialAPI-Delivery`, and `X-SocialAPI-Event`. The delivery ID is stable across retries; use it with the provider event ID for dedupe, while preserving the raw body for verification.

`GET /v1/inbox/sync` and `POST /v1/inbox/sync` are separate from webhook ingestion. A forced sync returns 202 and is polled; repeated forced syncs are rate-limited. It must not be run automatically inside a database transaction or confused with a successful provider delivery.

Official errors use `{error:{code,message,meta},request_id}`. Relevant statuses include 401 auth, 403 permission/reconnection, 404 missing resource, 429 rate limit, 501 unsupported operation, and 502 upstream failure. `X-Request-ID` mirrors `request_id` and should be retained in adapter diagnostics.

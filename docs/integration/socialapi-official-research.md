# SocialAPI.ai — research baseline

**التاريخ:** 25 أغسطس 2026

هذا الملف يلخص ما قرأه الفريق من توثيق SocialAPI.ai الرسمي قبل تعديل adapter. لا يحتوي مفاتيح API أو أسرارًا أو raw payloads من حسابات حقيقية.

## المصادر الرسمية

1. https://docs.social-api.ai/guides/authentication — Authentication.
2. https://docs.social-api.ai/guides/concepts — Core concepts.
3. https://docs.social-api.ai/guides/inbox — Unified inbox.
4. https://docs.social-api.ai/guides/webhooks — Webhooks.
5. https://docs.social-api.ai/guides/errors — Error handling.
6. https://docs.social-api.ai/api-reference/accounts/connect-a-social-account — Connect a social account.
7. https://docs.social-api.ai/api-reference/inbox/list-messages-in-a-conversation — List messages.
8. https://docs.social-api.ai/api-reference/inbox/send-a-message-in-a-conversation — Send a message.

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

## Second-pass official audit — 25 August 2026

أُعيد فتح الصفحات الرسمية التالية من النص الكامل، لا من snippets فقط: [Authentication](https://docs.social-api.ai/guides/authentication)، [Connect a social account](https://docs.social-api.ai/api-reference/accounts/connect-a-social-account)، [List connected accounts](https://docs.social-api.ai/api-reference/accounts/list-connected-accounts)، [List inbox conversations](https://docs.social-api.ai/api-reference/inbox/list-inbox-conversations)، [Get a conversation](https://docs.social-api.ai/api-reference/inbox/get-a-conversation)، [List messages](https://docs.social-api.ai/api-reference/inbox/list-messages-in-a-conversation)، [Send a message](https://docs.social-api.ai/api-reference/inbox/send-a-message-in-a-conversation)، [Unified inbox guide](https://docs.social-api.ai/guides/inbox)، [Webhooks guide](https://docs.social-api.ai/guides/webhooks)، [Create webhook endpoint](https://docs.social-api.ai/api-reference/webhooks/create-webhook-endpoint)، [List webhook endpoints](https://docs.social-api.ai/api-reference/webhooks/list-webhook-endpoints)، [Update webhook endpoint](https://docs.social-api.ai/api-reference/webhooks/update-webhook-endpoint)، [Select Google Profile or Facebook Pages](https://docs.social-api.ai/api-reference/accounts/select-a-google-business-profile-or-facebook-pages-to-connect)، و[official documentation index](https://docs.social-api.ai/llms.txt).

نتيجة التدقيق تؤكد أن authentication هو `Authorization: Bearer <token>`، وأن API keys بصيغة `sapi_key_` للاستخدام server-side فقط. `POST /v1/accounts/connect` يدعم HTTP 202 مع `auth_url` لتدفق OAuth، وHTTP 201 مع `account_id` وبيانات الحساب للتدفق المباشر. `GET /v1/accounts` يعيد `{count,data}` ويدعم `brand_id`.

نتيجة التدقيق تؤكد أن قائمة المحادثات تستخدم `GET /v1/inbox/conversations` مع `account_id`, `brand_id`, `page_id`, `platform`, `status`, `limit` من 1 إلى 100، و`cursor`، وأن response يحتوي `data`, `pagination`, `sync_state`, و`last_synced_at`. قراءة الرسائل تستخدم `GET /v1/inbox/conversations/{id}/messages`، newest-first، وبحد أقصى 200، وبحقول `id`, `conversation_id`, `platform_id`, `sender_id`, `sender_name`, `text`, `direction`, `status`, `created_at`, و`status_updated_at` مع حقول attachment اختيارية.

نتيجة التدقيق تؤكد أن إرسال الرسالة يستخدم `POST /v1/inbox/conversations/{id}/messages`، ويقبل `account_id` وواحدًا على الأقل من `text` أو attachment أو interactive content، ويعيد `success`, `message_id` deprecated، و`message_ids`. لم توثق صفحة endpoint أي `Idempotency-Key` header؛ لذلك أزيل من SocialAPI HTTP request. يبقى `IdempotencyKey` داخل Mujeeb metadata/application-outbox contract فقط، ولا يُقدَّم كضمان exactly-once عند provider.

نتيجة التدقيق تؤكد أن Webhook registration هو `POST /v1/webhooks`، مع HTTPS URL وevents غير فارغة، ويعيد HTTP 201 والـsecret مرة واحدة. `webhook.test` يجب acknowledge له بـ2xx دون التحقق من التوقيع الأولي وفق التوثيق. توقيع v1 هو HMAC-SHA256 للـraw body، وتوقيع v2 هو HMAC-SHA256 للنص `<timestamp>.<raw body>`، مع constant-time comparison ونافذة freshness موصى بها خمس دقائق. `X-SocialAPI-Delivery` ثابت عبر retries ويُستخدم للـdedupe، بينما retry يحمل timestamp جديدًا. التوثيق يذكر أيضًا response خلال 10 ثوانٍ و5 محاولات retry. أما list/get/update/delete ومراقبة deliveries فهي عقود رسمية منفصلة ولم تُضاف إلى Mujeeb adapter لأنها ليست application use case معتمدًا في هذه المرحلة.

نتيجة التدقيق على الأخطاء تؤكد الشكل `{error:{code,message,meta},request_id}`، وأن 401 authentication و403 permission/reconnection و404 missing resource و429 rate limit و501 `resource.not_supported` و502 upstream لها semantics مختلفة. صُنّف 401 و403 و404 و501 كغير قابلة لإعادة المحاولة، وصُنّف 429 و500 و502 فقط كقابلة لإعادة المحاولة في typed adapter error. لا يترجم ذلك إلى retry تلقائي؛ قرار retry النهائي يملكه Outbox/Worker بعد اكتماله، مع بقاء unknown external outcome ممنوعًا من resend أعمى.

### Evidence mapping to the implementation

| Official contract | Mujeeb implementation/evidence | Result |
|---|---|---|
| Bearer authentication | `client.go` request builder sets `Authorization: Bearer ...`; httptest asserts it | PASS — contract test |
| OAuth 202 and direct 201 connect | `BeginConnection`, `TestClientListsAccountsAndBeginsConnection`, `TestClientReadsInboxContractAndRegistersWebhook` | PASS — fake HTTP contract |
| Inbox query/pagination and message rows | `ListInboxConversations`, `ListConversationMessages`, corresponding query/response assertions | PASS — fake HTTP contract |
| v1 raw-body HMAC | `VerifyWebhook`, `TestClientVerifiesSocialAPIV1RawBodySignature` | PASS — local cryptographic contract |
| v2 timestamp/raw-body HMAC and replay window | `VerifyWebhook`, `TestClientNormalizesAndVerifiesWebhookV2`, `TestClientRejectsReplayAndAcceptsVerificationPing` | PASS — local cryptographic contract |
| `webhook.test` acknowledgement | `VerifyWebhook` bypass before normal signature checks; test assertion | PASS — local contract |
| Stable delivery ID/fallback identity | `NormalizeWebhook` separates `ProviderEventID` and `DeliveryID`; fallback strategy is explicit | PASS — local normalization contract |
| Official error shape and retry semantics | `parseAPIError`, `retryableStatus`, `TestClientMapsOfficialHTTPErrorSemantics` | PASS — local fake HTTP contract |
| Provider idempotency | No claim and no `Idempotency-Key` header is sent; Mujeeb key remains internal | PASS — negative assertion |
| Webhook lifecycle list/update/delete | Not implemented intentionally; no approved Mujeeb use case yet | NOT IN SCOPE |
| Live reachability, account connect, webhook registration, delivery, or Facebook send | No live call was run | NOT TESTED / BLOCKED BY DECISION |

This second-pass audit is evidence of documentation/code alignment only. It is not evidence that SocialAPI accepted a live request or that a Facebook message was delivered.

## References

[1]: https://docs.social-api.ai/guides/authentication "SocialAPI.ai Authentication"
[2]: https://docs.social-api.ai/api-reference/accounts/connect-a-social-account "SocialAPI.ai Connect a social account"
[3]: https://docs.social-api.ai/api-reference/accounts/list-connected-accounts "SocialAPI.ai List connected accounts"
[4]: https://docs.social-api.ai/api-reference/inbox/list-inbox-conversations "SocialAPI.ai List inbox conversations"
[5]: https://docs.social-api.ai/api-reference/inbox/get-a-conversation "SocialAPI.ai Get a conversation"
[6]: https://docs.social-api.ai/api-reference/inbox/list-messages-in-a-conversation "SocialAPI.ai List messages in a conversation"
[7]: https://docs.social-api.ai/api-reference/inbox/send-a-message-in-a-conversation "SocialAPI.ai Send a message in a conversation"
[8]: https://docs.social-api.ai/guides/inbox "SocialAPI.ai Unified inbox"
[9]: https://docs.social-api.ai/guides/webhooks "SocialAPI.ai Webhooks"
[10]: https://docs.social-api.ai/api-reference/webhooks/create-webhook-endpoint "SocialAPI.ai Create webhook endpoint"
[11]: https://docs.social-api.ai/api-reference/webhooks/list-webhook-endpoints "SocialAPI.ai List webhook endpoints"
[12]: https://docs.social-api.ai/api-reference/webhooks/update-webhook-endpoint "SocialAPI.ai Update webhook endpoint"
[13]: https://docs.social-api.ai/api-reference/accounts/select-a-google-business-profile-or-facebook-pages-to-connect "SocialAPI.ai Select Google Profile or Facebook Pages"
[14]: https://docs.social-api.ai/llms.txt "SocialAPI.ai documentation index"
[15]: https://docs.social-api.ai/guides/errors "SocialAPI.ai Error Handling"

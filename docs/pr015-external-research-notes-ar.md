# PR-015 External Research Notes

## SocialAPI.ai — official docs

Sources reviewed:

- https://docs.social-api.ai/
- https://docs.social-api.ai/guides/authentication

Verified facts from the official documentation:

1. SocialAPI.ai exposes a unified REST API and supports inbox capabilities across Instagram, Facebook, Threads, YouTube, Google Business Profile, X/Twitter, Telegram DMs, and WhatsApp DMs. The platform-support page must still be checked per feature before promising a channel capability.
2. Most external integrations use API keys with the `sapi_key_` prefix. Requests use `Authorization: Bearer ...`.
3. OAuth 2.1 access tokens are described for MCP clients; the normal external integration path is an API key. We must not implement an OAuth flow for Mujeeb without confirming the specific account-connection API contract.
4. The docs show unified inbox paths such as `/v1/inbox/comments`, `/v1/inbox/conversations`, and `/v1/inbox/reviews`, and an example comment reply endpoint. These examples do not by themselves prove a WhatsApp/Instagram/Facebook DM send endpoint.
5. API keys are returned in full only once when created/rotated, so Mujeeb must keep the key server-side and never put it in DTOs, logs, Domain records, or tenant-visible responses.
6. SocialAPI.ai documents request IDs, webhooks, pagination, error handling, and publishing reliability as separate concepts; these must map into provider-neutral Mujeeb ports rather than leak into Domain.

## Current implementation consequence

Do not use the Gmail credentials supplied in chat as a SocialAPI credential. Do not perform a live account connection or outbound send until the exact target service, API key, account scope, webhook secret, and explicit test action are confirmed. Begin with provider-neutral adapter contracts and contract tests; live calls must be opt-in and non-destructive.

## Chatwoot — official docs

Sources reviewed:

- https://developers.chatwoot.com/api-reference/introduction
- https://developers.chatwoot.com/api-reference/messages/create-new-message

Verified facts from the official documentation:

1. Chatwoot distinguishes Application APIs, Client APIs, and Platform APIs. Application APIs use a user `access_token`; Client APIs use `inbox_identifier` and `contact_identifier`; Platform APIs are for installation-level administration and have separate permissions.
2. For a server-side Mujeeb integration that mirrors conversations into a merchant's Chatwoot account, the Application API boundary is the relevant candidate, but the exact token/account ownership model must be decided before live wiring.
3. Creating a Chatwoot message uses `POST /api/v1/accounts/{account_id}/conversations/{conversation_id}/messages` with `api_access_token`. Text uses JSON; attachments use multipart form data. The payload includes `content`, `message_type`, `private`, `content_type`, and optional attributes/template fields.
4. Chatwoot responses expose numeric account/inbox/conversation/message identifiers and delivery-like status values such as sent/delivered/read/failed. Mujeeb must keep these as external references, not replace Mujeeb-owned message identity.
5. The docs describe WhatsApp template parameters in the message API. Capability checks must remain provider/channel-specific and must not assume every text operation is available for every channel.
6. Chatwoot official docs explicitly note that API documentation can differ from UI behavior; live integration should use a controlled test account and record request/response contract evidence without storing tokens or raw sensitive payloads.

## Current implementation consequence

SocialAPI and Chatwoot should be implemented behind provider-neutral `ChannelProvider` and `CommunicationWorkspace` ports. Do not call either service from Domain, PostgreSQL repositories, or HTTP handlers. Do not use the Gmail credentials supplied in chat as API credentials for either service.

## SocialAPI.ai — OAuth and Webhook details

Sources reviewed:

- https://docs.social-api.ai/guides/oauth
- https://docs.social-api.ai/guides/webhooks

Verified facts:

1. SocialAPI manages the platform OAuth connection on behalf of the integrator. Mujeeb initiates `POST /v1/accounts/connect` with `platform`, `redirect_uri`, optional opaque `state`, and optionally `brand_id`; the response contains an `auth_url` and the user is redirected there.
2. The platform access token is never shown to Mujeeb. SocialAPI stores it encrypted and proxies subsequent calls. Mujeeb should persist only the external `account_id` and provider-neutral connection references.
3. Facebook connections can return `selection_required` with a pending `connection_id`; no account exists until page selection is completed through the documented pending-selection endpoint. This is a real flow gap that must be modeled, not hidden as a normal success.
4. SocialAPI webhook endpoints require HTTPS and a non-empty event list. Registration sends a `webhook.test` verification event before the endpoint is saved; the endpoint must respond with 2xx. The first verification event has a special signature-verification caveat documented by SocialAPI.
5. Relevant webhook event types include `comment.received`, `dm.received`, `dm.sent`, `dm.status.sent`, `dm.status.delivered`, `dm.status.read`, `dm.status.failed`, `account.connected`, `account.disconnected`, and `page.removed`. The authoritative catalog is available from `GET /v1/webhooks/events`.
6. SocialAPI webhooks are signed and retried. Mujeeb must verify signatures, apply replay protection, persist the raw event safely through EventStore, return ACK only after durable acceptance, and rely on EventStore dedupe for repeated deliveries.
7. The webhook payload can include referral/postback and platform-specific metadata. It must not be copied unfiltered into Domain or logs; only normalized fields and safe references should cross the application boundary.

## Design decision

The first live vertical slice cannot honestly be implemented as "connect any Facebook account and immediately become ACTIVE". It must include the documented pending page-selection state, callback state correlation, webhook verification, signature/replay checks, and account mapping. These are adapter/integration concerns, not Domain hacks.

## SocialAPI.ai — account and conversation API reference

Sources reviewed:

- https://docs.social-api.ai/api-reference/accounts/list-connected-accounts
- https://docs.social-api.ai/api-reference/inbox/send-a-message-in-a-conversation

Verified facts:

1. Connected accounts are listed with `GET https://api.social-api.ai/v1/accounts`, using an Authorization header and optional `brand_id` filter. Returned accounts contain external `id`, `brand_id`, `platform`, `status`, `username/name`, and metadata.
2. The API reference exposes `POST /v1/inbox/conversations/{id}` for sending a message in an existing SocialAPI conversation. It requires the provider conversation ID and an `account_id` in the payload; the exact complete body schema must be extracted from the reference before production use.
3. This endpoint is distinct from Mujeeb's normalized `communication_messages` and `outbound_messages`; SocialAPI IDs must remain external references.
4. A connection integration must account for account-level and brand-level mapping. Mujeeb's Business owns its `ChannelConnection`; SocialAPI's `brand_id`/`account_id` are provider references.

## Chatwoot — contact and conversation API reference

Sources reviewed:

- https://developers.chatwoot.com/api-reference/contacts/create-contact
- https://developers.chatwoot.com/api-reference/conversations/create-new-conversation

Verified facts:

1. Application API contact creation is `POST /api/v1/accounts/{account_id}/contacts` with `api_access_token`, and `inbox_id` is required. The body supports `name`, email/phone, external `identifier`, additional attributes, and custom attributes.
2. Chatwoot conversation creation is `POST /api/v1/accounts/{account_id}/conversations` and requires a `source_id`; it can also receive `inbox_id`, `contact_id`, status, attributes, and an initial message.
3. Chatwoot uses numeric account/inbox/contact/conversation IDs. Mujeeb must store these as external mapping references, not as its primary business or conversation IDs.
4. A real integration needs configured Chatwoot account/inbox identifiers and a server-side user/agent API token; the previously supplied Gmail login is not an API token and must not be used as one.

## Delivery and Chatwoot webhook notes

Sources reviewed:

- https://docs.social-api.ai/api-reference/inbox/list-messages-in-a-conversation
- https://www.chatwoot.com/hc/user-guide/articles/1677693021-how-to-use-webhooks

Verified facts:

1. SocialAPI exposes a paginated list of messages for a conversation, and message records include external IDs/status fields; however delivery state is explicitly represented by SocialAPI webhook events (`dm.status.*`). The adapter should treat webhooks as authoritative for delivery transitions and not poll as a substitute for durable inbound handling.
2. Chatwoot webhooks are configured per account under Integrations → Webhooks and can subscribe to selected events. This is an account-level internal callback, distinct from the SocialAPI signed webhook stream.
3. A safe integration must validate which Chatwoot events are needed for mirror synchronization and must not forward every Chatwoot payload into Mujeeb's Domain.

## Chatwoot webhook verification

The reviewed Chatwoot help-center page documents `X-Chatwoot-Signature`, `X-Chatwoot-Timestamp`, and optional `X-Chatwoot-Delivery`. The signature is `sha256=` plus HMAC-SHA256 over `<timestamp>.<raw_body>`, using the webhook secret. Verification must use the raw bytes and constant-time comparison; a five-minute freshness window is recommended. The delivery ID is appropriate for deduplicating retries, while the event body ID remains the provider event/message reference.

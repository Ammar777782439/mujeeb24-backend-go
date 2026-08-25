# Chatwoot Official Direction Contract

## Sources

- https://www.chatwoot.com/hc/user-guide/articles/1677839703-how-to-create-an-api-channel-inbox
- https://www.chatwoot.com/hc/user-guide/articles/1677693021-how-to-use-webhooks
- https://developers.chatwoot.com/api-reference/messages/create-new-message

## Findings

Chatwoot's official API-channel guide states that messages sent by the end user are classified as `incoming`, while messages sent by an agent are classified as `outgoing`. Its webhook sample for `message_created` includes `message_type` as `incoming` or `outgoing`.

The official webhook guide also documents the Message object with `message_type` as an integer in webhook payloads, where the current implementation normalizes `0` to `incoming`, `1` to `outgoing`, `2` to `activity`, and `3` to `template`. The API-channel guide shows the string form in the sample webhook payload. Therefore the adapter must accept both string and numeric forms and must not infer direction from the sender ID alone.

Chatwoot signs outgoing webhooks using `X-Chatwoot-Timestamp` and `X-Chatwoot-Signature`, with `sha256=HMAC-SHA256(secret, timestamp + "." + raw_body)`. `X-Chatwoot-Delivery` can be present as a delivery identifier. Verification must use the raw request body and constant-time comparison.

## Implementation consequence

For `message_created`, only `message_type=incoming` may enter the AutoReply path. `outgoing`, `activity`, `template`, private notes, and unknown message types must not trigger AutoReply. They may still be materialized or acknowledged according to the existing inbound contract, but they must be excluded from the AutoReply orchestration.

A missing `message_type` must not be silently treated as a normal inbound event in the AutoReply path. Existing legacy normalization tests that omit the field can continue to test materialization compatibility, but the orchestration gate must require an explicit incoming direction. This is the safe default that prevents an AI/Chatwoot echo loop.

# Conversation Media & Attachments — Contract and Implementation Status

**Branch:** `feat/conversation-media-attachments`  
**Status:** Domain, tenant-safe persistence and inbound webhook placeholder materialization are implemented. Provider media download, upload HTTP APIs, public message projections and outbound delivery are not implemented.

## 1. Current repository facts

- `migrations/000083_catalog_media_mvp` introduced `media_assets` for Catalog images and `catalog_media_links` for CatalogItem/Variant galleries.
- `migrations/000084_catalog_media_backblaze` added Backblaze as a supported storage provider for the existing media objects.
- `CatalogMediaRepository` remains the owner of Catalog gallery rules. It still creates assets with `purpose='catalog'`.
- `communication_messages` is the Mujeeb-owned communication record and has a composite unique key `(business_id, id)`.
- The existing SocialAPI adapter can send a single image URL in its outbound message request, but that is not a generic multi-format media contract.

## 2. Separation rule

Shared media bytes and metadata do not imply shared domain relationships.

```text
CatalogItem / Variant
  -> catalog_media_links
  -> MediaAsset(purpose=catalog)

CommunicationMessage
  -> message_attachments
  -> MediaAsset(purpose=conversation_attachment)

Business brand media
  -> future business-owned relation
  -> MediaAsset(purpose=business_brand, reserved only)
```

`catalog_media_links` must never be used for message attachments. CatalogItem, Variant and Offer contracts remain unchanged. The `business_brand` purpose is reserved by the current schema but no brand relation or brand upload use case is implemented by this PR.

## 3. Implemented in the current branch

- Provider-neutral `AttachmentKind` values: `image`, `video`, `audio`, `document`, `file`.
- Inbound references are provider-neutral opaque references with MIME/name/size values treated as untrusted hints.
- Outbound references identify a Mujeeb-owned `MediaAssetID`; they contain no storage URL, bucket key, or provider media ID.
- Migration `000087_conversation_media_attachments` adds `media_kind`, adds the `pending_ingestion` asset status, permits the `conversation_attachment` purpose, and creates `message_attachments`.
- The private S3 transport accepts only `catalog/` and `conversation/` key namespaces. Catalog behavior remains controlled by `CATALOG_MEDIA_ENABLED`; conversation object-store initialization has its own `CONVERSATION_MEDIA_ENABLED` flag.
- `message_attachments` references both `communication_messages` and `media_assets` through composite `(business_id, id)` foreign keys.
- `ConversationMediaIngestionRepository` creates pending conversation assets for supported logical kinds and promotes verified image metadata only.
- SocialAPI webhook normalization reads `content.media[]`, validates that its URLs are HTTPS, discards those URLs, and emits opaque references based on the stable SocialAPI interaction ID and attachment position.
- `InboundMediaSourceResolver` resolves that opaque ID through SocialAPI's authenticated conversation message-history endpoint and returns the current `attachment_url` only for immediate ingestion use; the URL is not stored.
- `ProviderInboundStore.Materialize` creates pending MediaAsset and MessageAttachment records atomically with the communication message. The provider reference is not a URL.
- `MessageAttachmentRepository` creates and lists tenant-scoped links. A relation can become `ready` only if its underlying conversation MediaAsset is already `ready` and has the same kind.
- Provider source references are opaque IDs only; HTTP(S) URLs are rejected by the repository and database constraint. The reference is cleared after successful ingestion.
- The repositories perform local SQL only. Their methods can join a caller-owned `TransactionManager` transaction via the transaction context.

## 4. Current implementation limit

**The verified-to-ready path is image-only.** The source resolver follows the current SocialAPI message-history schema, which exposes one `attachment_url` per message. It resolves position zero; positions above zero are rejected as unsupported. The actual HTTP download, byte verification and object-storage write remain pending. Image/video/audio/file events can be materialized as pending placeholders; there is not yet a worker to download their bytes, determine verified MIME/size, write Object Storage, or transition them to ready. The current `media_assets` MIME and verification contract is still image-specific.

No claim is made that WhatsApp, Instagram and Facebook expose identical media behavior. Inbound retrieval and outbound delivery must be verified separately for every channel and media kind before enabling them.

## 5. Inbound flow — placeholder materialization wired; download pending

```text
SocialAPI webhook
  -> signature verification + event ledger/idempotency
  -> normalize content.media[] to opaque SocialAPI interaction-id/index references
  -> persist message + pending MediaAsset + MessageAttachment in one local transaction
  -> source resolver looks up the stable interaction ID in SocialAPI message history
  -> [next] claim pending reference through an internal worker port
  -> [next] fetch provider bytes outside the transaction
  -> [next] verify MIME, size, dimensions and checksum
  -> [next] write bytes to private Object Storage
  -> [next] mark MediaAsset and MessageAttachment ready in one local transaction
  -> [next] publish attachment state change
```

Do not block webhook acknowledgement on a media download. If ingestion fails, preserve the text message and mark the attachment unavailable/failed.

## 6. Required outbound flow — not wired yet

```text
Merchant upload request
  -> authenticated upload intent
  -> private Object Storage
  -> server-side verification
  -> MediaAsset ready
  -> outbound command references MediaAsset IDs
  -> tenant + readiness + capability validation
  -> message_attachments + outbound message + outbox in one local transaction
  -> outbox worker resolves provider payload outside SQL transaction
  -> SocialAPI send + delivery reconciliation
```

Never persist public or presigned URLs as canonical references. Generate a short-lived read URL only at the authorized delivery boundary. An unknown provider outcome follows the existing dead-letter/reconciliation rule; never blind-retry.

## 7. Explicitly not implemented

- SocialAPI provider media download endpoint and durable ingestion worker. The new storage feature flag only prepares the private S3 router; it does not mean downloads are active yet.
- Durable worker claim/read API for pending provider references (the current relation holds only an opaque source reference).
- Generic authenticated upload/finalize API.
- Message DTO projection and realtime attachment-state event.
- Outbound message commands, Outbox attachment resolver changes, and provider-specific send capability checks.
- Video/audio/document ingestion, scanning, thumbnails, retention and media quota accounting.
- Business logo/cover relation and brand-asset management.
- Live SocialAPI or Object Storage media tests.

## 8. Next implementation gates

1. Add the scoped pending-reference claim/read port and durable ingestion worker.
2. Resolve the whole opaque SocialAPI interaction ID and attachment position through the authenticated conversation message-history endpoint; use the returned `attachment_url` only for immediate ingestion.
3. Download and verify image bytes, store them privately, and transition asset+relation atomically.
4. Add authorized message attachment projections and realtime state notifications.
5. Add authenticated private upload/finalize for outbound media, then route through Outbox.
6. Verify per-channel constraints and live delivery before enabling the feature.
7. Complete brand-media relation, orphan cleanup, storage quotas and backup/restore proof.

## 9. Provider contract references

- Repository evidence: [`docs/integration/socialapi-official-research.md`](../integration/socialapi-official-research.md).
- Upstream message history: [List messages in a conversation](https://docs.social-api.ai/api-reference/inbox/list-messages-in-a-conversation).
- Upstream inbox behavior: [Unified inbox](https://docs.social-api.ai/guides/inbox).

SocialAPI interaction IDs are treated as opaque strings. The adapter encodes and decodes the whole ID for an internal reference; it does not parse prefixes or embedded platform identifiers.

This is a persistence and source-resolution foundation, not end-to-end support for customer media.
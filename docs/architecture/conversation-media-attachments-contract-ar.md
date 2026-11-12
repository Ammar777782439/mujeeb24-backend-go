# Conversation Media & Attachments — Contract and Implementation Status

**Branch:** `feat/conversation-media-attachments`  
**Status:** Tenant-safe persistence, inbound WhatsApp image ingestion, merchant image upload, scoped private reads, manual outbound image attachment persistence, and SocialAPI outbox resolution are implemented. Live provider/storage verification and frontend send-image UX remain separate gates.

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
- `POST /api/v1/businesses/{business_id}/conversation-media/uploads` accepts a bounded base64 JPEG/PNG image, verifies bytes and dimensions server-side, writes it to the recorded private-storage provider, and returns a `ready` `MediaAsset(purpose=conversation_attachment)`. It does not create a message relation; outbound message attachment support is still pending.
- Conversation message projections list attachment IDs, media IDs, kind, caption and state without exposing storage keys or SocialAPI references. The conversation/message-scoped read URL endpoint checks tenant, conversation, public message, attachment and ready asset before returning a five-minute signed URL.
- Provider source references are opaque IDs only; HTTP(S) URLs are rejected by the repository and database constraint. The reference is cleared after successful ingestion.
- The repositories perform local SQL only. Their methods can join a caller-owned `TransactionManager` transaction via the transaction context.

## 4. Current implementation limit

**Ready media remains image-only.** The inbound source resolver follows SocialAPI message history, which exposes one `attachment_url` per message. The current worker claims only the first WhatsApp image attachment (`sort_order=0`), downloads it through SocialAPI's authenticated media proxy, verifies JPEG/PNG bytes and dimensions, and stores it privately. Other channels, extra attachment positions, and video/audio/document/file kinds remain pending and are not marked ready. Merchant uploads also accept JPEG/PNG only, with a 10 MiB decoded-size bound and server-side dimension/pixel checks.

No claim is made that WhatsApp, Instagram and Facebook expose identical media behavior. Inbound retrieval and outbound delivery must be verified separately for every channel and media kind before enabling them.

## 5. Inbound flow — first WhatsApp image slice implemented

```text
SocialAPI webhook
  -> signature verification + event ledger/idempotency
  -> normalize content.media[] to opaque SocialAPI interaction-id/index references
  -> persist message + pending MediaAsset + MessageAttachment in one local transaction
  -> source resolver looks up the stable interaction ID in SocialAPI message history
  -> lease-based claim (WhatsApp image, first position only)
  -> authenticated SocialAPI media lookup and download outside SQL
  -> JPEG/PNG, size, dimensions and checksum verification
  -> private Object Storage write to the MediaAsset's recorded provider
  -> mark MediaAsset and MessageAttachment ready through local SQL
  -> message list exposes state; authorized read URL is available when ready
```

Do not block webhook acknowledgement on a media download. If ingestion fails, preserve the text message and mark the attachment unavailable/failed.

## 6. Outbound flow — implemented for one manual image

```text
Merchant upload request
  -> POST /businesses/{business_id}/conversation-media/uploads (base64 JPEG/PNG)
  -> private Object Storage
  -> server-side verification
  -> MediaAsset ready
  -> manual outbound command references attachment_media_ids (maximum one)
  -> tenant + purpose + readiness + feature flag + verified capability validation
  -> OutboundMessage + CommunicationMessage + MessageAttachment + Outbox in one local transaction
  -> outbox worker resolves the relation and recorded storage provider outside SQL transaction
  -> short-lived HTTPS URL is created at dispatch
  -> SocialAPI image send
```

Never persist public or presigned URLs as canonical references. Generate a short-lived read URL only at the authorized delivery boundary. An unknown provider outcome follows the existing dead-letter/reconciliation rule; never blind-retry.

## 7. Explicitly not implemented

- Generic multipart/upload-intent/finalize API; the current direct upload endpoint supports base64 JPEG/PNG only.
- Realtime attachment-state event after a pending inbound attachment becomes ready or failed.
- Live SocialAPI send confirmation and production object-storage read/write verification.
- Video/audio/document ingestion, scanning, thumbnails, retention and media quota accounting.
- Business logo/cover relation and brand-asset management.
- Live SocialAPI or Object Storage media tests.

## 8. Next implementation gates

1. Complete controlled live SocialAPI image-send and private object-storage read/write verification before enabling this capability for merchants.
2. Emit tenant-scoped realtime attachment-state notifications when inbound ingestion completes or fails.
3. Add Facebook/Instagram inbound media only after proving provider URL retrieval and SSRF/redirect protections.
4. Add additional attachment positions and video/audio/document/file only after provider evidence, format-specific verification and tests.
5. Define orphan cleanup, storage quotas, retention and backup/restore proof.
6. Add multipart/upload-intent/finalize only if measured product requirements justify it.

## 9. Provider contract references

- Repository evidence: [`docs/integration/socialapi-official-research.md`](../integration/socialapi-official-research.md).
- Upstream message history: [List messages in a conversation](https://docs.social-api.ai/api-reference/inbox/list-messages-in-a-conversation).
- Upstream inbox behavior: [Unified inbox](https://docs.social-api.ai/guides/inbox).

SocialAPI interaction IDs are treated as opaque strings. The adapter encodes and decodes the whole ID for an internal reference; it does not parse prefixes or embedded platform identifiers.

This is a persistence and source-resolution foundation, not end-to-end support for customer media.
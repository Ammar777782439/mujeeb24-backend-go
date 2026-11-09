# Conversation Media & Attachments — Contract and Implementation Status

**Branch:** `feat/conversation-media-attachments`  
**Status:** Domain and tenant-safe persistence foundation implemented. Provider retrieval, upload HTTP APIs, public message projections and delivery are not implemented.

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
- `message_attachments` references both `communication_messages` and `media_assets` through composite `(business_id, id)` foreign keys.
- `ConversationMediaIngestionRepository` creates pending conversation image assets and promotes verified image metadata.
- `MessageAttachmentRepository` creates and lists tenant-scoped links. A relation can become `ready` only if its underlying conversation MediaAsset is already `ready` and has the same kind.
- Provider source references are opaque IDs only; HTTP(S) URLs are rejected by the repository and database constraint. The reference is cleared after successful ingestion.
- The repositories perform local SQL only. Their methods can join a caller-owned `TransactionManager` transaction via the transaction context.

## 4. Current implementation limit

**The enabled ingestion path is image-only.** The other logical kinds are represented in the domain contract but cannot yet be created as ready conversation assets. The current `media_assets` MIME constraint and verification contract are still image-specific.

No claim is made that WhatsApp, Instagram and Facebook expose identical media behavior. Inbound retrieval and outbound delivery must be verified separately for every channel and media kind before enabling them.

## 5. Required inbound flow — not wired yet

```text
SocialAPI webhook
  -> signature verification + event ledger/idempotency
  -> normalize provider media reference
  -> persist message + pending MediaAsset + message_attachments in one local transaction
  -> claim pending reference through an internal worker port
  -> fetch provider bytes outside the transaction
  -> verify signature/content, MIME, size, dimensions and checksum
  -> write bytes to private Object Storage outside the transaction
  -> mark MediaAsset and MessageAttachment ready in one local transaction
  -> publish attachment state change
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

- SocialAPI inbound media normalization and provider download endpoint.
- Durable worker claim/read API for pending provider references.
- Generic authenticated upload/finalize API.
- Message DTO projection and realtime attachment-state event.
- Outbound message commands, Outbox attachment resolver changes, and provider-specific send capability checks.
- Video/audio/document ingestion, scanning, thumbnails, retention and media quota accounting.
- Business logo/cover relation and brand-asset management.
- Live SocialAPI or Object Storage media tests.

## 8. Next implementation gates

1. Add the scoped pending-reference claim/read port and durable ingestion worker.
2. Normalize verified SocialAPI webhook media references and materialize pending relations with the inbound message transaction.
3. Download and verify image bytes, store them privately, and transition asset+relation atomically.
4. Add authorized message attachment projections and realtime state notifications.
5. Add authenticated private upload/finalize for outbound media, then route through Outbox.
6. Verify per-channel constraints and live delivery before enabling the feature.
7. Complete brand-media relation, orphan cleanup, storage quotas and backup/restore proof.

This is a persistence foundation, not end-to-end support for customer media.
# Conversation Media & Attachments — Persistence Slice 2

**Branch:** `feat/conversation-media-attachments`  
**Scope:** Tenant-safe media ingestion metadata, message-to-media relations, and inbound webhook normalization/materialization. The SocialAPI downloader, upload endpoint, and outbound attachment transport are still not enabled.

## What changed

- Migration `000087_conversation_media_attachments` expands `media_assets.purpose` to `catalog`, `conversation_attachment`, and `business_brand`.
- Existing assets are retained as `media_kind=image`; the new kind field supports the provider-neutral logical kinds already defined in the channel contract.
- `pending_ingestion` is a distinct MediaAsset lifecycle state, separate from a merchant's pending upload.
- `message_attachments` is a dedicated relation to `communication_messages` and `media_assets`, both guarded by composite `(business_id, id)` foreign keys.
- `ConversationMediaIngestionRepository` creates tenant-scoped pending assets for image/video/audio/document/file and promotes verified image metadata only.
- `MessageAttachmentRepository` creates/lists message attachment relations and marks them ready only after the backing MediaAsset is ready.
- The S3 adapter now permits the isolated `conversation/` object namespace alongside `catalog/`; the Catalog feature flag remains independent from `CONVERSATION_MEDIA_ENABLED`.
- SocialAPI `content.media[]` is normalized for inbound events. The temporary URL is validated and discarded; only an opaque SocialAPI interaction-id/attachment-index reference crosses into the domain.
- `InboundMediaSourceResolver` re-queries authenticated conversation history using cursor pagination and resolves the interaction to its current attachment source. SocialAPI's documented message-history contract exposes one attachment per message, so only position zero is currently resolvable. For WhatsApp and Telegram, the resolver recognizes SocialAPI's `/v1/inbox/media/{id}` URL and returns only the opaque media ID; other platforms return their platform-hosted HTTPS URL.
- `ProviderInboundStore.Materialize` creates the pending MediaAsset and MessageAttachment rows in the same PostgreSQL transaction as the inbound communication message.

## Important boundary

- `catalog_media_links` continues to serve only CatalogItem/Variant galleries.
- A Catalog-purpose asset cannot be attached to a conversation.
- The current verified-to-ready path remains image-only. Video/audio/document/file can be represented as pending placeholders, but are not yet downloaded, verified, made ready, or delivered.
- Source provider media references are opaque identifiers only. URL values are rejected before persistence and are never copied into a message DTO.
- The provider reference is cleared from the relation after successful ingestion; a later worker will need a dedicated, tenant-scoped internal claim/read port while ingestion is pending.

## Atomicity and operations

Inbound normalization stores only an opaque SocialAPI message reference and attachment position. It never persists a temporary media URL. The `MediaAsset` placeholder and `MessageAttachment` relation are created in the same transaction as the inbound message; a failure rolls the entire materialization back.

The new repositories use the transaction executor from the caller context. They perform only local SQL; object storage and SocialAPI calls must remain outside SQL transactions. The intended worker sequence is: fetch provider bytes outside a transaction, verify bytes, write private storage, then mark the MediaAsset and MessageAttachment ready inside one short local transaction.

## Verification added

The PostgreSQL integration suite now checks:
- inbound materialization creates image and file pending placeholders with the message,
- pending conversation asset creation and attachment linkage,
- tenant isolation between the message and its media,
- refusing to mark the relation ready while the asset is pending,
- successful transition after verified image metadata is recorded,
- rejection of Catalog-purpose assets from Conversation.

This does not yet prove real SocialAPI media retrieval or Object Storage upload/download. The source resolver is covered by HTTP contract tests only; pending placeholders remain pending until the provider fetcher and media ingestion worker are implemented. `CONVERSATION_MEDIA_ENABLED` currently initializes the storage router in the worker, but does not run media downloads.

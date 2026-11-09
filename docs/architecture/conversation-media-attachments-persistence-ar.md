# Conversation Media & Attachments — Persistence Slice 2

**Branch:** `feat/conversation-media-attachments`  
**Scope:** Tenant-safe media ingestion metadata and message-to-media relations. The SocialAPI downloader, upload endpoint, and outbound attachment transport are still not enabled.

## What changed

- Migration `000087_conversation_media_attachments` expands `media_assets.purpose` to `catalog`, `conversation_attachment`, and `business_brand`.
- Existing assets are retained as `media_kind=image`; the new kind field supports the provider-neutral logical kinds already defined in the channel contract.
- `pending_ingestion` is a distinct MediaAsset lifecycle state, separate from a merchant's pending upload.
- `message_attachments` is a dedicated relation to `communication_messages` and `media_assets`, both guarded by composite `(business_id, id)` foreign keys.
- `ConversationMediaIngestionRepository` creates tenant-scoped pending image assets and promotes only verified image metadata.
- `MessageAttachmentRepository` creates/lists message attachment relations and will mark them ready only after the backing MediaAsset is ready.

## Important boundary

- `catalog_media_links` continues to serve only CatalogItem/Variant galleries.
- A Catalog-purpose asset cannot be attached to a conversation.
- This slice only enables verified image ingestion. Video/audio/document/file values remain logical contract values; there is no claim that those formats can be uploaded, downloaded or delivered yet.
- Source provider media references are opaque identifiers only. URL values are rejected before persistence and are never copied into a message DTO.
- The provider reference is cleared from the relation after successful ingestion; a later worker will need a dedicated, tenant-scoped internal claim/read port while ingestion is pending.

## Atomicity and operations

The new repositories use the transaction executor from the caller context. They perform only local SQL; object storage and SocialAPI calls must remain outside SQL transactions. The intended worker sequence is: fetch provider bytes outside a transaction, verify bytes, write private storage, then mark the MediaAsset and MessageAttachment ready inside one short local transaction.

## Verification added

The PostgreSQL integration suite now checks:
- pending conversation asset creation and attachment linkage,
- tenant isolation between the message and its media,
- refusing to mark the relation ready while the asset is pending,
- successful transition after verified image metadata is recorded,
- rejection of Catalog-purpose assets from Conversation.

This does not yet prove real SocialAPI media retrieval or Object Storage upload/download.

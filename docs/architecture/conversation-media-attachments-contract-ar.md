# Conversation Media & Attachments — Domain Contract (Slice 1)

**Status:** Implemented domain contract groundwork on feat/conversation-media-attachments. Persistence, HTTP upload, provider retrieval and delivery are not implemented by this slice.

## 1. Repository baseline

This work starts from main of Ammar777782439/mujeeb24-backend-go.

Relevant current implementation observed before this change:

- migrations/000083_catalog_media_mvp creates media_assets with purpose = catalog and catalog_media_links.
- migrations/000084_catalog_media_backblaze permits the existing catalog assets to use R2 or Backblaze.
- CatalogMediaRepository and MediaObjectStorage currently serve the catalog image feature.
- SendMessageRequest currently accepts text only.
- InboundEvent and ProviderInboundDraft currently carry text and provider identifiers, but no attachment collection is materialized.
- The HTTP MessageAttachment projection currently describes an image with a catalog media_id/preview path. It is not a general persisted conversation attachment contract.
- SocialAPI outbound messaging currently rejects more than one attachment and only accepts image URLs.

This contract does not claim end-to-end attachment support.

## 2. Locked separation rule

**Shared media infrastructure does not mean shared domain relationships.**

CatalogItem and Variant media remain Catalog-owned relationships through catalog_media_links. Conversation attachments belong to Communication and will use an explicit MessageAttachment relation. Do not use catalog_media_links for messages, and do not change CatalogItem/Offer/Variant contracts to accommodate chat media.

The existing media_assets schema is catalog-scoped today. Generalizing it requires a forward-only migration after the broader MediaAsset contract is reviewed. Do not alter migrations 000083/000084.

## 3. Domain-level attachment references added in this slice

### AttachmentKind

The provider-neutral kinds are image, video, audio, document and file. This is a logical classification. It does not prove that a channel/provider supports the kind or that the file bytes actually match it.

### InboundAttachmentReference

An inbound message carries an opaque provider reference, plus untrusted descriptive hints:

- ProviderMediaReference
- Kind
- MIMEType (hint only)
- FileName (hint only)
- Caption
- SizeBytes (zero means unknown; negative is invalid)

It must not contain a durable Mujeeb storage URL/key or assume the provider URL is permanent. The SocialAPI adapter must resolve provider media through an explicitly supported provider operation. If the provider exposes only a URL, the adapter must treat it as a transient retrieval detail, not as Mujeeb's canonical file location.

### OutboundAttachmentReference

An outbound message refers to a Mujeeb-owned MediaAssetID, the logical kind and an optional caption. It deliberately contains no presigned storage URL, bucket/key, or SocialAPI media identifier. Authorization, tenant ownership, readiness and channel capability checks happen in Application before dispatch.

Both reference types have pure validation functions. Validation rejects unknown kinds, missing stable references/IDs, and negative inbound sizes. It does not validate actual MIME bytes, file safety, size limits or provider capabilities.

## 4. Storage ownership

PostgreSQL stores metadata, ownership, lifecycle and relations. Private Object Storage stores file bytes. A conversation attachment must be linked to the tenant-scoped message and media asset through an explicit relation, such as a future message_attachments table with composite tenant-safe foreign keys.

Do not place bytes/Base64 in PostgreSQL message rows. Do not persist a long-lived public URL. Signed read URLs are generated only after authorization and only for a short-lived operation.

## 5. Inbound lifecycle — future implementation

SocialAPI webhook
  -> signature verification + event ledger/idempotency
  -> normalize attachment references
  -> materialize message + pending attachment relation
  -> enqueue provider media retrieval
  -> fetch through SocialAPI adapter
  -> verify bytes, MIME, size and checksum
  -> private Object Storage
  -> mark MediaAsset/MessageAttachment ready
  -> publish message/attachment state update

A webhook must not block while downloading large media. A media retrieval failure must not discard the text message; the attachment remains failed/unavailable with an explicit state and reason safe for the merchant.

## 6. Outbound lifecycle — future implementation

Merchant client
  -> upload intent
  -> private Object Storage
  -> finalize and verify bytes
  -> MediaAsset ready
  -> send message with MediaAsset IDs
  -> tenant + readiness + capability validation
  -> message + attachment rows + outbound + outbox in one local transaction
  -> outbox worker resolves provider payload
  -> SocialAPI adapter sends
  -> delivery status reconciliation

No network call may occur inside a PostgreSQL transaction. An unknown provider outcome must follow the repository's dead-letter/reconciliation policy; never perform a blind retry.

## 7. Channel capabilities

Do not promise WhatsApp, Facebook and Instagram parity based on one provider example. Inbound and outbound support are separate capabilities and must be verified per channel and media kind, including size/MIME/count limits and provider retrieval semantics. The current SocialAPI adapter's one-image outbound behavior is not a universal attachment contract.

No numeric media limits are locked by this document. They must be sourced from the current provider contract and proven in channel-specific tests.

## 8. Explicitly out of scope for Slice 1

- No database migration or new table.
- No generic upload/finalize HTTP API.
- No SocialAPI webhook parsing or media download.
- No outbound multi-attachment delivery.
- No AI image/document processing.
- No catalog migration or modification.
- No feature flag enablement or live provider claim.

## 9. Next implementation gates

1. Review and close the full MediaAsset + MessageAttachment persistence contract.
2. Add forward-only tenant-safe schema and repository integration tests.
3. Implement generic upload/finalize on private storage while preserving existing Catalog API compatibility.
4. Implement inbound attachment normalization, provider retrieval worker and durable storage.
5. Implement outbound attachment commands, Outbox delivery and provider-specific capability checks.
6. Add live channel tests, retention, orphan cleanup, quotas and backup/restore proof.

This document describes an incremental engineering boundary, not a declaration that customer media is already supported end-to-end.
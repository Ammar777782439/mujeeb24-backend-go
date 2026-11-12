# Mujeeb 24 — Conversation Media Handoff

**Updated:** 2026-11-12  
**Repository:** `Ammar777782439/mujeeb24-backend-go`  
**Branch:** `feat/conversation-media-attachments`  
**PR:** [#35](https://github.com/Ammar777782439/mujeeb24-backend-go/pull/35) — Open, Draft, not merged.  
**Current branch HEAD at review:** `c52f1b31758e6a43a360e3ad05a0d39206b78b0f`.
**PR state:** Open / Draft / not merged.

## Current implementation

This is more than the initial persistence foundation. The branch currently implements:

1. Provider-neutral domain attachment kinds and inbound/outbound references.
2. `media_assets` purpose extension plus the independent `message_attachments` relation. Catalog stays on `catalog_media_links`; never reuse that relation for messages.
3. Atomic inbound webhook materialization: CommunicationMessage + pending MediaAsset + MessageAttachment in one PostgreSQL transaction. The provider reference is opaque; temporary CDN URLs are not persisted.
4. Durable lease/retry queue using migration 000088, claim with `FOR UPDATE SKIP LOCKED`, attempts, next retry time, lease expiry and safe error code.
5. First inbound worker slice: **WhatsApp image only, first attachment position only**. It resolves the source through SocialAPI history, downloads through SocialAPI's authenticated media proxy, verifies JPEG/PNG bytes and dimensions, limits decoded bytes to 10 MiB and pixels to 16 million, calculates SHA-256, writes to the storage provider recorded on the MediaAsset, then transitions the asset and relation.
6. Merchant image upload endpoint: `POST /api/v1/businesses/{business_id}/conversation-media/uploads`. It accepts Base64 JPEG/PNG, verifies bytes/dimensions, stores privately, and creates a ready conversation-purpose MediaAsset. It does **not** attach that asset to an outbound message.
7. Message attachment projection plus a tenant/conversation/message/attachment-scoped read URL endpoint with a five-minute signed URL. Storage keys/provider references are not returned in DTOs.
8. Manual outbound image attachments: `attachment_media_ids` accepts at most one merchant-owned `MediaAsset` ID. The Application Service checks the tenant, `purpose=conversation_attachment`, ready/not-deleted state, JPEG/PNG MIME, pinned storage metadata, `CONVERSATION_MEDIA_ENABLED`, and a verified `media_outbound` capability (`enabled`, checked timestamp, non-empty evidence source).
9. Outbound persistence creates `OutboundMessage`, public `CommunicationMessage`, `MessageAttachment`, and Outbox row inside the same PostgreSQL transaction. The relation is the canonical attachment reference; no URL or storage key is accepted from the client or stored in message content.
10. Outbound dispatch re-reads the relation and ready asset, rechecks the connection capability, presigns from the provider recorded on the asset for 45 minutes, then passes one `image` HTTPS URL to SocialAPI. An unavailable/mismatched attachment is dead-lettered before provider invocation; unknown provider send outcomes retain the existing no-blind-retry behavior.

## Important limits

- Ready inbound media: WhatsApp images only. Other channels and media kinds may be represented as pending, but are not ingested/verified/marked ready.
- Message history exposes one attachment URL per message in the currently implemented resolver; position zero only is supported.
- Outbound scope is deliberately **one image per message**, JPEG or PNG only. No video/audio/document/multiple-attachment delivery.
- SocialAPI adapter code accepts one image with HTTPS URL, but no live SocialAPI send was performed. A connection without a verified `media_outbound` capability is rejected.
- The new PostgreSQL integration test for outbound atomicity was added after the last fully green CI run; it has not yet been verified on the current branch HEAD.
- No realtime event after inbound media transitions to ready/failed.
- No generic multipart or upload-intent/finalize API.
- No logo/cover business-brand relation.
- No live SocialAPI, R2 or Backblaze end-to-end test was run. Gemini Live Smoke is manual and skipped in CI.

## Migrations and database baseline

- 000087 `conversation_media_attachments`: `media_kind`, `conversation_attachment` purpose, `pending_ingestion` state, `message_attachments` with composite tenant-safe FKs.
- 000088 `conversation_media_ingestion_queue`: lease/retry fields and queue index.
- Existing Catalog migrations 000083/000084 were not modified.
- PostgreSQL 16 schema snapshot: migration_count 88, 81 tables, 1025 columns, 717 constraints, 29 standalone unique indexes.
- The source inventory comment already records migrations 000001..000088; no follow-up is required there.

## Feature flag

`CONVERSATION_MEDIA_ENABLED=false` by default. Enabling it requires SocialAPI API key and configured private object storage. Object-storage writes and provider downloads happen outside SQL transactions.

## CI status and verification boundary

The current branch HEAD c52f1b31758e6a43a360e3ad05a0d39206b78b0f has a complete green CI set:

| Workflow | Run | Result |
|---|---:|---|
| Native AI Contracts | 43218143153 | success |
| AI Contract Verification | 43218143404 | success |
| Quality gate | 43218143275 | success |
| Render image verification | 43218143161 | success |

This confirms automated checks on the current backend HEAD, including the outbound PostgreSQL integration test, feature gate, formatting/build/vet/full Go tests/race detector/OpenAPI checks and production Docker image verification.

CI does not prove live SocialAPI delivery, real R2/Backblaze object writes/reads, or merchant UI behavior. Those remain manual integration gates. Gemini Live Smoke remains manual/skipped.

## Next implementation priorities

### P0 — close the code and integration boundary
1. Keep PR #35 open until the code review is accepted; CI is green on current HEAD.
2. Frontend must consume the conversation/message/attachment-scoped read URL rather than the old tenant-wide Catalog preview path.
3. Add merchant image-send UX only against the `attachment_media_ids` contract; preserve the one-image JPEG/PNG limit.
4. Operator performs the controlled live SocialAPI image-send and private object-storage read/write checks. Do not claim provider E2E support before those checks.

### P1 — inbound completion events
Publish tenant-scoped realtime state changes after attachment becomes ready/failed; never expose provider references or storage keys.

### P2 — broader inbound support
Implement Facebook/Instagram URL retrieval only with DNS/IP SSRF protection and redirect restrictions. Add more positions/formats only after confirming provider API contract; add format-specific bounds/scanning.

### P3 — operations
Consider multipart/upload intent only if needed; add quotas, retention, orphan cleanup, metrics, backup/restore tests.

## Source files

- [Contract status](conversation-media-attachments-contract-ar.md)
- [Persistence and implementation notes](conversation-media-attachments-persistence-ar.md)
- [Domain attachments](../../internal/domain/channel/attachments.go)
- [Ports](../../internal/application/ports/conversation_media.go)
- [Ingestion worker](../../internal/application/services/conversation_media_ingestion.go)
- [Postgres repository](../../internal/adapters/secondary/persistence/postgres/conversation_media_repository.go)
- [Outbound delivery resolver](../../internal/application/services/outbound_delivery_resolver.go)
- [Outbound atomicity integration test](../../internal/adapters/secondary/persistence/postgres/outbound_conversation_media_integration_test.go)
- [SocialAPI adapter](../../internal/adapters/secondary/providers/socialapi/client.go)
- [Worker bootstrap](../../internal/bootstrap/worker.go)
- [Merchant image upload](../../internal/application/services/conversation_image_upload.go)
- [Read URL service](../../internal/application/services/conversation_media_read.go)
- [Migration 000087](../../migrations/000087_conversation_media_attachments.up.sql)
- [Migration 000088](../../migrations/000088_conversation_media_ingestion_queue.up.sql)
- [Postgres 16 schema baseline](../../internal/platform/database/testdata/merchant_assist_schema_pg16.json)
- [PR #35](https://github.com/Ammar777782439/mujeeb24-backend-go/pull/35)

## Instructions for next chat

Start from `feat/conversation-media-attachments`, not only main. Verify live branch HEAD, PR state and CI before edits. Do not reimplement inbound WhatsApp image ingestion, merchant upload, read projections, or the outbound single-image relation/delivery code. First run CI on the latest HEAD and fix integration failures. Then perform controlled live SocialAPI and private-storage verification, followed by realtime attachment state events. Do not touch/merge main without explicit approval; do not claim support for channels/formats that are not implemented and tested.

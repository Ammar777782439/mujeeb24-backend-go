# Mujeeb 24 — Conversation Media Handoff

**Updated:** 2026-11-10  
**Repository:** `Ammar777782439/mujeeb24-backend-go`  
**Branch:** `feat/conversation-media-attachments`  
**PR:** [#35](https://github.com/Ammar777782439/mujeeb24-backend-go/pull/35) — Open, Draft, not merged.  
**Verified code HEAD before this handoff note:** `2d3ae0c363df3ead67742f81229f31c86382946c`.

## Current implementation

This is more than the initial persistence foundation. The branch currently implements:

1. Provider-neutral domain attachment kinds and inbound/outbound references.
2. `media_assets` purpose extension plus the independent `message_attachments` relation. Catalog stays on `catalog_media_links`; never reuse that relation for messages.
3. Atomic inbound webhook materialization: CommunicationMessage + pending MediaAsset + MessageAttachment in one PostgreSQL transaction. The provider reference is opaque; temporary CDN URLs are not persisted.
4. Durable lease/retry queue using migration 000088, claim with `FOR UPDATE SKIP LOCKED`, attempts, next retry time, lease expiry and safe error code.
5. First inbound worker slice: **WhatsApp image only, first attachment position only**. It resolves the source through SocialAPI history, downloads through SocialAPI's authenticated media proxy, verifies JPEG/PNG bytes and dimensions, limits decoded bytes to 10 MiB and pixels to 16 million, calculates SHA-256, writes to the storage provider recorded on the MediaAsset, then transitions the asset and relation.
6. Merchant image upload endpoint: `POST /api/v1/businesses/{business_id}/conversation-media/uploads`. It accepts Base64 JPEG/PNG, verifies bytes/dimensions, stores privately, and creates a ready conversation-purpose MediaAsset. It does **not** attach that asset to an outbound message.
7. Message attachment projection plus a tenant/conversation/message/attachment-scoped read URL endpoint with a five-minute signed URL. Storage keys/provider references are not returned in DTOs.

## Important limits

- Ready inbound media: WhatsApp images only. Other channels and media kinds may be represented as pending, but are not ingested/verified/marked ready.
- Message history exposes one attachment URL per message in the currently implemented resolver; position zero only is supported.
- No outbound message attachment command/linking to MessageAttachment/Outbox delivery yet.
- No realtime event after inbound media transitions to ready/failed.
- No generic multipart or upload-intent/finalize API.
- No logo/cover business-brand relation.
- No live SocialAPI, R2 or Backblaze end-to-end test was run. Gemini Live Smoke is manual and skipped in CI.

## Migrations and database baseline

- 000087 `conversation_media_attachments`: `media_kind`, `conversation_attachment` purpose, `pending_ingestion` state, `message_attachments` with composite tenant-safe FKs.
- 000088 `conversation_media_ingestion_queue`: lease/retry fields and queue index.
- Existing Catalog migrations 000083/000084 were not modified.
- PostgreSQL 16 schema snapshot: migration_count 88, 81 tables, 1025 columns, 717 constraints, 29 standalone unique indexes.
- Small documentation cleanup: comment in `internal/domain/ai/contracts/source_inventory.go` still says migrations 000001..000087; update it to 000001..000088 (migration 88 adds queue columns/index, no new table).

## Feature flag

`CONVERSATION_MEDIA_ENABLED=false` by default. Enabling it requires SocialAPI API key and configured private object storage. Object-storage writes and provider downloads happen outside SQL transactions.

## Verified CI on code HEAD 2d3ae0c

| Workflow | Run | Result |
|---|---:|---|
| Quality gate | 42828908034 | success |
| AI Contract Verification — Build/Vet/Test + Race | 42828908218 | success |
| Gemini Live Smoke | 42828908218 | skipped (manual only) |
| Native AI Contracts + Sales PostgreSQL integration | 42828909397 | success |
| Render production Docker image verification | 42828908332 | success |

CI success does not imply live provider/storage verification.

## Next implementation priorities

### P0 — outbound attachments
1. Add outbound request/command contract referencing ready MediaAsset IDs.
2. Validate same business, purpose `conversation_attachment`, status `ready`, kind and channel capability.
3. Persist MessageAttachment + outbound message + Outbox row in one local transaction.
4. Resolve private media from the asset's recorded storage provider at dispatch; never persist presigned URLs.
5. Add SocialAPI provider payload behavior based on verified per-channel capability. Do not claim generic multi-attachment support without evidence.
6. Preserve existing unknown-provider-outcome reconciliation; never blind-retry a potentially sent message.

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
- [SocialAPI adapter](../../internal/adapters/secondary/providers/socialapi/client.go)
- [Worker bootstrap](../../internal/bootstrap/worker.go)
- [Merchant image upload](../../internal/application/services/conversation_image_upload.go)
- [Read URL service](../../internal/application/services/conversation_media_read.go)
- [Migration 000087](../../migrations/000087_conversation_media_attachments.up.sql)
- [Migration 000088](../../migrations/000088_conversation_media_ingestion_queue.up.sql)
- [Postgres 16 schema baseline](../../internal/platform/database/testdata/merchant_assist_schema_pg16.json)
- [PR #35](https://github.com/Ammar777782439/mujeeb24-backend-go/pull/35)

## Instructions for next chat

Start from `feat/conversation-media-attachments`, not only main. Verify live branch HEAD, PR state and CI before edits. Do not reimplement inbound WhatsApp image ingestion, merchant image upload or message read projections. Start with the **outbound attachment command and atomic MessageAttachment + Outbox persistence**, then provider delivery. Do not touch/merge main without explicit approval; do not claim support for channels/formats that are not implemented and tested.

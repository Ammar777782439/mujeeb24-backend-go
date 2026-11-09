# Conversation Media & Attachments — Persistence Slice 2

**Branch:** `feat/conversation-media-attachments`  
**Scope:** Tenant-safe media ingestion metadata, message-to-media relations, inbound webhook normalization/materialization, and the first bounded WhatsApp image-ingestion worker slice. The public upload endpoint and outbound attachment transport are still not enabled.

## What changed

- Migration `000087_conversation_media_attachments` expands `media_assets.purpose` to `catalog`, `conversation_attachment`, and `business_brand`.
- Existing assets are retained as `media_kind=image`; the new kind field supports the provider-neutral logical kinds already defined in the channel contract.
- `pending_ingestion` is a distinct MediaAsset lifecycle state, separate from a merchant's pending upload.
- `message_attachments` is a dedicated relation to `communication_messages` and `media_assets`, both guarded by composite `(business_id, id)` foreign keys.
- `ConversationMediaIngestionRepository` creates tenant-scoped pending assets for image/video/audio/document/file and promotes verified image metadata only.
- `MessageAttachmentRepository` creates/lists message attachment relations and marks them ready only after the backing MediaAsset is ready.
- The S3 adapter permits the isolated `conversation/` object namespace alongside `catalog/`; the Catalog feature flag remains independent from `CONVERSATION_MEDIA_ENABLED`. When Conversation Media is enabled, both API and worker startup validate the selected private-storage provider and require `SOCIALAPI_API_KEY`; when disabled, inbound attachments do not create pending database rows.
- SocialAPI `content.media[]` is normalized for inbound events. The temporary URL is validated and discarded; only an opaque SocialAPI interaction-id/attachment-index reference crosses into the domain.
- `InboundMediaSourceResolver` re-queries authenticated conversation history using cursor pagination and resolves the interaction to its current attachment source. SocialAPI's documented message-history contract exposes one attachment per message, so only position zero is currently resolvable. For WhatsApp and Telegram, the resolver recognizes SocialAPI's `/v1/inbox/media/{id}` URL and returns only the opaque media ID; other platforms return their platform-hosted HTTPS URL.
- `ProviderInboundStore.Materialize` creates the pending MediaAsset and MessageAttachment rows in the same PostgreSQL transaction as the inbound communication message.

## Important boundary

- `catalog_media_links` continues to serve only CatalogItem/Variant galleries.
- A Catalog-purpose asset cannot be attached to a conversation.
- The current verified-to-ready path remains image-only. Video/audio/document/file can be represented as pending placeholders, but are not yet downloaded, verified, made ready, or delivered.
- Source provider media references are opaque identifiers only. URL values are rejected before persistence and are never copied into a message DTO.
- A lease-based claim/retry queue is present. It currently claims only the first WhatsApp image attachment per message; Facebook/Instagram URL retrieval and video/audio/document/file verification are deliberately left queued for later channel-specific implementations.
- The worker downloads WhatsApp media through SocialAPI's authenticated `GET /v1/inbox/media/{id}` endpoint, refuses redirects, enforces a 10 MiB body bound, verifies JPEG/PNG bytes and image dimensions, and writes to the provider recorded on the MediaAsset. It never fetches a webhook-supplied URL.
- Provider 504/503, transport failures, and object-storage write failures use bounded retry scheduling; permanent provider responses, exhausted retry budgets, unsupported media sources, and invalid image bytes are terminal. Terminal failure now changes the attachment and its pending MediaAsset in one SQL statement and clears the opaque provider reference.

## Atomicity and operations

Inbound normalization stores only an opaque SocialAPI message reference and attachment position. It never persists a temporary media URL. The `MediaAsset` placeholder and `MessageAttachment` relation are created in the same transaction as the inbound message; a failure rolls the entire materialization back.

The new repositories use the transaction executor from the caller context. They perform only local SQL; object storage and SocialAPI calls must remain outside SQL transactions. The worker sequence is: claim a lease, resolve the opaque reference through authenticated provider history, download provider bytes outside a transaction, verify bytes, write private storage to the recorded provider, then mark the MediaAsset and MessageAttachment ready through short local SQL operations. If the asset becomes ready but the relation update fails, the queue can reclaim the relation and converge without downloading again.

## Verification added

The PostgreSQL integration suite now checks:
- inbound materialization creates image and file pending placeholders with the message,
- pending conversation asset creation and attachment linkage,
- tenant isolation between the message and its media,
- refusing to mark the relation ready while the asset is pending,
- successful transition after verified image metadata is recorded,
- terminal attachment failure atomically rejects its pending MediaAsset,
- rejection of Catalog-purpose assets from Conversation.

Unit and integration tests cover image verification, retry/state transitions, and the PostgreSQL lease queue. CI success does not prove live SocialAPI retrieval or real R2/Backblaze object writes; no live provider or storage operation has been performed as part of this slice. The public upload endpoint and outbound attachment transport remain out of scope.


## واجهة قراءة مرفقات المحادثة

- إسقاط الرسائل يجلب مرفقات الرسائل العامة بدفعة واحدة، ولا يضمّن مفاتيح التخزين أو مراجع SocialAPI.
- المرفق الجاهز يعرض `attachment_id` و`media_id` والنوع والحالة والتسمية، مع مسار قراءة خاص بالمحادثة والرسالة والمرفق. المرفقات المعلقة أو الفاشلة لا تحصل على مسار قراءة.
- `GET /api/v1/businesses/{business_id}/conversations/{conversation_id}/messages/{message_id}/attachments/{attachment_id}/read-url` يتحقق من صلاحية قراءة المحادثة، ثم يتحقق في PostgreSQL من تطابق المستأجر والمحادثة والرسالة والمرفق وأن الرسالة عامة وأن الأصل من غرض `conversation_attachment` وحالته `ready`.
- رابط التخزين الموقّع قصير العمر (5 دقائق)، ويُنشأ من مزود التخزين المسجل على `MediaAsset` نفسه؛ لا يحدث fallback إلى bucket آخر عند تغيير مزود الكتابة الافتراضي.

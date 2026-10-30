# Catalog Media MVP — Backend Implementation Contract

Status: **in progress; draft PR, do not merge or deploy yet**.

## Scope

The merchant uploads JPEG/PNG/WebP product photos, associates approved photos with
catalog items and variants, and the existing DeepSeek `evaluate_full_catalog`
selects item/variant/offer from customer **text**. After validated selection the
media resolver obtains the primary image and SocialAPI sends it with the reply.

Out of scope: DINOv2, ONNX, pgvector, `media_embeddings`,
`media_processing_jobs`, smart links, source attribution, Go intent engines,
Go product text search, and changes to conversation summary/focus.

## Domain boundaries

- `media_assets`: immutable storage key (private R2), staged/ready/rejected/deleted lifecycle,
  verified server-side image MIME, dimensions, byte length, SHA-256.
- `catalog_media_links`: one item gallery (variant=NULL) or variant gallery.
  Composite foreign keys prevent cross-business or cross-product attachment.
- Images are attached to items/variants, **never directly to offers**.
- Merchant gallery mutations hold `catalog_items` row lock for the duration of
  a DB transaction. Partial unique indexes prevent multiple active primaries.
  First attached image becomes primary; deleting primary elects a successor.
- No child gallery auto-fallback to another variant's colour image.
- Active images must be approved. One active primary per nonempty gallery is
  enforced by application service, with database indexes enforcing at most one.

## Milestones / release gates

### M0 — schema / application ports
- Migration 000083 with up/down and tenant-safe constraints.
- `ports.CatalogMediaRepository` and PostgreSQL adapter.
- Service/gallery locking + unit and PostgreSQL integration checks.

### M1 — actual R2 media upload
- Merchant-authenticated, `catalog:write` protected staged media upload.
- Prefer direct staged presigned PUT, **never treat a browser upload as verified**.
- Verify stored bytes (content signature, size, SHA-256, dimensions), then atomically
  promote to immutable storage key before status `ready`. A presigned PUT
  URL is reusable until expiry and must **not** be able to overwrite approved bytes.
- Enforce <=10MiB, valid supported image type, pixel limits, file ownership,
  short-lived URLs, CORS allowlist, cleanup of abandoned uploads.
- Item-create transaction must atomically persist item, initial offer, image
  links. Existing item/variant gallery management must use version checks.
- Ensure no DB transaction spans external R2 calls.

### M2 — outbound AutoReply media
- Keep DeepSeek `evaluate_full_catalog` and current validated `selected[]`.
- Extend strict customer-sales response schema with **optional** media request,
  referencing only the already selected validated item/variant.
- Resolve primary link after validation and re-check at dispatch time.
- Versioned `outbound_messages.content_reference` and message `mixed` type.
- Extend `OutboundDeliveryResolver`, `SendMessageCommand`, and SocialAPI
  adapter; prove live attachment payload and channel-specific semantics.
- Distinguish accepted/sent/delivered and partial text/image failures.

### M3 — production proof
- CI: gofmt, Go packages, vet, OpenAPI generated parity, PostgreSQL migrations.
- Real R2 upload and SocialAPI sandbox tests; tenant isolation, races,
  deleted/archived products, expired staged tokens, failed media gracefully
  falling back to text, idempotent retries.
- Feature flag off by default until media end-to-end is proven.

## Files in first implementation slice

- `migrations/000083_catalog_media_mvp.up.sql`
- `migrations/000083_catalog_media_mvp.down.sql`
- `internal/application/ports/catalog_media.go`
- `internal/application/services/catalog_media_service.go`
- `internal/application/services/catalog_media_service_test.go`
- `internal/adapters/secondary/persistence/postgres/catalog_media_repository.go`

The above are a **foundation**, not the complete media feature; do not enable
uploads or image replies until M1/M2 wiring, integration tests and rollout gates pass.

## Implemented HTTP API (draft, feature gated)

All operations are protected by bearer authentication, merchant membership,
and `catalog:read` / `catalog:write` permissions. No business ID can be
supplied by the model. Base path: `/api/v1/businesses/{business_id}`.

| Method | Path | Permission |
|---|---|---|
| POST | `/media/uploads` | `catalog:write` |
| GET | `/media/{media_id}/read-url` | `catalog:read` |
| GET | `/catalog-items/{item_id}/media?variant_id=` | `catalog:read` |
| POST | `/catalog-items/{item_id}/media?variant_id=` | `catalog:write` |
| POST | `/catalog-items/{item_id}/media/{link_id}/primary?variant_id=` | `catalog:write` |
| DELETE | `/catalog-items/{item_id}/media/{link_id}?variant_id=` | `catalog:write` |

Upload request currently uses `{"data_base64":"..."}`. Go applies a 15 MiB
request-body limit, decodes no more than 10 MiB, validates full JPEG/PNG
stream and limits pixel count. WebP is **not** yet accepted by this endpoint.
Uploaded image bytes are stored privately via R2 SigV4 and object MIME metadata.
Merchant preview URLs expire after 10 minutes.

### Required environment (API and worker)

```text
CATALOG_MEDIA_ENABLED=false
R2_ACCOUNT_ID=
R2_BUCKET=
R2_ACCESS_KEY_ID=
R2_SECRET_ACCESS_KEY=
```

All R2 credentials belong only in server-side secrets. Never expose them to
React. Both API and worker must receive consistent config. `CATALOG_MEDIA_ENABLED`
is **false by default** and should remain false until release gates pass.

### Reply flow

- The existing DeepSeek `evaluate_full_catalog` remains unchanged.
- If the terminal informational reply contains exactly **one validated**
  selected item/variant, Go may resolve that gallery's primary approved photo.
- The outbound message stores both `media_id` and exact `media_link_id`
  in the versioned inline-message envelope, not a presigned URL.
- At Outbox dispatch Go checks that exact link is still active, primary and
  approved and the product/catalog/variant and underlying asset remain active.
- Only then it issues a 45-minute signed R2 read URL and sends
  `text + attachments[0]` in one SocialAPI REST call.
- If the image has been removed or R2 isn't configured, text still sends.
- A SocialAPI error after sending a combined message has an unknown outcome:
  do not naively retry another text-only send and risk duplicates.

### Unverified release gates

- **No live R2 credentials have been supplied for an upload and download test.**
- **No live SocialAPI account/provider media delivery has been verified.**
- A live channel-specific WhatsApp/Instagram/Facebook media test is required.
- Upload currently uses base64 JSON, not an efficient streaming binary API;
  a future raw-upload transport may be needed for high throughput.
- End-to-end atomic item + first offer + staged media links at item creation
  remains a separate integration item.
- Gallery optimistic `If-Match` must be fully wired before concurrent
  merchant-editor operations are considered production-ready.
- Pending R2 upload cleanup and unused-asset retention need a reconciler.
- Media UI is a frontend task and not part of this backend-only branch.

Do **not** deploy this draft PR to production based on CI success alone.


## Running the implemented Media MVP (private R2; API and Worker)

Media remains disabled unless `CATALOG_MEDIA_ENABLED=true` is set.
Configure **both the Go API and Go Worker** with the same settings:

```text
CATALOG_MEDIA_ENABLED=true
R2_ACCOUNT_ID=<Cloudflare-account-ID>
R2_BUCKET=<private-bucket-name>
R2_ACCESS_KEY_ID=<R2-object-access-key>
R2_SECRET_ACCESS_KEY=<R2-object-secret>
```

Provision a *private* bucket. Never put R2 secrets into the React build,
repository, CI logs or returned JSON. Scope the R2 API token to the required
bucket and operations; store credentials as server-side deployment secrets.

The first Backend MVP upload endpoint is an authenticated JSON/base64 POST,
`/api/v1/businesses/{business_id}/media/uploads`, with a 10 MiB decoded
limit and server-side JPEG/PNG decoding; WebP is not accepted by this first
implementation. The API replies with an opaque `media_id`. Merchant gallery
operations use `/api/v1/businesses/{business_id}/catalog-items/{item_id}/media`
with an optional `variant_id`; product creation accepts up to twelve
`initial_media_ids` and links them in the commercial DB transaction.

`GET /api/v1/businesses/{business_id}/media/{media_id}/read-url` returns
a short-lived signed GET URL. Signed links are bearer credentials; do not log,
cache publicly or persist them in the database. Media is not public by default.

Outbound AutoReply continues to use the validated DeepSeek
`evaluate_full_catalog` result. On a single selected catalog item it can
resolve a matching approved primary link, save `media_id` and `media_link_id`
with the outbound text and resolve a fresh R2 URL at worker dispatch.
Media is optional: unavailable/invalidated links fall back to text. The current
SocialAPI adapter serializes one image under `attachments` using the
documented `{type:"image", source_type:"url", source:"https://..."}` shape.
Do not claim a customer received the photo until provider delivery events show
it; provider acceptance is not delivery.

**Rollout criteria**: run real R2 PUT + presigned GET against the configured
bucket and a real SocialAPI channel in a non-production environment, verify
photo retrieval after worker delay, old/removed links, tenant isolation,
partial media failures, retry/idempotency and outbound delivery status.
No such provider-account live test is represented by local HTTP mocks.
Do not merge or deploy the feature as complete solely on a green GitHub build.

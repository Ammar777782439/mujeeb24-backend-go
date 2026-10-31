# Catalog Media MVP — Backend Implementation Contract

Status: **in progress; draft PR, do not merge or deploy yet**.

## Scope

The merchant uploads JPEG/PNG product photos (WebP is deferred), associates approved photos with
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

Upload request currently uses `{"data_base64":"..."}`. Go applies a 14 MiB
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

## Multi-photo extension (integration branch only; not production-approved)

The old one-photo limit remains **per SocialAPI REST request**, not per
customer turn. After the validated terminal informational decision, a
separate strict `choose_reply_media` model tool may select zero to four
exact approved link IDs from the already selected item/variant galleries.
Go does not classify photo intent or rematch product text. A variant gallery
must match the validated variant exactly, avoiding a wrong-colour photo.

The first selected image accompanies the model-written customer text.
Additional photos are stored as independent `outbound_messages` +
Outbox entries with deterministic per-photo dedupe keys, one image
attachment per request, and independent provider IDs/statuses. Additional
outbox entries have staggered availability (ordering is best-effort, not
a cross-provider ordering guarantee). All obligations are committed
atomically; one customer turn still counts as **one AI reply**. The
`media_required` envelope flag forbids a caption-only follow-up when
its image becomes invalid between queueing and dispatch. The original
text reply may still fall back to text-only.

Remaining release gates: real channel-by-channel SocialAPI media tests,
live R2 tests, image delivery-status checks, partial failure recovery,
provider order semantics and confirmation of customer-facing display.
Do **not** flip the feature flag on in production just because code or
unit tests compile.

## Backblaze B2 private image storage (integration branch, not production-approved)

Catalog images can use either Cloudflare R2 or Backblaze B2 over S3 SigV4.
The original `R2_*` environment remains unchanged. Unless explicitly
selected, new uploads continue to use R2.

To choose **Backblaze for new uploads** on BOTH API and Worker:

```env
CATALOG_MEDIA_ENABLED=true
S3_PROVIDER=backblaze
S3_BUCKET=mujeeb24-images
S3_REGION=us-west-004
S3_ENDPOINT=https://s3.us-west-004.backblazeb2.com
S3_ACCESS_KEY_ID=<B2 application key ID>
S3_SECRET_ACCESS_KEY=<B2 application key secret>
```

Use the actual region and S3 endpoint shown in your private Backblaze bucket.
No Cloudflare R2 account ID is accepted as a Backblaze identifier.

If media already exists in R2, **keep R2_ACCOUNT_ID, R2_BUCKET,
R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY configured on both processes**:
the persisted `media_assets.storage_provider` selects the original
provider when a previously uploaded image is read or dispatched.
Changing the default upload target does **not** migrate historical objects
or automatically create copies. Misconfigured/unavailable historical
providers fail closed rather than silently reading a similarly named key
from another bucket.

`migrations/000084_catalog_media_backblaze.up.sql` permits `backblaze`
in the existing provider check constraint. Do not bypass migrations.
Existing R2 assets keep their provider label and storage key; rolling back
is forbidden while Backblaze records exist.

The existing upload path keeps server-side JPEG/PNG validation,
the same 10 MiB limit, catalog ownership/variant approval, private object
storage and 45-minute signed URLs at worker dispatch. B2 endpoint values
must exactly match `https://s3.<S3_REGION>.backblazeb2.com`; arbitrary
IP/private/custom endpoints are rejected. Credentials and signed URLs must
not be put into GitHub, React, or logs.

New images have `storage_provider=backblaze` in PostgreSQL. The API upload
service and Outbox worker must deploy from the **same branch revision**, and
run with the same storage credentials. This change is not live-provider
validation. Test S3 Put/Get/authorization on B2 and actual SocialAPI delivery
before switching a production tenant.

## Official AWS SDK S3 transport (integration branch)

The previous hand-written S3 SigV4/HTTP client has been removed.
Both Backblaze B2 and Cloudflare R2 now use the **official AWS SDK for Go v2**:
`s3.PutObject`, `s3.DeleteObject` and `s3.NewPresignClient(...).PresignGetObject`.
The existing `ports.MediaObjectStorage` contract and storage-provider routing
are preserved, as are `R2_*` and `S3_*` environment variable names.

The SDK is pinned to compatible releases for this repository's Go 1.22 CI:
`github.com/aws/aws-sdk-go-v2/service/s3 v1.71.1`, predating the
default CRC32 upload checksum behavior introduced in v1.73.0.
Keep it pinned while the target stores are being verified; upgrading to
newer S3 clients requires a checksum-compatibility regression test.
Use the configured Backblaze region and endpoint, path-style addressing,
private buckets, static credentials, 10 MiB upload limits and bounded
presigned GET TTLs. Never expose signed URLs or secret credentials in logs.

CI tests verify SDK-generated requests, full-body transport to an in-process
HTTP server, bounded signed URL generation, and structured error redaction.
This does **not** demonstrate successful live upload to a customer's B2
bucket: external provider integration needs independent operational proof.

## Opt-in Backblaze authentication diagnostics (safe and read-only)

When an upload fails with `SignatureDoesNotMatch`, distinguish a wrong
key pair from a S3 request-signing problem using two independent probes.

Temporarily set `CATALOG_MEDIA_B2_DIAGNOSTICS=true` in Render for the
API service and redeploy. The default is **off**. No new storage keys are
required; the probes read the existing `S3_ACCESS_KEY_ID`,
`S3_SECRET_ACCESS_KEY`, `S3_BUCKET` and `S3_ENDPOINT` only in memory.

1. `stage=b2_native_key_check` calls the official Backblaze v4
   `b2_authorize_account` HTTPS endpoint with Basic authentication.
   `result=invalid_key_pair` means Backblaze rejected the current
   application key ID / application key combination.
   `result=key_pair_valid` means those credentials are valid, and
   `endpoint_match`, `bucket_scope`, `write_files` show the
   returned S3 URL and scoped permissions.
2. `stage=b2_s3_readonly_check` signs a `GetBucketLocation` request
   using the very same AWS SDK Go v2 configuration as the upload.
   Success demonstrates that S3 can authenticate a simple signed read.
   Failure with `SignatureDoesNotMatch` isolates an S3 configuration /
   request signing problem separate from the binary PUT payload.

These probes make no object writes or deletions. Only sanitized status
labels and error codes are logged. The response's bearer authorization
token, raw response JSON/XML, URLs with signatures, key ID and secret
are **never** logged. Probe errors do not stop the API or Worker.
Disable the diagnostic flag and redeploy when complete.

Do not conclude that an S3 signing bug is proven solely because a PUT
fails; credentials, key scope, HTTP proxy/request modification, or
provider-specific signature rules must be distinguished.

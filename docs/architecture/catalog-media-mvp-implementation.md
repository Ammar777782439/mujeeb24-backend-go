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

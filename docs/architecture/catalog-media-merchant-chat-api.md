# Catalog media in merchant chat — Backend API contract

Target branch: `integration/catalog-media-main`. No frontend changes are part of this work.

## Message list

Authenticated `GET /api/v1/businesses/{business_id}/conversations/{conversation_id}/messages`
preserves the existing `text`, `status`, and pagination fields. An AI
message with a selected, recorded catalog image now additionally includes:

```json
{
  "content_type": "mixed",
  "attachments": [
    {
      "type": "image",
      "media_id": "<UUID>",
      "preview_path": "/api/v1/businesses/<UUID>/media/<UUID>/preview"
    }
  ],
  "status": "failed"
}
```

This is a shape example, not a live message. `attachments` is omitted when
there is no recorded image. `status=failed` is exposed when its outbound
Outbox entry became `dead_letter`; **no provider name, error code, signed
storage URL, or authentication token is returned**. This is a local delivery
status, not proof that a remote customer read the message. An image can be
selected and recorded even when the provider refused to send it.

For `status=accepted`, the external system has accepted the request but no
delivery/read confirmation is implied. Other existing non-error status values
are unchanged.

## Private image preview

Authenticated `GET /api/v1/businesses/{business_id}/media/{media_id}/preview`
verifies the caller's business scope and that the media asset is ready and
not deleted. The API fetches bytes directly through the official AWS SDK
GetObject, routed to the asset's recorded R2/Backblaze provider, without
creating a second presigned HTTP request. It validates JPEG/PNG image
content, size and persisted checksum (when present), with a 10 MiB maximum,
and replies with:

```json
{"data":{"data_base64":"<base64-JPEG-or-PNG>","mime_type":"image/jpeg","width":640,"height":480}}
```

The response is `Cache-Control: private, no-store`. Clients must not
persist the base64 data beyond their authorized UI session. This JSON preview
endpoint deliberately avoids exposing storage hosts or credentials and is
not an optimized streaming/thumbnail service; consumers should fetch images
on demand rather than for every message in every paginated conversation.
A later thumbnail proxy could reduce bandwidth.

The existing media `read-url` endpoint remains for catalog-management
backwards compatibility; merchant chat should use `preview_path`.

## AI media contract

An initial grounded terminal answer selects real item/variant references.
The post-validation `choose_reply_media` model tool now returns both
`media_link_ids` and `reply_text`. When valid photos were selected, the
final stored answer uses the post-selection reply, so it can refer to those
photos without making a delivery claim. The model never controls sending;
Go verifies linked product and variant, persists Outbox obligations, and
the Worker separately attempts provider delivery.

The backend still cannot guarantee model wording is perfect; observe real
messages and evaluate text/image consistency with regression tests. It
does not turn provider errors into model-generated customer claims.

## Debugging preview failures

Look for `[CatalogMedia] stage=preview_lookup_failed`,
`stage=preview_read_failed`, or `stage=preview_ready` in backend logs,
filtering by the image's `media_id`. The logs contain **no** provider URL,
storage credentials, or file bytes. If these stages never occur, inspect the
HTTP status in the browser Network tab: authorization (401/403), route
deployment, or JSON decoding may be responsible.

`preview_path` points to an authenticated JSON endpoint, **not** a raw
image URL. The frontend must call it with its normal Bearer authorization,
read `data.data_base64` and `data.mime_type`, and render a data URL
(`data:image/jpeg;base64,...`), or an equivalent blob URL. Putting
`preview_path` straight into an HTML `img.src` will not work because
the endpoint returns JSON and browsers cannot attach the Bearer header to
an ordinary image navigation. No customer/provider account information
is ever sent to the merchant.

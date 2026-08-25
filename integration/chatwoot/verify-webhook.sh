#!/usr/bin/env bash
set -euo pipefail

fixture_path="${CHATWOOT_WEBHOOK_FIXTURE:-$(dirname "$0")/fixtures/inbound-webhook.json}"
endpoint="${MUJEEB_CHATWOOT_WEBHOOK_URL:-http://127.0.0.1:8080/api/v1/webhooks/chatwoot/chatwoot-test}"
secret="${CHATWOOT_WEBHOOK_SECRET:?CHATWOOT_WEBHOOK_SECRET must be provided by the local runtime environment}"
timestamp="${CHATWOOT_WEBHOOK_TIMESTAMP:-$(date +%s)}"

sign_fixture() {
  {
    printf '%s.' "$timestamp"
    cat "$fixture_path"
  } | openssl dgst -sha256 -hmac "$secret" -hex | awk '{print "sha256=" $2}'
}

valid_signature="$(sign_fixture)"
invalid_signature="${valid_signature%?}0"
valid_status="$(curl -sS -o /dev/null -w '%{http_code}' -X POST \
  -H "Content-Type: application/json" \
  -H "X-Chatwoot-Timestamp: $timestamp" \
  -H "X-Chatwoot-Signature: $valid_signature" \
  --data-binary "@$fixture_path" "$endpoint")"
invalid_status="$(curl -sS -o /dev/null -w '%{http_code}' -X POST \
  -H "Content-Type: application/json" \
  -H "X-Chatwoot-Timestamp: $timestamp" \
  -H "X-Chatwoot-Signature: $invalid_signature" \
  --data-binary "@$fixture_path" "$endpoint")"

printf 'chatwoot_webhook_valid_status=%s\n' "$valid_status"
printf 'chatwoot_webhook_invalid_status=%s\n' "$invalid_status"
[[ "$valid_status" == "202" ]]
[[ "$invalid_status" == "401" ]]

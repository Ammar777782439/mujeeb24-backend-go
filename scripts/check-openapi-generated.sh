#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
export PATH="/usr/local/go/bin:${PATH}"

tracked="api/openapi/mujeeb24-dashboard-v1.generated.yaml"
tmp="$(mktemp --suffix=.yaml)"
trap 'rm -f "$tmp"' EXIT

if [[ ! -f "$tracked" ]]; then
  echo "missing generated OpenAPI artifact: $tracked" >&2
  exit 1
fi

GOTOOLCHAIN=local go run ./cmd/openapi-gen -output "$tmp"
if ! cmp -s "$tracked" "$tmp"; then
  echo "OpenAPI drift detected: run scripts/generate-openapi.sh and commit the generated artifact" >&2
  diff -u "$tracked" "$tmp" | head -200 || true
  exit 1
fi

echo "openapi_drift_check=PASS"

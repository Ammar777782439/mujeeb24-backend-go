#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
export PATH="/usr/local/go/bin:${PATH}"

output="${1:-api/openapi/mujeeb24-dashboard-v1.generated.yaml}"
GOTOOLCHAIN=local go run ./cmd/openapi-gen -output "$output"

#!/usr/bin/env bash
# Safety contract: no external calls; all checks are deterministic and local.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

go_bin="${GO_BIN:-go}"
gofmt_bin="${GOFMT_BIN:-gofmt}"
export GOTOOLCHAIN="${GOTOOLCHAIN:-local}"

fail() {
  printf 'VERIFY FAILED: %s\n' "$*" >&2
  exit 1
}

printf '%s\n' '[1/6] checking forbidden temporary tools'
for file in generate-keys.go testjwt.go; do
  [[ ! -e "$file" ]] || fail "forbidden temporary tool exists: $file"
done

printf '%s\n' '[2/6] scanning tracked files for likely private keys or SocialAPI keys'
if git grep -I -E --quiet -e 'sapi_key_[[:alnum:]_]{20,}' -e '-----BEGIN (EC |RSA |OPENSSH )?PRIVATE KEY-----'; then
  fail 'tracked files contain a likely secret; remove and rotate it before merging'
fi

printf '%s\n' '[3/6] checking Go formatting and Git whitespace'
go_files=()

if [[ "${VERIFY_GOFMT_SCOPE:-all}" == "changed" ]]; then
  base_ref=""
  if [[ "${GITHUB_EVENT_NAME:-}" == "pull_request" && -n "${GITHUB_BASE_REF:-}" ]]; then
    base_ref="$(git merge-base HEAD "origin/${GITHUB_BASE_REF}")"
  elif [[ -n "${GITHUB_BEFORE:-}" && "${GITHUB_BEFORE}" != "0000000000000000000000000000000000000000" ]]; then
    base_ref="${GITHUB_BEFORE}"
  elif git rev-parse --verify HEAD^ >/dev/null 2>&1; then
    base_ref="$(git rev-parse HEAD^)"
  fi

  if [[ -n "$base_ref" ]]; then
    while IFS= read -r file; do
      [[ -f "$file" ]] && go_files+=("$file")
    done < <(git diff --name-only "$base_ref" HEAD -- '*.go')
  fi
else
  while IFS= read -r file; do
    [[ -f "$file" ]] && go_files+=("$file")
  done < <(git ls-files '*.go')
fi

if (( ${#go_files[@]} > 0 )); then
  mapfile -t unformatted < <("$gofmt_bin" -l "${go_files[@]}")
  (( ${#unformatted[@]} == 0 )) || fail "gofmt required: ${unformatted[*]}"
else
  printf '%s\n' 'no Go files in formatting scope'
fi
git diff --check
printf '%s\n' '[4/6] validating generated OpenAPI contract'
before="$(mktemp)"
trap 'rm -f "$before"' EXIT
cp api/openapi/mujeeb24-dashboard-v1.generated.yaml "$before"
openapi_scope="${VERIFY_OPENAPI_SCOPE:-all}"
check_openapi=true

if [[ "$openapi_scope" == "changed" ]]; then
  base_ref=""
  if [[ "${GITHUB_EVENT_NAME:-}" == "pull_request" && -n "${GITHUB_BASE_REF:-}" ]]; then
    base_ref="$(git merge-base HEAD "origin/${GITHUB_BASE_REF}")"
  elif [[ -n "${GITHUB_BEFORE:-}" && "${GITHUB_BEFORE}" != "0000000000000000000000000000000000000000" ]]; then
    base_ref="${GITHUB_BEFORE}"
  elif git rev-parse --verify HEAD^ >/dev/null 2>&1; then
    base_ref="$(git rev-parse HEAD^)"
  fi
  if [[ -n "$base_ref" ]]; then
    if ! git diff --name-only "$base_ref" HEAD -- 'internal/adapters/primary/http/**' 'cmd/openapi-gen/**' | grep -q .; then
      check_openapi=false
    fi
  fi
fi

if [[ "$check_openapi" == "true" ]]; then
  "$go_bin" run ./cmd/openapi-gen
  cmp -s "$before" api/openapi/mujeeb24-dashboard-v1.generated.yaml || fail 'generated OpenAPI drift; regenerate and commit the contract intentionally'
else
  printf '%s\n' 'OpenAPI check skipped: no API-contract source files changed'
fi
printf '%s\n' '[5/6] running unit and package checks'
"$go_bin" test ./...
"$go_bin" vet ./...

printf '%s\n' '[6/6] optional PostgreSQL 16 integration checks'
if [[ "${RUN_INTEGRATION:-0}" == "1" ]]; then
  [[ -n "${POSTGRES_TEST_DSN:-}" ]] || fail 'RUN_INTEGRATION=1 requires POSTGRES_TEST_DSN'
  "$go_bin" test -tags=integration ./internal/adapters/secondary/persistence/postgres
else
  printf '%s\n' 'integration checks skipped; set RUN_INTEGRATION=1 with POSTGRES_TEST_DSN to run them'
fi

printf '%s\n' 'VERIFY PASSED'

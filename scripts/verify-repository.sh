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
while IFS= read -r file; do
  [[ -f "$file" ]] && go_files+=("$file")
done < <(git ls-files '*.go')
mapfile -t unformatted < <("$gofmt_bin" -l "${go_files[@]}")
(( ${#unformatted[@]} == 0 )) || fail "gofmt required: ${unformatted[*]}"
git diff --check

printf '%s\n' '[4/6] validating generated OpenAPI contract'
before="$(mktemp)"
trap 'rm -f "$before"' EXIT
cp api/openapi/mujeeb24-dashboard-v1.generated.yaml "$before"
"$go_bin" run ./cmd/openapi-gen
cmp -s "$before" api/openapi/mujeeb24-dashboard-v1.generated.yaml || fail 'generated OpenAPI drift; regenerate and commit the contract intentionally'

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

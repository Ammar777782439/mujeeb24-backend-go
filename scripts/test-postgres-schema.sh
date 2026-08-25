#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONTAINER="mujeeb24-postgres-schema-test-$$"
PORT="${POSTGRES_TEST_PORT:-55434}"
IMAGE="${POSTGRES_TEST_IMAGE:-postgres:16-alpine}"
FOUNDATION_DB="mujeeb24_foundation_test"
FULL_SCHEMA_DB="mujeeb24_full_schema_test"
RUNNER_DB="mujeeb24_runner_test"

if docker info >/dev/null 2>&1; then
    DOCKER=(docker)
elif sudo docker info >/dev/null 2>&1; then
    DOCKER=(sudo docker)
else
    echo "docker is unavailable or permission was denied" >&2
    exit 1
fi

cleanup() {
    "${DOCKER[@]}" rm -f "$CONTAINER" >/dev/null 2>&1 || true
}
trap cleanup EXIT

cd "$ROOT_DIR"
"${DOCKER[@]}" rm -f "$CONTAINER" >/dev/null 2>&1 || true
"${DOCKER[@]}" run --network host --rm -d --name "$CONTAINER" \
    -e POSTGRES_PASSWORD=testpassword \
    -e POSTGRES_DB=postgres \
    "$IMAGE" -c "port=${PORT}" >/dev/null

ready=0
for _ in $(seq 1 180); do
    if "${DOCKER[@]}" exec "$CONTAINER" psql -p "$PORT" -U postgres -d postgres -Atqc 'SELECT 1' >/dev/null 2>&1; then
        ready=1
        break
    fi
    sleep 1
done
if [[ "$ready" -ne 1 ]]; then
    echo "postgres did not become ready" >&2
    "${DOCKER[@]}" logs "$CONTAINER" >&2 || true
    exit 1
fi
sleep 2
"${DOCKER[@]}" exec "$CONTAINER" psql -p "$PORT" -U postgres -d postgres -Atqc 'SELECT 1' >/dev/null

for db in "$FOUNDATION_DB" "$FULL_SCHEMA_DB" "$RUNNER_DB"; do
    "${DOCKER[@]}" exec "$CONTAINER" createdb -p "$PORT" -U postgres "$db"
done

cat migrations/0000*.up.sql | "${DOCKER[@]}" exec -i "$CONTAINER" \
    psql -p "$PORT" -v ON_ERROR_STOP=1 -U postgres -d "$FOUNDATION_DB" >/dev/null
"${DOCKER[@]}" exec -i "$CONTAINER" psql -p "$PORT" -v ON_ERROR_STOP=1 -U postgres -d "$FOUNDATION_DB" \
    < tests/integration/foundation_constraints.sql >/dev/null

cat migrations/0000*.up.sql | "${DOCKER[@]}" exec -i "$CONTAINER" \
    psql -p "$PORT" -v ON_ERROR_STOP=1 -U postgres -d "$FULL_SCHEMA_DB" >/dev/null
"${DOCKER[@]}" exec -i "$CONTAINER" psql -p "$PORT" -v ON_ERROR_STOP=1 -U postgres -d "$FULL_SCHEMA_DB" \
    < tests/integration/full_schema_constraints.sql >/dev/null

GOTOOLCHAIN=local go build -o /tmp/mujeeb24-migrate-schema-test ./cmd/migrate
DATABASE_URL="postgres://postgres:testpassword@127.0.0.1:${PORT}/${RUNNER_DB}?sslmode=disable" \
    /tmp/mujeeb24-migrate-schema-test >/tmp/mujeeb24-migrate-schema-test-1.log 2>&1
DATABASE_URL="postgres://postgres:testpassword@127.0.0.1:${PORT}/${RUNNER_DB}?sslmode=disable" \
    /tmp/mujeeb24-migrate-schema-test >/tmp/mujeeb24-migrate-schema-test-2.log 2>&1

grep -q 'applied=37' /tmp/mujeeb24-migrate-schema-test-1.log
grep -q 'applied=0' /tmp/mujeeb24-migrate-schema-test-2.log
[[ "$(${DOCKER[@]} exec "$CONTAINER" psql -p "$PORT" -U postgres -d "$RUNNER_DB" -Atqc 'SELECT count(*) FROM schema_migrations')" == "37" ]]

echo 'postgres_schema_validation=passed'
echo 'foundation_constraints=passed'
echo 'full_schema_constraints=passed'
echo 'migration_runner_first=applied-37'
echo 'migration_runner_second=applied-0'

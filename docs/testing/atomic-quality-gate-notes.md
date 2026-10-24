# Atomic branch — Quality gate reconciliation

Target branch: `feat/catalog-complete-commercial-entity-atomic`. This note does not change product runtime behavior.

## OpenAPI baseline

The canonical generator is `go run ./cmd/openapi-gen` with Go `1.22.2`, as pinned by `go.mod`, the Docker build and GitHub Actions. The previous `api/openapi/mujeeb24-dashboard-v1.generated.yaml` included two extra `Cookie` properties, `Quoted` and `Partitioned`, and listed both as required. Regenerating under the pinned toolchain removes exactly these two property declarations and two required entries (six removed YAML lines in total). The change did **not** remove merchant dashboard endpoints or catalog/sales business fields.

Verification: `bash scripts/check-openapi-generated.sh`; `scripts/verify-repository.sh` delegates to the same read-only check and prints the file-level diff if regeneration changes the contract. Do not regenerate with a different local Go standard library and commit unnoticed drift.

## Integration fixture for team invitation acceptance

Accepting an employee invitation requires an active subscription with a free team seat. The PostgreSQL integration fixture now explicitly asserts that an unsubscribed merchant cannot accept a new-account invitation, then creates and activates a Pricing V2 Growth subscription with three seats (owner plus two accepted agents). The seat-entitlement production guard remains enforced. Fixture teardown deletes the subscription before the business.

## Native providers are tested separately

The source Quality gate verifies Go/OpenAPI/PostgreSQL without reaching Gemini or DeepSeek. Live provider qualification, billable test requests, secret rotation and outbound message permissions require an explicit operator-approved procedure. A successful mock 503 fallback does not prove upstream compatibility.

## Channel activation and global uniqueness integration fixtures

A successful OAuth callback is not a subscription entitlement. PostgreSQL channel activation requires a current ACTIVE subscription with an unused included channel slot (plus any paid channel add-ons). Channel provisioning integration first asserts activation fails without a subscription, then creates and activates a Pricing V2 Basic subscription for the business. Provider-account uniqueness tests seed independent active Basic subscriptions for both businesses so the test reaches the global uniqueness constraint instead of failing earlier at billing. No production channel guard was removed; subscriptions are deleted before fixture businesses in teardown.

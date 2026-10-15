# Mujeeb 24 — Complete Commercial Entity / Atomic AI Evaluation v2

Status: **Proposed for acceptance on feat/catalog-complete-commercial-entity-atomic**.
Supersedes the conflicting transport details in ① §3 and ② §§2–4, 6–7; it does not replace the Universal Catalog domain, merchant data, or the ⑤ entity/data boundary.
Source branch: `feat/ai-litellm-multi-provider-gateway`.
No changes to backend `main`.

## Non-negotiable invariant

**An active CatalogItem is presented to any model performing commercial evaluation as one complete, indivisible unit containing its original commercial facts, every eligible active Variant and Offer, its Catalog identity and every matching AttributeSchema/AttributeDefinition.**

- A successful full-catalog evaluation must include each eligible item in **exactly one initial evaluation batch**. An item cannot be divided into fragments, even if its complete representation exceeds a provider token limit.
- The commercial child records and their source IDs must not be removed when a batch nominates a candidate. Candidate child IDs describe relevance, not authorization to delete siblings in later model evaluations.
- When an item is too large for a vetted request budget, the run **fails closed**, without a partial or invented answer. An operator can deliberately configure a validated larger-context route; legacy item fragmentation and truncation are forbidden.
- This guarantees **transport completeness** in successful runs, NOT that any LLM actually reasons correctly about every submitted fact. Semantic reliability requires separate evaluations.
- All merchant data remains tenant-scoped, read-only at the AI data boundary. Gemini/other LiteLLM routed models choose capabilities; Go only assembles, verifies, authorizes and executes.

## Representation

The canonical PostgreSQL hierarchy remains:

`Catalog → CatalogItem → AttributeSchema / AttributeDefinitions → Attributes → Variants → Offers`

The AI projection is a **read model**, never a second database/domain. It embeds the relevant Catalog and versioned Schema beside each Item, while retaining source IDs and the existing top-level arrays. Resolution exposes both original `attributes` and `resolved_attributes`: canonical key, schema label, declared type, origin and unchanged value. Variants also expose `effective_attributes` and resolved values after explicit variant overrides. Unknown free attributes preserve their literal keys; Go never invents merchant meanings.

Active offers without `variant_id` belong directly to their item; an offer with a referenced missing/inactive variant is **invalid evidence**, not an item-level offer. The read model also exposes `variant.offers[]` and `item.item_level_offers[]` to remove cross-array joining from the model, while retaining the canonical `item.offers[]` and original IDs for exact evidence verification. Duplicated representation is transport-only, not a second commercial source of truth.

## Atomic token batching

1. Hydrate eligible records and schema definitions before serialization.
2. Assemble complete self-describing items, then count the **actual complete model request** including system instructions, catalog contract, customer message and conversational context.
3. Partition only between complete items, according to the configured LiteLLM route's vetted context allowance. There is no product-count heuristic.
4. Reject any item that cannot fit intact. Reject duplicated, missing, cross-item or mutated entities before provider dispatch.
5. Send independent batches. Track batch completion and reject partial overall coverage.
6. Model-selected candidates may undergo bounded reductions, but every selected commercial entity stays whole in every reduction/decision stage.

Each catalog-wide `overview` produces a general summary, not detail-level evidence. Detailed product claims require verified complete entity evidence. Explicit hierarchy must not silently downgrade to overview.

## Scoped conversation context

When focusing an item, offer, variant or comparison, retrieve every active variant and offer of **each referenced item**, using cursor pagination until completion. A catalog-level focus must not expose an arbitrary first-N prefix as a complete catalog; only the manifest may be shown until `evaluate_full_catalog` is requested. Missing references, non-progressing cursors and cross-tenant relationships fail closed. When interpreting comparison references, the alternate item/offer lookup is permitted **only** after a typed `not_found`; authorization failures, storage errors and tenant mismatches must propagate and must never be reinterpreted as an unknown reference. The normal catalog manifest remains discovery-only and is not proof of completeness.

## Currency, evidence and version integrity

- Preserve Offer amount as its decimal string and JSON attribute numbers without float64 conversion.
- Preserve price/availability verification and timestamps. Never infer an available, verified price when evidence is missing or stale.
- A stable snapshot/revision check is required to avoid emitting decisions from concurrently changed merchant records. The legacy count + MAX(updated_at) revision is not sufficient to prove full content consistency; hardening of this boundary is part of acceptance.
- A fallback provider may execute only if **its own verified limits and supported API contract** fit the complete request, without truncation.

## Test acceptance (real runs required)

- `gofmt`, `go build ./...`, `go vet ./...`, `go test ./... -count=1` and repository Quality gate, including the configured PostgreSQL 16 integration service.
- Real PostgreSQL integration must cover >200 variants/offers of one item and must detect a price change even when item/offer counts remain unchanged.
- Tests must assert: exactly one occurrence of each original item; full equality of item and nested facts; no splitting of an oversize item; semantic key-label/type resolution and variant inheritance; all paginated options/offers in scoped AI context; rejection of orphan offers, changed prices, duplicate IDs or missing definitions.
- Live LiteLLM/provider model behavior requires real configured credentials and a separate reproducible run; a stubbed adapter is never described as a successful live provider test.

## Explicit exclusions

No Intent Engine, keyword triggers, semantic search, tenant-selected provider, catalog-domain redesign, legacy fragmented-item fallback, candidate-child pruning, or changes to production backend main.

This v2 amendment explicitly supersedes only the incompatible AI **transport** constraints in CLOSED v1 documents. Other closed business contracts are preserved.

# internal/domain/ai/contracts

## Overview
Defines provider-neutral, versioned domain metadata, evidence types, and projections for merchant-facing and customer-facing AI interactions. These contracts act as pure domain representations that decouple model-facing input/output schemas from database models and third-party AI APIs.

## Interface
- `BusinessContextV1` / `BusinessRuntimePolicyContextV1`: Merchant business status, vertical, locale, and policy flags ([business.go](./business.go#L13-L34)).
- `EvidenceSourceType`, `EvidenceAuthority`, `EvidenceEnvelopeV1`: Structured provenance metadata and evidence containment ([evidence.go](./evidence.go#L19-L80)).
- `LeadAssistV1`, `TransactionAssistV1`, `SalesWorkflowAssistV1`: Sanitized sales projections and commercial workflow states ([sales.go](./sales.go#L12-L180)).
- `LeadAttributionAssistV1`, `LeadScoreSnapshotAssistV1`: Attribution provenance and bounded historical lead score snapshots ([lead_evidence.go](./lead_evidence.go#L12-L58)).
- `ConversationAssistV1`, `PublicTextMessageAssistV1`, `ConversationSummaryAssistV1`, `CustomerAssistV1`: Sanitized conversation context, public message stream, committed summaries, and customer profile assist ([inbox.go](./inbox.go#L11-L105)).
- `PublishedPolicyAssistV1`, `PublishedKnowledgeAssistV1`: Merchant business policies and FAQs ([knowledge.go](./knowledge.go#L9-L52)).
- `QualificationReferenceSetV1`, `ParseQualificationReferences`: Bounded reference set projection of qualification evidence strings ([qualification.go](./qualification.go#L14-L40)).
- `TableSourceRecord`, `SourceDisposition`, `SourceInventory()`: PostgreSQL database table security census auditing, restricting AI projection exposure against raw tables ([source_inventory.go](./source_inventory.go#L13-L27)). Note: This is an internal table security perimeter, not product inventory snapshots.
- `Registry`: Contract registry validating evidence sources against allowed schema fields ([registry.go](./registry.go#L106)).
- `NewMerchantAssistRegistry`: Constructor initializing the merchant assist evidence registry ([merchant_registry.go](./merchant_registry.go#L18)).

## Invariants
- Zero SQL or database dependencies (`pgx` is never imported).
- Zero network or HTTP calls inside contract methods.
- Tenant ID (`business_id`) and raw internal database keys stay strictly outside model-facing fields.
- Projections enforce closed enum sets matching the PostgreSQL 16 schema baseline (schema census benchmarks baseline migrations `000001..000083` in `source_inventory.go:27`).
- All numbers representing currency or scores enforce fixed-precision strings validated via `big.Rat`.

## Side Effects
- None (pure in-memory value objects and validation logic).

## Verification
- Run: `go test ./internal/domain/ai/contracts`

# internal/domain/ai/prompts

## Overview
Defines versioned, canonical system prompts and operational boundary instructions for AI agents in Mujeeb 24. These prompts are compiled directly into the binary as Go string constants, ensuring immutable, auditable, and version-controlled instructions for Customer Sales AI, Merchant Catalog AI, and catalog evaluation stages.

## Interface
- `CustomerSalesSystemPrompt`: Boundary contract for customer conversation, intent understanding, evidence requirements, and capability function calling ([prompts.go](./prompts.go#L25)).
- `CustomerSalesSystemPromptVersion`: Version identifier string (`customer-sales-v15-evidence-boundaries`) ([prompts.go](./prompts.go#L65)).
- `MerchantCatalogAIV2SystemPrompt`: Boundary contract for merchant catalog authoring, entity operations, and proposal creation ([merchant_catalog_v2.go](./merchant_catalog_v2.go#L6)).
- `BatchEvaluationSystemPrompt` / `BatchEvaluationSystemPromptVersion`: First-stage catalog candidate evaluation prompt and version (`batch-evaluation-v2`) ([prompts.go](./prompts.go#L67-L78)).
- `CandidateReductionSystemPromptSuffix`: Token budget pruning instructions for large candidate sets ([prompts.go](./prompts.go#L82)).
- `FinalEvaluationSystemPrompt` / `FinalEvaluationSystemPromptVersion`: Standalone terminal decision prompt and version (`final-evaluation-v5-evidence-boundaries`) ([prompts.go](./prompts.go#L97-L116)).

## Invariants
- Stored as typed Go constants; never loaded from external files or environment variables at runtime.
- Customer Sales and Merchant Catalog prompts remain strictly segregated with distinct agent roles and tool capabilities.
- Prohibits hallucination or invention of prices, products, availability, or operational execution.
- Mandates structured tool selection (single function call) without speculative prose outside the call.
- Catalog Evaluation Operational Limits (Issue 4):
  - Current Code Behavior: Multi-stage prompts (`BatchEvaluationSystemPrompt`, `FinalEvaluationSystemPrompt`) are invoked sequentially by `CatalogBatchController.runPagedCatalogEvaluation`. The current controller implementation loops through all catalog pages without an enforced batch ceiling.
  - Known Operational Risk: For large catalogs, this unbounded loop poses a Denial-of-Wallet risk and potential HTTP cycle timeout.
  - Proposed Improvement: Enforce a hard ceiling on batch evaluations (e.g. max 3 batches / ~150 candidates) before failing closed to `handoff_to_human`.

## Side Effects
- None (pure immutable string constants).

## Verification
- Run: `go test ./internal/domain/ai/prompts`

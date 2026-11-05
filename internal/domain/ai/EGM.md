# internal/domain/ai

## Overview
Subsystem governing AI boundary contracts, structured evidence definitions, domain projections, and immutable system prompts for Mujeeb 24's intelligent agent workflows.

## Subdirectories & Boundaries
- [contracts](./contracts/EGM.md) - Provider-neutral, versioned domain projections, sanitized customer sales and evidence data structures.
- [prompts](./prompts/EGM.md) - Compiled Go string constants defining immutable, versioned system prompts for Customer Sales and Merchant Catalog agents.

## Invariants
- Zero external imports: No SQL packages (`pgx`), no HTTP frameworks, and no third-party AI provider SDKs are imported into the AI domain.
- Vendor neutrality: Contracts specify sanitized domain projections and tool capability definitions independent of LLM vendor schemas.
- Immutable system prompts: Versioned prompt templates are compiled directly into the binary as Go string constants.

## Side Effects
- None (pure domain value objects, contracts, and constant definitions).

## Verification
- Run: `go test ./internal/domain/ai/...`

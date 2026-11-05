# internal/application/merchantcatalogai

## Overview
Domain engine for AI-assisted merchant catalog authoring. Allows merchants to create, update, and structure complex product catalogs, variants, pricing offers, and attribute schemas conversationally. Implements a strict two-phase proposal lifecycle where the model outputs reviewable proposals that require explicit merchant approval prior to execution.

## Interface
- `Agent`: Conversational orchestrator handling merchant catalog authoring turns (`HandleTurn(ctx, TurnInput)`) ([agent.go](./agent.go#L25)).
- `Proposal`: Authoritative proposal container adhering to schema version 3 ([proposal.go](./proposal.go#L3)).
- `ProposalStatus` / `Operation`: Status enums (`StatusResolved`, `StatusAmbiguous`, `StatusNeedsMoreData`, `StatusNotFound`) and operation targets (`OperationCreate`, `OperationUpdate`, `OperationDelete`, `OperationAskMerchant`) ([proposal.go](./proposal.go#L9-L25)).
- `ExecutionService`: Concrete executor applying approved proposals to catalog storage ([execution.go](./execution.go)); implements the `ports.ProposalExecutor` interface.
- `Proposal.ValidateAttributeDefinitions`: Schema preflight validation and managed attribute materialization ([attribute_definition_validation.go](./attribute_definition_validation.go)).

## Invariants
- Two-phase proposal boundary: The AI model strictly outputs structured `Proposal` objects; it is completely barred from direct SQL writes or catalog mutations.
- Execution requires explicit merchant approval and an authorized `PrincipalID`.
- AI guard check (`ports.BusinessAIExecutionGuard`) halts processing immediately if merchant has opted out.
- Model-generated attribute keys remain semantic aliases until translated to authoritative `attr_` keys during approved execution.

## Side Effects
- Invokes AI inference; approved execution applies catalog entity mutations within database transactions.

## Verification
- Run: `go test ./internal/application/merchantcatalogai`

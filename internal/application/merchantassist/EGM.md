# internal/application/merchantassist

## Overview
Merchant-facing AI assistant providing grounded, read-only analytics, conversational insights, and guidance for merchant operators. Uses strict tool contracts (function calling) over business records, conversations, leads, and catalog data without ever granting the model mutation privileges.

## Interface
- `Assistant`: Core assistant service coordinating tool calls, evidence readers, and content sanitization ([assistant.go](./assistant.go#L35)).
- `AssistRequest` / `AssistResponse`: Strongly typed input/output envelope carrying sanitized answer, evidence payload, and human review flags ([assistant.go](./assistant.go#L19-L33)).
- `ToolModel`: Provider-agnostic tool calling interface (`CompleteTools(ctx, messages, tools, pinned string)`) ([tool_contract.go](./tool_contract.go#L38)).
- `ToolDefinition`, `ToolCall`, `ToolCompletion`: Structured capability contracts for vendor-neutral tool calling ([tool_contract.go](./tool_contract.go#L10-L35)).
- Readers: `PublishedReader`, `RecordReader`, `ConversationHistoryReader`, `LeadEvidenceReader`, `DiscoveryReader` ([discovery_reader.go](./discovery_reader.go), [record_reader.go](./record_reader.go)).

## Invariants
- Zero mutation privileges: The assistant executes strictly read-only tools (`dashboard_overview`, `find_customers`, `customer_activity`, etc.); it cannot create, update, or delete products or records.
- Question Size Bounds: Questions are strictly bounded to a maximum size of 1600 bytes (approximately 800 UTF-8 Arabic characters).
- All retrieved entity fields are validated against `contracts.Registry` before inclusion in AI context.
- Tool parameter schemas strictly mandate `additionalProperties: false`.

## Side Effects
- Invokes external AI tool calling completions and queries database read models.

## Verification
- Run: `go test ./internal/application/merchantassist`

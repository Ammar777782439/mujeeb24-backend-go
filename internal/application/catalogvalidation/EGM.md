# internal/application/catalogvalidation

## Overview
Validation engine for merchant catalog dynamic attribute schemas, structured data types, range bounds, regular expression patterns, and enum constraints. Ensures catalog items and variants adhere to structured definitions while accommodating free-form custom attributes.

## Interface
- `ValidateDefinition(dataType string, rules map[string]any) error`: Validates schema attribute definitions, rule constraints (min/max, regex, enum), and data type compatibility ([attributes.go](./attributes.go#L24)).
- `ValidateAttributes(schema *ports.AttributeSchemaRecord, attributes map[string]any) error`: Enforces full schema constraints, required fields, and type checking on item attributes ([attributes.go](./attributes.go#L76)).
- `ValidatePartialAttributes(schema *ports.AttributeSchemaRecord, attributes map[string]any) error`: Validates partial attribute maps for variant overrides without mandating parent required fields ([attributes.go](./attributes.go#L83)).

## Invariants
- Allowed data types are strictly bounded: `text`, `number`, `boolean`, `date`, `datetime`, `select`, `multi_select`, `location`, `money`.
- `attr_` prefix is strictly reserved for managed schema keys; model-generated semantic attribute aliases are materialized to authoritative `attr_` keys during approved execution.
- Free-form merchant attributes outside the managed schema are preserved without validation failure.
- Min rule cannot exceed max rule; regex patterns must compile successfully.

## Side Effects
- None (pure in-memory validation).

## Verification
- Run: `go test ./internal/application/catalogvalidation`

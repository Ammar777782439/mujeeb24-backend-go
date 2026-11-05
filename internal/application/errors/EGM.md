# internal/application/errors

## Overview
Defines canonical application error codes and structured error types for Mujeeb 24. Standardizes error responses across use cases and maps cleanly to HTTP status envelopes and OpenAPI error schemas.

## Interface
- Error Codes: `CodeValidation`, `CodeNotFound`, `CodeForbidden`, `CodeUnauthenticated`, `CodeConflict`, `CodeIdempotencyConflict`, `CodeStaleResource`, `CodeInvalidState`, `CodeExternalDependency`, `CodeNotImplemented` ([errors.go](./errors.go#L8)).
- `Error`: Structured error type supporting code, human message, target field name, retryability flag, and root cause ([errors.go](./errors.go#L20)).
- Functions: `New(code, message)`, `NotImplemented()`, `Error()`, `Unwrap()` ([errors.go](./errors.go#L28-L36)).

## Invariants
- Error codes are closed enum constants matching HTTP API response envelope specifications.
- Implements standard Go `error` and `Unwrap()` contracts for chaining and inspection.
- Provides `Retryable` boolean flag to inform callers whether an operation may be retried.

## Side Effects
- None (pure error values).

## Verification
- Run: `go test ./internal/application/...`

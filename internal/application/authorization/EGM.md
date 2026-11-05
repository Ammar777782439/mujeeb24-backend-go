# internal/application/authorization

## Overview
Implements role-based access control (RBAC) and granular permission enforcement across all business operations in Mujeeb 24. Maps API operation IDs to exact permission strings and validates actor capabilities and conversation access scopes.

## Interface
- Permission constants: `PermissionBusinessRead`, `PermissionConversationsRead`, `PermissionConversationsReply`, `PermissionCatalogWrite`, `PermissionSalesApprove`, `PermissionChannelsManage`, `PermissionMerchantAIUse`, `PermissionAll` ([permissions.go](./permissions.go#L10)).
- `RequiredForOperation(operationID string) (string, bool)`: Resolves the permission required for a specific OpenAPI operation ([permissions.go](./permissions.go#L79)).
- `NormalizePermissions(values []string) ([]string, bool)`: Validates and deduplicates permission strings ([permissions.go](./permissions.go#L84)).
- `Has(actor commands.ActorContext, permission string) bool`: Checks if an actor holds a specific permission or wildcard ([permissions.go](./permissions.go#L104)).
- `CanGrant(actor commands.ActorContext, requested []string) bool`: Prevents privilege escalation during invitation or role assignment ([permissions.go](./permissions.go#L113)).
- `KnownPermissions() []string`: Returns a sorted list of all registered permissions ([permissions.go](./permissions.go#L122)).
- Conversation Visibility Helpers:
  - `CanReadConversation(actor commands.ActorContext, conversation ports.ConversationRecord) bool` ([conversations.go](./conversations.go#L12)).
  - `CanReplyToConversation(actor commands.ActorContext, conversation ports.ConversationRecord) bool` ([conversations.go](./conversations.go#L23)).
  - `CanClaimConversation(actor commands.ActorContext, conversation ports.ConversationRecord) bool` ([conversations.go](./conversations.go#L34)).

## Invariants
- Unknown permissions are strictly rejected during normalization.
- Wildcard `*` is reserved for owners/admins and cannot be assigned via granular normalization inputs.
- Privilege escalation is prevented: an actor cannot grant permissions that exceed their own verified scope.
- Perimeter Security Model (Known Architectural Inversion):
  - Current Code Behavior: Permission checks are performed by HTTP middleware based on OpenAPI `operationID`. In `internal/adapters/primary/http/handlers/core.go:183`, if `operationID` is missing from the context, permission checks are bypassed.
  - CQRS Command Handlers in `internal/application/commands` perform zero internal permission checks, assuming upstream HTTP validation.
  - Defense-in-depth Recommendation: Handlers executing sensitive state mutations should inspect `actor.Role` or call `authorization.Has()` internally to prevent unauthorized internal execution.

## Side Effects
- None (pure policy evaluation and boolean checks).

## Verification
- Run: `go test ./internal/application/authorization`

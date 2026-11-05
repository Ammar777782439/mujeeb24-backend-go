# internal/application/commands

## Overview
CQRS command declarations and mutation input structures across all business subsystems. Encapsulates actor authorization contexts, idempotency keys, optimistic locking resource versions, and strongly typed domain identifiers.

## Interface
- Base Types: `ActorContext` (caller authorization context encapsulating `PrincipalID`, `BusinessID`, `Role`, and permissions), `CommandMeta`, `QueryMeta`, `MutationResult`, `CommandHandler[C, R]` ([meta.go](./meta.go#L30-L59)).
- Customer & Catalog: `CreateCustomerCommand`, `UpdateCustomerCommand`, `MergeCustomerCommand`, `CreateCatalogCommand`, `CreateCatalogItemCommand`, `CreateOfferCommand`, `CreateVariantCommand`, `CreateAttributeSchemaVersionCommand` ([customers_catalog.go](./customers_catalog.go)).
- Conversations & Messaging: `CreateOutboundMessageCommand`, `AssignConversationCommand`, `AddPrivateNoteCommand`, `UpdateConversationLabelsCommand` ([channels_conversations.go](./channels_conversations.go)), `SendCannedReplyCommand` ([inbox.go](./inbox.go#L37)).
- Sales & Leads: `CreateLeadCommand`, `QualifyLeadCommand`, `MarkLeadLostCommand`, `CreateTransactionDraftCommand`, `ConfirmTransactionCommand`, `CancelTransactionCommand` ([leads_transactions.go](./leads_transactions.go)).
- Team Governance: `InviteTeamMemberCommand`, `AcceptTeamInvitationCommand`, `UpdateTeamMemberRoleCommand`, `RevokeTeamMemberCommand` ([team.go](./team.go#L38)).
- Channel Lifecycle: `BeginChannelConnectionCommand`, `ReconnectChannelCommand`, `DisconnectChannelCommand`, `PurgeChannelHistoryCommand` ([channels_conversations.go](./channels_conversations.go)).

## Invariants
- Commands must encapsulate `CommandMeta` ensuring caller identity (`ActorContext`) is never omitted.
- Resource mutations supporting optimistic concurrency check `ExpectedVersion`.
- Sensitive state operations enforce unique `IdempotencyKey`.
- Pure Go structures without persistence or transport logic.
- Defense-in-Depth Authorization (Known Inversion): Command handlers currently rely on HTTP OpenAPI middleware for permission checks and do not validate permissions internally. Handlers executing sensitive mutations should verify `ActorContext.Role`.

## Side Effects
- None (pure command structures and handler interfaces).

## Verification
- Run: `go test ./internal/application/...`

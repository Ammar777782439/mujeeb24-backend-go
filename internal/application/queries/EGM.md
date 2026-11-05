# internal/application/queries

## Overview
CQRS read models, query declarations, and projection views across all domain facets. Defines cursor-based pagination parameters, filter specifications, and returned view structures for dashboard, inbox, team, and reporting APIs.

## Interface
- Identity & Tenancy: `GetCurrentPrincipalQuery`, `ListAccessibleBusinessesQuery`, `GetBusinessQuery`, `GetBusinessPolicyQuery` ([queries.go](./queries.go#L11)).
- Conversations & Inbox: `ListConversationsQuery`, `GetConversationQuery`, `ListConversationMessagesQuery`, `GetConversationInboxOverviewQuery`, `GetConversationSummaryQuery` ([queries.go](./queries.go), [conversation_inbox.go](./conversation_inbox.go)).
- Channels: `ListChannelConnectionsQuery`, `GetChannelConnectionQuery`, `GetConnectionCapabilitiesQuery` ([queries.go](./queries.go#L36)).
- Customers & Catalog: `ListCustomersQuery`, `GetCustomerQuery`, `ListCatalogsQuery`, `ListCatalogItemsQuery`, `ListOffersQuery`, `ListVariantsQuery` ([queries.go](./queries.go)).
- Sales & Audit: `ListLeadsQuery`, `ListTransactionsQuery`, `ListAuditEventsQuery`, `ListAIDecisionsQuery` ([queries.go](./queries.go)).
- Team: `ListTeamMembersQuery`, `ListTeamInvitationsQuery` ([team.go](./team.go)).

## Invariants
- Queries are strictly read-only and immutable.
- All listing operations implement cursor-based pagination (`Limit`, `Cursor`) returning `ListResult[T]` with `NextCursor` and `HasMore`.
- Queries require `QueryMeta` containing `ActorContext` to guarantee tenant isolation.
- Dead-Letter Observability (Known Gap): No query currently exists to list dead-letter queue entries (`ListDeadLettersQuery` is not implemented in the application layer; direct database inspection or admin SQL queries are required to inspect quarantined messages).

## Side Effects
- None (pure query DTOs).

## Verification
- Run: `go test ./internal/application/...`

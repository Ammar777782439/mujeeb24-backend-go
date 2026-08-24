package commands

import "context"

type ID string
type PrincipalID ID
type BusinessID ID
type ConnectionID ID
type CustomerID ID
type ConversationID ID
type MessageID ID
type CatalogID ID
type CatalogItemID ID
type AttributeSchemaID ID
type OfferID ID
type VariantID ID
type LeadID ID
type TransactionID ID
type AIDecisionID ID
type AuditEventID ID

type ResourceVersion string

// Decimal is a validated base-10 representation for quantities. It is not a money type;
// conversion and scale validation belong to the application/domain boundary.
type Decimal string

type ActorContext struct {
	PrincipalID PrincipalID
	BusinessID  BusinessID
	Role        string
	Permissions []string
}

type CommandMeta struct {
	Actor           ActorContext
	RequestID       string
	CorrelationID   string
	IdempotencyKey  string
	ExpectedVersion *ResourceVersion
}

type QueryMeta struct {
	Actor         ActorContext
	RequestID     string
	CorrelationID string
}

type CommandHandler[C any, R any] interface {
	Handle(context.Context, C) (R, error)
}

type QueryHandler[Q any, R any] interface {
	Handle(context.Context, Q) (R, error)
}

type MutationResult struct {
	ResourceID      ID
	ResourceVersion ResourceVersion
	Status          string
	Accepted        bool
}

type EmptyResult struct{}

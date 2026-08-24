package commands

import "time"

type BusinessView struct {
	ID              BusinessID
	Name            string
	Slug            string
	Status          string
	VerticalType    string
	Timezone        string
	DefaultCurrency string
	Locale          string
	ResourceVersion ResourceVersion
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
type BusinessPolicyView struct {
	BusinessID                BusinessID
	AIMode                    string
	DefaultHumanReview        bool
	AllowAutoReply            bool
	AllowAutoLeadCreation     bool
	AllowAutoTransactionDraft bool
	AllowAutoConfirmation     bool
	ResourceVersion           ResourceVersion
}
type ChannelConnectionView struct {
	ID                       ConnectionID
	BusinessID               BusinessID
	Provider                 string
	Channel                  string
	Status                   string
	ExternalAccountReference string
	ResourceVersion          ResourceVersion
}
type ConversationView struct {
	ID              ConversationID
	BusinessID      BusinessID
	CustomerID      CustomerID
	State           string
	Ownership       string
	AIMode          string
	ResourceVersion ResourceVersion
}
type MessageView struct {
	ID              MessageID
	ConversationID  ConversationID
	Direction       string
	Origin          string
	Status          string
	Text            string
	CreatedAt       time.Time
	ResourceVersion ResourceVersion
}
type CustomerView struct {
	ID              CustomerID
	BusinessID      BusinessID
	DisplayName     string
	Status          string
	ResourceVersion ResourceVersion
}
type AttributeSchemaView struct {
	ID         AttributeSchemaID
	BusinessID BusinessID
	Name       string
	Version    int
}
type CatalogView struct {
	ID              CatalogID
	BusinessID      BusinessID
	Name            string
	Status          string
	ResourceVersion ResourceVersion
}
type CatalogItemView struct {
	ID              CatalogItemID
	BusinessID      BusinessID
	CatalogID       CatalogID
	Name            string
	ItemType        string
	Status          string
	ResourceVersion ResourceVersion
}
type OfferView struct {
	ID              OfferID
	BusinessID      BusinessID
	CatalogItemID   CatalogItemID
	VariantID       VariantID
	Name            string
	Status          string
	ResourceVersion ResourceVersion
}
type VariantView struct {
	ID              VariantID
	BusinessID      BusinessID
	CatalogItemID   CatalogItemID
	Name            string
	Status          string
	ResourceVersion ResourceVersion
}
type LeadView struct {
	ID              LeadID
	BusinessID      BusinessID
	CustomerID      CustomerID
	Status          string
	ResourceVersion ResourceVersion
}
type TransactionView struct {
	ID              TransactionID
	BusinessID      BusinessID
	CustomerID      CustomerID
	State           string
	TransactionType string
	ResourceVersion ResourceVersion
}
type AIDecisionView struct {
	ID              AIDecisionID
	BusinessID      BusinessID
	Lifecycle       string
	RequiresHuman   bool
	RequestedAction string
}
type AuditEventView struct {
	ID           AuditEventID
	BusinessID   BusinessID
	ActorType    string
	Action       string
	ResourceType string
	ResourceID   string
	OccurredAt   time.Time
}
type ListResult[T any] struct {
	Items      []T
	NextCursor string
	HasMore    bool
}

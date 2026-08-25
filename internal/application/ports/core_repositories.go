package ports

import (
	"context"
	"time"
)

type CustomerRecord struct {
	ID                 string
	BusinessID         string
	Profile            []byte
	ContactPoints      []byte
	LocalePreference   *string
	Status             string
	MergedIntoCustomer *string
	ResourceVersion    int64
	UpdatedAt          time.Time
}

type CustomerRepository interface {
	GetByID(ctx context.Context, businessID, customerID string) (CustomerRecord, error)
}

type CustomerPage struct {
	Items      []CustomerRecord
	NextCursor string
	HasMore    bool
}

type CustomerCreate struct {
	ID               string
	BusinessID       string
	Profile          []byte
	ContactPoints    []byte
	LocalePreference *string
}

type CustomerUpdate struct {
	BusinessID       string
	CustomerID       string
	ExpectedVersion  int64
	Profile          []byte
	ContactPoints    []byte
	LocalePreference *string
}

type CustomerMerge struct {
	BusinessID       string
	CustomerID       string
	TargetCustomerID string
	ExpectedVersion  int64
	Reason           string
}

type CustomerRuntimeRepository interface {
	List(ctx context.Context, businessID, search, status string, limit int, cursor string) (CustomerPage, error)
	Create(ctx context.Context, create CustomerCreate) (CustomerRecord, error)
	Update(ctx context.Context, update CustomerUpdate) (CustomerRecord, error)
	Merge(ctx context.Context, merge CustomerMerge) (CustomerRecord, error)
}

type ConversationRecord struct {
	ID                  string
	BusinessID          string
	CustomerID          string
	State               string
	Ownership           string
	AIModeOverride      *string
	Priority            string
	AssignmentReference *string
	ResourceVersion     int64
	LastActivityAt      time.Time
}

type ConversationRepository interface {
	GetByID(ctx context.Context, businessID, conversationID string) (ConversationRecord, error)
}

type ConversationPage struct {
	Items      []ConversationRecord
	NextCursor string
	HasMore    bool
}

type ConversationUpdate struct {
	BusinessID          string
	ConversationID      string
	ExpectedVersion     int64
	State               *string
	Ownership           *string
	AIModeOverride      *string
	Priority            *string
	AssignmentReference *string
}

type ConversationRuntimeRepository interface {
	List(ctx context.Context, businessID, state, ownership, channel string, customerID *string, limit int, cursor string) (ConversationPage, error)
	Update(ctx context.Context, update ConversationUpdate) (ConversationRecord, error)
	AdvanceVersion(ctx context.Context, businessID, conversationID string, expectedVersion int64) (ConversationRecord, error)
}

type ChannelConnectionRecord struct {
	ID                       string
	BusinessID               string
	ProviderReference        string
	Channel                  string
	ProviderAccountReference *string
	ProviderConnectionRef    string
	Status                   string
	SecretReference          string
	ResourceVersion          int64
	UpdatedAt                time.Time
}

type ChannelConnectionRepository interface {
	GetByID(ctx context.Context, businessID, connectionID string) (ChannelConnectionRecord, error)
	GetByProviderReferences(ctx context.Context, providerReference, providerAccountReference, providerConnectionReference string) (ChannelConnectionRecord, error)
}

type ChannelConnectionPage struct {
	Items      []ChannelConnectionRecord
	NextCursor string
	HasMore    bool
}

type ChannelConnectionTransition struct {
	BusinessID      string
	ConnectionID    string
	ExpectedVersion int64
	TargetStatus    string
	Action          string
	Reason          string
	ActorReference  string
}

type ChannelConnectionRuntimeRepository interface {
	List(ctx context.Context, businessID, status, channel string, limit int, cursor string) (ChannelConnectionPage, error)
	Transition(ctx context.Context, transition ChannelConnectionTransition) (ChannelConnectionRecord, error)
}

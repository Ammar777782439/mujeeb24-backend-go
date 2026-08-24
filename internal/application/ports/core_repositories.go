package ports

import "context"

type CustomerRecord struct {
	ID                 string
	BusinessID         string
	Profile            []byte
	ContactPoints      []byte
	LocalePreference   *string
	Status             string
	MergedIntoCustomer *string
}

type CustomerRepository interface {
	GetByID(ctx context.Context, businessID, customerID string) (CustomerRecord, error)
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
}

type ConversationRepository interface {
	GetByID(ctx context.Context, businessID, conversationID string) (ConversationRecord, error)
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
}

type ChannelConnectionRepository interface {
	GetByID(ctx context.Context, businessID, connectionID string) (ChannelConnectionRecord, error)
}

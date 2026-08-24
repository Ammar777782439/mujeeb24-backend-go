package ports

import (
	"context"
	"time"
)

type CatalogRecord struct {
	ID          string
	BusinessID  string
	Name        string
	Description *string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CatalogItemRecord struct {
	ID                     string
	BusinessID             string
	CatalogID              string
	AttributeSchemaID      *string
	AttributeSchemaVersion *int
	ItemType               string
	Name                   string
	Status                 string
	Attributes             []byte
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type OfferRecord struct {
	ID                 string
	BusinessID         string
	CatalogItemID      string
	VariantID          *string
	Name               string
	PricingMode        string
	Amount             *string
	Currency           *string
	AvailabilityStatus string
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type VariantRecord struct {
	ID            string
	BusinessID    string
	CatalogItemID string
	Name          string
	Attributes    []byte
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type AttributeDefinitionRecord struct {
	ID           string
	Key          string
	Label        string
	DataType     string
	Required     bool
	Searchable   bool
	DisplayOrder int
}

type AttributeSchemaRecord struct {
	ID          string
	BusinessID  string
	Name        string
	Version     int
	Definitions []AttributeDefinitionRecord
}

type CatalogPage struct {
	Items      []CatalogRecord
	NextCursor string
	HasMore    bool
}

type CatalogItemPage struct {
	Items      []CatalogItemRecord
	NextCursor string
	HasMore    bool
}

type OfferPage struct {
	Items      []OfferRecord
	NextCursor string
	HasMore    bool
}

type VariantPage struct {
	Items      []VariantRecord
	NextCursor string
	HasMore    bool
}

type AttributeSchemaPage struct {
	Items      []AttributeSchemaRecord
	NextCursor string
	HasMore    bool
}

type CatalogRepository interface {
	ListCatalogs(ctx context.Context, businessID, status string, limit int, cursor string) (CatalogPage, error)
	GetCatalog(ctx context.Context, businessID, catalogID string) (CatalogRecord, error)
	ListCatalogItems(ctx context.Context, businessID, catalogID, search, status string, limit int, cursor string) (CatalogItemPage, error)
	GetCatalogItem(ctx context.Context, businessID, catalogID, itemID string) (CatalogItemRecord, error)
	ListOffers(ctx context.Context, businessID, itemID, status string, limit int, cursor string) (OfferPage, error)
	ListVariants(ctx context.Context, businessID, itemID, status string, limit int, cursor string) (VariantPage, error)
	ListAttributeSchemas(ctx context.Context, businessID, name string, version *int, limit int, cursor string) (AttributeSchemaPage, error)
	GetAttributeSchema(ctx context.Context, businessID, schemaID string) (AttributeSchemaRecord, error)
}

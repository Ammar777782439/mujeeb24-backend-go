package ports

import (
	"context"
	"time"
)

type BusinessRecord struct {
	ID              string
	Name            string
	Slug            string
	Status          string
	VerticalType    string
	Timezone        string
	DefaultCurrency string
	Locale          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// BusinessRepository is the first persistence port. The record is deliberately
// application-facing and contains no pgx, SQL rows, or HTTP DTOs.
type BusinessRepository interface {
	GetByID(ctx context.Context, businessID string) (BusinessRecord, error)
}

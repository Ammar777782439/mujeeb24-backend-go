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
	ResourceVersion int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// BusinessRepository is the first persistence port. The record is deliberately
// application-facing and contains no pgx, SQL rows, or HTTP DTOs.
type BusinessRepository interface {
	GetByID(ctx context.Context, businessID string) (BusinessRecord, error)
}

type BusinessRuntimePolicyRecord struct {
	BusinessID                string
	AIMode                    string
	DefaultHumanReview        bool
	AllowAutoReply            bool
	AllowAutoLeadCreation     bool
	AllowAutoTransactionDraft bool
	AllowAutoConfirmation     bool
	ResourceVersion           int64
}

type BusinessProfileUpdate struct {
	BusinessID      string
	ExpectedVersion int64
	Name            *string
	VerticalType    *string
	Timezone        *string
	DefaultCurrency *string
	Locale          *string
}

type BusinessRuntimePolicyUpdate struct {
	BusinessID                string
	ExpectedVersion           int64
	AIMode                    *string
	DefaultHumanReview        *bool
	AllowAutoReply            *bool
	AllowAutoLeadCreation     *bool
	AllowAutoTransactionDraft *bool
	AllowAutoConfirmation     *bool
}

type BusinessManagementRepository interface {
	UpdateProfile(ctx context.Context, update BusinessProfileUpdate) (BusinessRecord, error)
	GetRuntimePolicy(ctx context.Context, businessID string) (BusinessRuntimePolicyRecord, error)
	UpdateRuntimePolicy(ctx context.Context, update BusinessRuntimePolicyUpdate) (BusinessRuntimePolicyRecord, error)
}

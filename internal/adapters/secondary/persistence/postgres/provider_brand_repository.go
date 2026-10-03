package postgres

import (
    "context"
    "errors"
    "strings"
    "time"

    "github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
)

type ProviderBrandRepository struct {
    adapter *Adapter
}

func NewProviderBrandRepository(adapter *Adapter) *ProviderBrandRepository {
    return &ProviderBrandRepository{adapter: adapter}
}

func (r *ProviderBrandRepository) Get(ctx context.Context, businessID, providerRef string) (ports.ProviderBrandRecord, bool, error) {
    if r == nil || r.adapter == nil {
        return ports.ProviderBrandRecord{}, false, ErrPoolClosed
    }
    businessID = strings.TrimSpace(businessID)
    providerRef = strings.TrimSpace(providerRef)
    if businessID == "" || providerRef == "" {
        return ports.ProviderBrandRecord{}, false, invalidRepositoryInput("provider_brand.get", "business and provider are required")
    }

    executor, err := r.adapter.Executor(ctx)
    if err != nil {
        return ports.ProviderBrandRecord{}, false, err
    }

    const query = `
        SELECT id::text, business_id::text, provider_ref, provider_brand_ref, display_name, created_at, updated_at
        FROM channel_provider_brands
        WHERE business_id = $1::uuid AND provider_ref = $2
    `
    var record ports.ProviderBrandRecord
    if err := executor.QueryRow(ctx, query, businessID, providerRef).Scan(
        &record.ID,
        &record.BusinessID,
        &record.ProviderRef,
        &record.ProviderBrandRef,
        &record.DisplayName,
        &record.CreatedAt,
        &record.UpdatedAt,
    ); err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            return ports.ProviderBrandRecord{}, false, nil
        }
        return ports.ProviderBrandRecord{}, false, classifyRepositoryWriteError("provider_brand.get", err)
    }
    return record, true, nil
}

func (r *ProviderBrandRepository) Create(ctx context.Context, brand ports.ProviderBrandRecord) (ports.ProviderBrandRecord, error) {
    if r == nil || r.adapter == nil {
        return ports.ProviderBrandRecord{}, ErrPoolClosed
    }
    brand.BusinessID = strings.TrimSpace(brand.BusinessID)
    brand.ProviderRef = strings.TrimSpace(brand.ProviderRef)
    brand.ProviderBrandRef = strings.TrimSpace(brand.ProviderBrandRef)
    brand.DisplayName = strings.TrimSpace(brand.DisplayName)
    if brand.ID == "" {
        brand.ID = uuid.NewString()
    }
    if brand.BusinessID == "" || brand.ProviderRef == "" || brand.ProviderBrandRef == "" || brand.DisplayName == "" {
        return ports.ProviderBrandRecord{}, invalidRepositoryInput("provider_brand.create", "id, business, provider, provider brand, and display name are required")
    }
    now := time.Now().UTC()

    executor, err := r.adapter.Executor(ctx)
    if err != nil {
        return ports.ProviderBrandRecord{}, err
    }

    const query = `
        INSERT INTO channel_provider_brands (
            id, business_id, provider_ref, provider_brand_ref, display_name, created_at, updated_at
        )
        VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $6)
        RETURNING id::text, business_id::text, provider_ref, provider_brand_ref, display_name, created_at, updated_at
    `
    var record ports.ProviderBrandRecord
    if err := executor.QueryRow(ctx, query, brand.ID, brand.BusinessID, brand.ProviderRef, brand.ProviderBrandRef, brand.DisplayName, now).Scan(
        &record.ID,
        &record.BusinessID,
        &record.ProviderRef,
        &record.ProviderBrandRef,
        &record.DisplayName,
        &record.CreatedAt,
        &record.UpdatedAt,
    ); err != nil {
        return ports.ProviderBrandRecord{}, classifyRepositoryWriteError("provider_brand.create", err)
    }
    return record, nil
}

var _ ports.ProviderBrandStore = (*ProviderBrandRepository)(nil)

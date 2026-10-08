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
        SELECT id::text, business_id::text, provider_ref, provider_brand_ref, display_name, lifecycle_state, created_at, updated_at
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
		&record.LifecycleState,
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
        RETURNING id::text, business_id::text, provider_ref, provider_brand_ref, display_name, lifecycle_state, created_at, updated_at
    `
	var record ports.ProviderBrandRecord
	if err := executor.QueryRow(ctx, query, brand.ID, brand.BusinessID, brand.ProviderRef, brand.ProviderBrandRef, brand.DisplayName, now).Scan(
		&record.ID,
		&record.BusinessID,
		&record.ProviderRef,
		&record.ProviderBrandRef,
		&record.DisplayName,
		&record.LifecycleState,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		classified := classifyRepositoryWriteError("provider_brand.create", err)
		if IsRepositoryKind(classified, RepositoryConflict) {
			return ports.ProviderBrandRecord{}, errors.Join(ports.ErrProviderBrandConflict, classified)
		}
		return ports.ProviderBrandRecord{}, classified
	}
	return record, nil
}

func (r *ProviderBrandRepository) Upsert(ctx context.Context, brand ports.ProviderBrandRecord) (ports.ProviderBrandRecord, error) {
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
		return ports.ProviderBrandRecord{}, invalidRepositoryInput("provider_brand.upsert", "id, business, provider, provider brand, and display name are required")
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
        ON CONFLICT (business_id, provider_ref)
        DO UPDATE SET
            provider_brand_ref = EXCLUDED.provider_brand_ref,
            display_name = EXCLUDED.display_name,
            lifecycle_state = 'active',
            updated_at = EXCLUDED.updated_at
        RETURNING id::text, business_id::text, provider_ref, provider_brand_ref, display_name, lifecycle_state, created_at, updated_at
    `
	var record ports.ProviderBrandRecord
	if err := executor.QueryRow(ctx, query, brand.ID, brand.BusinessID, brand.ProviderRef, brand.ProviderBrandRef, brand.DisplayName, now).Scan(
		&record.ID,
		&record.BusinessID,
		&record.ProviderRef,
		&record.ProviderBrandRef,
		&record.DisplayName,
		&record.LifecycleState,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return ports.ProviderBrandRecord{}, classifyRepositoryWriteError("provider_brand.upsert", err)
	}
	return record, nil
}

func (r *ProviderBrandRepository) ReserveUnused(ctx context.Context, businessID, providerRef string) (string, bool, error) {
	if r == nil || r.adapter == nil {
		return "", false, ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(providerRef) == "" {
		return "", false, invalidRepositoryInput("provider_brand.reserve", "business and provider are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return "", false, err
	}
	// Account-level disconnections are completed before this reservation.
	// No SocialAPI request is made while a database transaction is held.
	const query = `UPDATE channel_provider_brands b
        SET lifecycle_state='deleting', updated_at=now()
        WHERE b.business_id=$1::uuid AND b.provider_ref=$2
          AND NOT EXISTS (
            SELECT 1 FROM channel_connections c
            WHERE c.business_id=b.business_id AND c.provider_ref=b.provider_ref
              AND c.status NOT IN ('disconnected','archived')
          )
          AND NOT EXISTS (
            SELECT 1 FROM channel_provisioning_sessions s
            WHERE s.business_id=b.business_id AND s.provider_ref=b.provider_ref
              AND s.status IN ('pending_authorization','provisioning','reconnect_required')
              AND s.created_at > now() - interval '15 minutes'
          )
        RETURNING provider_brand_ref`
	var brandRef string
	if err := executor.QueryRow(ctx, query, businessID, providerRef).Scan(&brandRef); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, classifyRepositoryWriteError("provider_brand.reserve", err)
	}
	return brandRef, true, nil
}

// Only remove the local mapping after the provider confirms the deletion.
func (r *ProviderBrandRepository) CompleteDeletion(ctx context.Context, businessID, providerRef, brandRef string) error {
	if r == nil || r.adapter == nil {
		return ErrPoolClosed
	}
	if strings.TrimSpace(businessID) == "" || strings.TrimSpace(providerRef) == "" || strings.TrimSpace(brandRef) == "" {
		return invalidRepositoryInput("provider_brand.complete_delete", "business, provider and brand id are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return err
	}
	_, err = executor.Exec(ctx, `DELETE FROM channel_provider_brands
        WHERE business_id=$1::uuid AND provider_ref=$2
          AND provider_brand_ref=$3 AND lifecycle_state='deleting'`, businessID, providerRef, brandRef)
	if err != nil {
		return classifyRepositoryWriteError("provider_brand.complete_delete", err)
	}
	return nil
}

var _ ports.ProviderBrandStore = (*ProviderBrandRepository)(nil)
var _ ports.ProviderBrandCleanupStore = (*ProviderBrandRepository)(nil)

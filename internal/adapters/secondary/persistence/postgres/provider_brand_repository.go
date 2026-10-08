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
		classified := classifyRepositoryWriteError("provider_brand.create", err)
		if IsRepositoryKind(classified, RepositoryConflict) {
			return ports.ProviderBrandRecord{}, errors.Join(ports.ErrProviderBrandConflict, classified)
		}
		return ports.ProviderBrandRecord{}, classified
	}
	return record, nil
}

// Replace updates a stale mapping only when the caller still owns its previous
// external reference. A concurrent replacement must never be overwritten.
func (r *ProviderBrandRepository) Replace(ctx context.Context, businessID, providerRef, expectedBrandRef, nextBrandRef string) (bool, error) {
	if r == nil || r.adapter == nil {
		return false, ErrPoolClosed
	}
	businessID = strings.TrimSpace(businessID)
	providerRef = strings.TrimSpace(providerRef)
	expectedBrandRef = strings.TrimSpace(expectedBrandRef)
	nextBrandRef = strings.TrimSpace(nextBrandRef)
	if businessID == "" || providerRef == "" || expectedBrandRef == "" || nextBrandRef == "" {
		return false, invalidRepositoryInput("provider_brand.replace", "business, provider, old and new brand ids are required")
	}
	executor, err := r.adapter.Executor(ctx)
	if err != nil {
		return false, err
	}
	const query = `UPDATE channel_provider_brands SET provider_brand_ref=$4, updated_at=$5
		WHERE business_id=$1::uuid AND provider_ref=$2 AND provider_brand_ref=$3`
	tag, err := executor.Exec(ctx, query, businessID, providerRef, expectedBrandRef, nextBrandRef, time.Now().UTC())
	if err != nil {
		return false, classifyRepositoryWriteError("provider_brand.replace", err)
	}
	return tag.RowsAffected() == 1, nil
}

// CleanupIfUnused keeps the local mapping and provider brand in sync. Locking
// the business row serializes the check and provider DELETE against new OAuth
// session creation (which takes the same lock in CreateOrGet). The DELETE is
// idempotent at SocialAPI: a missing remote brand counts as already removed.
func (r *ProviderBrandRepository) CleanupIfUnused(ctx context.Context, businessID, providerRef string, deleteRemote func(context.Context, string) error) error {
	if r == nil || r.adapter == nil {
		return ErrPoolClosed
	}
	businessID = strings.TrimSpace(businessID)
	providerRef = strings.TrimSpace(providerRef)
	if businessID == "" || providerRef == "" || deleteRemote == nil {
		return invalidRepositoryInput("provider_brand.cleanup", "business, provider, and delete callback are required")
	}
	return r.adapter.Within(ctx, func(txCtx context.Context) error {
		executor, err := r.adapter.Executor(txCtx)
		if err != nil {
			return err
		}
		var lockedBusinessID string
		if err := executor.QueryRow(txCtx, `SELECT id::text FROM businesses WHERE id=$1::uuid FOR UPDATE`, businessID).Scan(&lockedBusinessID); err != nil {
			return classifyRepositoryWriteError("provider_brand.cleanup_lock", err)
		}
		brand, found, err := r.Get(txCtx, businessID, providerRef)
		if err != nil || !found {
			return err
		}
		// Never delete a shared brand while another channel (including pending
		// or reconnect_required) or an unexpired OAuth session may use it.
		const usageQuery = `SELECT
			EXISTS(
				SELECT 1 FROM channel_connections
				WHERE business_id=$1::uuid AND provider_ref=$2
				  AND status NOT IN ('disconnected', 'archived')
			) OR EXISTS(
				SELECT 1 FROM channel_provisioning_sessions
				WHERE business_id=$1::uuid AND provider_ref=$2
				  AND status IN ('pending_authorization', 'provisioning', 'reconnect_required')
				  AND created_at > now() - interval '15 minutes'
			)`
		var inUse bool
		if err := executor.QueryRow(txCtx, usageQuery, businessID, providerRef).Scan(&inUse); err != nil {
			return classifyRepositoryWriteError("provider_brand.cleanup_usage", err)
		}
		if inUse {
			return nil
		}
		if err := deleteRemote(txCtx, brand.ProviderBrandRef); err != nil {
			return err
		}
		_, err = executor.Exec(txCtx, `DELETE FROM channel_provider_brands
			WHERE business_id=$1::uuid AND provider_ref=$2 AND provider_brand_ref=$3`,
			businessID, providerRef, brand.ProviderBrandRef)
		if err != nil {
			return classifyRepositoryWriteError("provider_brand.cleanup_delete", err)
		}
		return nil
	})
}

var _ ports.ProviderBrandStore = (*ProviderBrandRepository)(nil)
var _ ports.ProviderBrandCleanupStore = (*ProviderBrandRepository)(nil)

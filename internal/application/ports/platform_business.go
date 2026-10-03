package ports

import (
	"context"
	"time"
)

// PlatformBusinessRecord is the Platform Admin view of a business.
//
// Per Platform Administration Contract §13 "Platform Business View": the
// platform view shows ONLY:
//   - business_id
//   - business_name
//   - owner_identity_summary
//   - platform_status
//   - created_at
//   - updated_at
//   - subscription_summary
//
// It does NOT show: customer records, conversation messages, order contents,
// full catalog, AI context, merchant secrets. Per Contract §51.
//
// The platform_status field is the businesses.status column — already CHECK-
// constrained to ('pending_setup', 'active', 'suspended', 'archived'). The
// Contract's three terminal states (ACTIVE / SUSPENDED / ARCHIVED) map onto
// the existing schema; pending_setup is the pre-activation state owned by the
// existing merchant bootstrap flow.
type PlatformBusinessRecord struct {
	ID                   string
	Name                 string
	Slug                 string
	PlatformStatus       string
	OwnerIdentitySummary *string
	SubscriptionSummary  *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// PlatformBusinessListFilter is the input to List.
// Per Contract §13: supports pagination, search, status filter, created date filter.
type PlatformBusinessListFilter struct {
	Search      string
	Status      string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Limit       int
	Cursor      string
}

type PlatformBusinessPage struct {
	Items      []PlatformBusinessRecord
	NextCursor string
	HasMore    bool
}

// PlatformBusinessLifecyclePort is the Platform Admin's lifecycle hook into
// the businesses table. Per Contract §10-14:
//   - ACTIVE → SUSPENDED (suspend)
//   - SUSPENDED → ACTIVE (reactivate)
//   - ACTIVE | SUSPENDED → ARCHIVED (archive)
//   - ARCHIVED is terminal — no restore command in V1 (Contract §11).
//
// The transitions are SQL-level UPDATEs filtered by current status. The
// repository returns RepositoryConflict for invalid transitions.
//
// These methods are intentionally separate from the merchant-owned Business
// admin endpoints — only Platform Admin can suspend/archive a business. The
// HTTP handler enforces Platform Scope (RequirePlatformAdminHuma).
type PlatformBusinessLifecyclePort interface {
	Create(ctx context.Context, create PlatformBusinessCreate) (PlatformBusinessRecord, error)
	List(ctx context.Context, filter PlatformBusinessListFilter) (PlatformBusinessPage, error)
	GetByID(ctx context.Context, businessID string) (PlatformBusinessRecord, error)
	Suspend(ctx context.Context, businessID string, now time.Time) (PlatformBusinessRecord, error)
	Reactivate(ctx context.Context, businessID string, now time.Time) (PlatformBusinessRecord, error)
	Archive(ctx context.Context, businessID string, now time.Time) (PlatformBusinessRecord, error)
}

// PlatformBusinessCreate is the input for creating a new business from the
// Platform Admin. Per Contract §9: the admin creates the business + initial
// owner invitation. The owner invitation is a SEPARATE call using the existing
// team invitation mechanism (POST /businesses/{id}/team/invitations).
//
// The new business starts in status='pending_setup' — the pre-activation
// state owned by the merchant bootstrap flow. The admin does NOT enter the
// owner's password.
type PlatformBusinessOwnerPort interface {
	GetByID(ctx context.Context, businessID string) (PlatformBusinessRecord, error)
	ActivateFromPendingSetup(ctx context.Context, businessID string, now time.Time) (PlatformBusinessRecord, error)
}

type PlatformBusinessCreate struct {
	ID              string
	Name            string
	Slug            string
	VerticalType    string
	Timezone        string
	DefaultCurrency string
	Locale          string
	Now             time.Time
}
